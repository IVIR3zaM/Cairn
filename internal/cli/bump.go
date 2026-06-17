package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/IVIR3zaM/Cairn/internal/report"
	versioning "github.com/IVIR3zaM/Cairn/internal/version"
	"github.com/spf13/cobra"
)

// canonicalRe locates the value of project.canonical_version in a cairn.yaml so bump can
// advance it after a release. Group 1 is the key plus any opening quote, group 2 the
// version literal, group 3 the optional closing quote — so re-quoting is preserved.
var canonicalRe = regexp.MustCompile(`(canonical_version:\s*"?)(v?\d+\.\d+\.\d+)("?)`)

func newBumpCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "bump [pkg] [level|version]",
		Short: "Bump the version (interactive wizard, or pass a level/version), updating manifests + docs",
		Long: "Bump computes the next version from project.canonical_version and applies it: " +
			"it updates every registered manifest in the repo and each language dir, rewrites " +
			"version-sync doc patterns, and updates canonical_version in cairn.yaml. Run it with " +
			"no argument for a guided, colorful wizard (patch/minor/major/custom with a " +
			"downgrade safeguard), or pass a level (major|minor|patch) or an explicit X.Y.Z to " +
			"apply directly. In a monorepo with project.packages, pass `cairn bump <pkg> " +
			"<level|version>` to advance a single declared package from its own version line — " +
			"updating only that package's manifests, its dependents' interdependency constraints, " +
			"and its cairn.yaml entry, leaving the others (and canonical_version) untouched; the " +
			"no-argument wizard stays repo-wide. A direct bump refuses to go backwards; pass " +
			"--force to allow an explicit downgrade (the wizard double-confirms one instead). It " +
			"prints a suggested commit and tag but never commits.",
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := config.LoadOrDefault("cairn.yaml")
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			color := report.Detect(out, false, false).Color
			switch len(args) {
			case 2:
				return runPackageBump(wd, cfg, args[0], args[1], time.Now(), out, color, force)
			case 1:
				return runBump(wd, cfg, args[0], time.Now(), out, color, force)
			}
			in := cmd.InOrStdin()
			if !canPrompt(in) {
				return fmt.Errorf("bump needs a level or version when not run interactively (e.g. `cairn bump patch`)")
			}
			return runBumpWizard(wd, cfg, in, out, color)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "allow an explicit downgrade in a direct (non-interactive) bump")
	return cmd
}

// runBump applies a non-interactive bump from arg (a level or explicit version), guarding
// an unset canonical and a non-increase before touching anything. force lifts only the
// downgrade guard, the same escape hatch the wizard offers via its double-confirm. It
// leaves I/O at the edges so tests drive it on a temp tree; the wizard shares the same
// applyBump core.
func runBump(wd string, cfg *config.Config, arg string, now time.Time, out io.Writer, color, force bool) error {
	cur, next, err := computeNext(cfg, arg, now, force)
	if err != nil {
		return err
	}
	return applyBump(wd, cfg, repoPlan(cur, next), out, palette{on: color})
}

// runPackageBump advances a single declared package in a monorepo: it computes the package's
// next version from its own project.packages[].version (honoring its versioning scheme), then
// applies it only to that package — its manifests, its dependents' interdependency constraints,
// and its cairn.yaml entry — leaving every other package and canonical_version alone. The
// resolver it builds reflects the post-bump state (the target at next, all others unchanged),
// so the shared engine naturally skips honest manifests and repairs only the stale sibling pins
// that point at the bumped package.
func runPackageBump(wd string, cfg *config.Config, pkgArg, arg string, now time.Time, out io.Writer, color, force bool) error {
	idx := findPackage(cfg.Project.Packages, pkgArg)
	if idx < 0 {
		return fmt.Errorf("no package %q declared in project.packages (declared: %s)", pkgArg, declaredPaths(cfg.Project.Packages))
	}
	pkg := cfg.Project.Packages[idx]
	cur, next, err := computeNextFrom(
		fmt.Sprintf("project.packages[%s].version", path.Clean(pkg.Path)),
		pkg.Version, pkg.VersioningFor(cfg.Project.Versioning), arg, now, force)
	if err != nil {
		return err
	}

	proj := cfg.Project
	pkgs := make([]config.PackageVersion, len(proj.Packages))
	copy(pkgs, proj.Packages)
	pkgs[idx].Version = next.String()
	proj.Packages = pkgs

	label := path.Clean(pkg.Path)
	plan := bumpPlan{
		cur:   cur,
		next:  next,
		res:   versioning.NewResolver(proj),
		label: label,
		updateConfig: func(wd string) (string, error) {
			did, err := updatePackageVersion(wd, label, next)
			if err != nil || !did {
				return "", err
			}
			return fmt.Sprintf("cairn.yaml (packages/%s)", label), nil
		},
	}
	return applyBump(wd, cfg, plan, out, palette{on: color})
}

// repoPlan is the repo-wide bump plan: every unit moves to one version, so a lockstep resolver
// (everything resolves to next) drives the manifest, workspace, and version-sync passes, and the
// config update advances project.canonical_version.
func repoPlan(cur, next versioning.Version) bumpPlan {
	return bumpPlan{
		cur:   cur,
		next:  next,
		res:   versioning.NewResolver(config.Project{CanonicalVersion: next.String()}),
		label: "",
		updateConfig: func(wd string) (string, error) {
			did, err := updateCanonical(wd, next)
			if err != nil || !did {
				return "", err
			}
			return "cairn.yaml (canonical_version)", nil
		},
	}
}

// bumpPlan is one fully-decided bump: the version transition, the resolver that maps each unit
// to its target version, an optional package label (empty = repo-wide), and how to update
// cairn.yaml (canonical_version vs a single packages entry). It lets applyBump be the one shared
// tail for the direct, wizard, and per-package paths — they differ only in how the plan is built.
type bumpPlan struct {
	cur, next    versioning.Version
	res          *versioning.Resolver
	label        string
	updateConfig func(wd string) (string, error)
}

// runBumpWizard is the interactive front-end: it shows the current version, offers
// patch/minor/major/custom with their target versions, explains the jump, and confirms —
// double-confirming a downgrade in loud red — before applying. A quit or declined prompt
// aborts cleanly (nil, nothing written). Downgrades are allowed here (unlike runBump's
// guard) precisely because the operator has just confirmed twice.
func runBumpWizard(wd string, cfg *config.Config, in io.Reader, out io.Writer, color bool) error {
	p := palette{on: color}
	cur, err := versioning.Parse(cfg.Project.CanonicalVersion)
	if err != nil {
		return fmt.Errorf("project.canonical_version: %w (set it before bumping)", err)
	}
	r := bufio.NewReader(in)

	nextPatch, _ := cur.Next("patch")
	nextMinor, _ := cur.Next("minor")
	nextMajor, _ := cur.Next("major")

	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+p.paint(cBold+cCyan, "cairn — version bump"))
	hr(out, p)
	fmt.Fprintf(out, "  current version: %s\n", p.paint(cBold+cGreen, cur.String()))
	hr(out, p)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  How do you want to bump the version?")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "    %s) %s  (bug fixes)        %s → %s\n", p.paint(cBold, "1"), p.paint(cGreen, "patch"), cur, p.paint(cBold, nextPatch.String()))
	fmt.Fprintf(out, "    %s) %s  (new features)     %s → %s\n", p.paint(cBold, "2"), p.paint(cYellow, "minor"), cur, p.paint(cBold, nextMinor.String()))
	fmt.Fprintf(out, "    %s) %s  (breaking changes) %s → %s\n", p.paint(cBold, "3"), p.paint(cRed, "major"), cur, p.paint(cBold, nextMajor.String()))
	fmt.Fprintf(out, "    %s) %s (type an exact version)\n", p.paint(cBold, "4"), p.paint(cCyan, "custom"))
	fmt.Fprintf(out, "    %s) quit\n", p.paint(cBold, "q"))
	fmt.Fprintln(out)

	var next versioning.Version
	for {
		choice, err := prompt(r, out, "  "+p.paint(cBold, "choice")+" "+p.paint(cDim, "[1/2/3/4/q]")+" ")
		if err != nil {
			return err
		}
		switch strings.ToLower(choice) {
		case "1":
			next = nextPatch
		case "2":
			next = nextMinor
		case "3":
			next = nextMajor
		case "4":
			typed, err := prompt(r, out, "  enter version "+p.paint(cDim, "(e.g. 1.2.3)")+": ")
			if err != nil {
				return err
			}
			v, perr := versioning.Parse(typed)
			if perr != nil {
				fmt.Fprintln(out, "  "+p.paint(cRed, fmt.Sprintf("%q is not a valid version (expected X.Y.Z).", typed)))
				continue
			}
			if v.Compare(cur) == 0 {
				fmt.Fprintln(out, "  "+p.paint(cRed, "that is the current version — nothing to bump."))
				continue
			}
			next = v
		case "q":
			fmt.Fprintln(out, "  aborted.")
			return nil
		default:
			fmt.Fprintln(out, "  "+p.paint(cRed, "please choose 1, 2, 3, 4 or q."))
			continue
		}
		break
	}

	fmt.Fprintln(out)
	hr(out, p)
	fmt.Fprintf(out, "  %s  →  %s\n", cur, p.paint(cBold+cGreen, next.String()))
	fmt.Fprintf(out, "  This is %s\n", describeJump(p, cur, next))
	hr(out, p)

	if jumpKind(cur, next) == "downgrade" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  "+p.paint(cBgRed+cBold, " WARNING: THIS IS A DOWNGRADE "))
		fmt.Fprintln(out, "  "+p.paint(cRed+cBold, fmt.Sprintf("Going from %s down to %s should not happen normally.", cur, next)))
		fmt.Fprintln(out, "  "+p.paint(cRed, "Published versions are immutable; a downgrade can break consumers."))
		fmt.Fprintln(out)
		if !confirm(r, out, p, "  "+p.paint(cRed+cBold, "Are you absolutely sure you want to DOWNGRADE?")) {
			fmt.Fprintln(out, "  aborted.")
			return nil
		}
		if !confirm(r, out, p, "  "+p.paint(cRed+cBold, fmt.Sprintf("Confirm once more — really downgrade to %s?", next))) {
			fmt.Fprintln(out, "  aborted.")
			return nil
		}
	}

	fmt.Fprintln(out)
	if !confirm(r, out, p, fmt.Sprintf("  Apply version %s across the repo?", p.paint(cBold, next.String()))) {
		fmt.Fprintln(out, "  aborted.")
		return nil
	}
	return applyBump(wd, cfg, repoPlan(cur, next), out, p)
}

// applyBump writes the plan's next version into the resolved manifests, the version-sync docs,
// and cairn.yaml, then prints a colorful per-file summary and the suggested (never executed)
// commit/tag. It is the shared tail of the direct, interactive, and per-package paths; the
// version decision, the resolver, and the config-update strategy are all decided in the plan
// before it is called. A package-scoped plan (non-empty label) reports and tags the package.
func applyBump(wd string, cfg *config.Config, plan bumpPlan, out io.Writer, p palette) error {
	cur, next := plan.cur, plan.next
	var changed []string
	mans, err := updateManifests(wd, plan.res)
	if err != nil {
		return err
	}
	changed = append(changed, mans...)

	docs, err := versioning.Rewrite(wd, plan.res, cfg.VersionSync.Files)
	if err != nil {
		return err
	}
	for _, d := range docs {
		changed = append(changed, d+" (version-sync)")
	}

	cfgDesc, err := plan.updateConfig(wd)
	if err != nil {
		return err
	}
	if cfgDesc != "" {
		changed = append(changed, cfgDesc)
	}

	fmt.Fprintln(out)
	if len(changed) == 0 {
		fmt.Fprintln(out, "  "+p.paint(cYellow, "! nothing to update (already at this version)"))
	}
	for _, c := range changed {
		fmt.Fprintf(out, "  %s %s\n", p.paint(cGreen, "✓"), c)
	}
	fmt.Fprintln(out)
	banner := fmt.Sprintf("Bumped %s → %s.", cur, next)
	relSubject := next.String()
	tag := "v" + next.String()
	if plan.label != "" {
		banner = fmt.Sprintf("Bumped %s: %s → %s.", plan.label, cur, next)
		relSubject = plan.label + " " + next.String()
		tag = plan.label + "-v" + next.String()
	}
	fmt.Fprintln(out, "  "+p.paint(cBold+cGreen, banner))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+p.paint(cBold, "Next steps:"))
	fmt.Fprintln(out, "    "+p.paint(cDim, "# review the changes"))
	fmt.Fprintln(out, "    git diff")
	fmt.Fprintln(out, "    "+p.paint(cDim, "# run the gate"))
	fmt.Fprintln(out, "    cairn verify")
	fmt.Fprintln(out, "    "+p.paint(cDim, "# commit and tag (nothing committed for you)"))
	commit := "git commit"
	if cfg.Commits.Signoff {
		commit += " -s"
	}
	fmt.Fprintf(out, "    %s -am %q\n", commit, releaseCommitMessage(cfg.Commits.Convention, relSubject))
	fmt.Fprintf(out, "    git tag %s\n", tag)
	return nil
}

// releaseCommitMessage formats the suggested release-commit subject for the configured
// commit convention, so the hint matches the style the repo actually enforces. Full
// convention handling (validation, the rest of gitmoji/none) is the commit registry's job
// in a later iteration; bump only needs the release subject here.
func releaseCommitMessage(convention, ver string) string {
	switch convention {
	case "gitmoji":
		return "🔖 Release " + ver
	case "none":
		return "Release " + ver
	default: // conventional, and the safe fallback for an unset convention
		return "chore(release): " + ver
	}
}

// computeNext resolves the current and next version from config and the bump argument: an
// explicit X.Y.Z is honored directly; otherwise the arg is a level interpreted per the
// project's versioning scheme (semver math, or a date-based CalVer step). It guards the two
// failure modes the math layer can't see on its own: an unset canonical and a non-increase.
// force lifts the downgrade guard so an operator can deliberately set a lower version; a
// no-op (next equal to current) is still refused since there is nothing to apply.
func computeNext(cfg *config.Config, arg string, now time.Time, force bool) (versioning.Version, versioning.Version, error) {
	return computeNextFrom("project.canonical_version", cfg.Project.CanonicalVersion, cfg.Project.Versioning, arg, now, force)
}

// computeNextFrom is the version math shared by the repo-wide and per-package paths: from a
// current literal and its versioning scheme it resolves the next version (explicit X.Y.Z, or a
// level stepped per scheme), guarding the failure modes the math layer can't see — an unset/
// malformed current (subject names it for an actionable error) and a non-increase (force lifts
// only the downgrade guard, never the no-op refusal).
func computeNextFrom(subject, curLit, scheme, arg string, now time.Time, force bool) (versioning.Version, versioning.Version, error) {
	var zero versioning.Version
	cur, err := versioning.Parse(curLit)
	if err != nil {
		return zero, zero, fmt.Errorf("%s: %w (set it before bumping)", subject, err)
	}

	var next versioning.Version
	if v, perr := versioning.Parse(arg); perr == nil {
		next = v
	} else if scheme == "calver" {
		next = versioning.NextCalVer(cur, now)
	} else {
		next, err = cur.Next(arg)
		if err != nil {
			return zero, zero, err
		}
	}

	if next.Compare(cur) == 0 {
		return zero, zero, fmt.Errorf("refusing to bump: next %s is the same as current %s", next, cur)
	}
	if !force && next.Compare(cur) < 0 {
		return zero, zero, fmt.Errorf("refusing to bump: next %s is not greater than current %s (pass --force to downgrade)", next, cur)
	}
	return cur, next, nil
}

// jumpKind classifies next relative to cur for the wizard's explanation and downgrade
// safeguard: "same"/"downgrade" by ordering first, then which component increased.
func jumpKind(cur, next versioning.Version) string {
	switch {
	case next.Compare(cur) == 0:
		return "same"
	case next.Compare(cur) < 0:
		return "downgrade"
	case next.Major > cur.Major:
		return "major"
	case next.Minor > cur.Minor:
		return "minor"
	default:
		return "patch"
	}
}

// describeJump renders the one-line, color-coded meaning of the jump shown before confirm.
func describeJump(p palette, cur, next versioning.Version) string {
	switch jumpKind(cur, next) {
	case "major":
		return p.paint(cRed+cBold, "MAJOR") + " bump — signals breaking changes to the public API."
	case "minor":
		return p.paint(cYellow+cBold, "MINOR") + " bump — new backwards-compatible functionality."
	case "patch":
		return p.paint(cGreen+cBold, "PATCH") + " bump — backwards-compatible bug fixes only."
	case "same":
		return "the same version (no change)."
	default:
		return p.paint(cRed+cBold, "a DOWNGRADE") + " from the current version."
	}
}

// updateManifests sets next in each detected language's version-owned manifest, writing only
// files that changed. It discovers locations from detection — every detected unit (repo root
// and each sub-package, including pub-workspace members) contributes its declared manifest
// filename(s), resolved to a writer via versioning.ManagerFor — so a manifest is updated
// because the language owns it, not because a dir is listed in cairn.yaml. A declared file
// with no writer registered yet is skipped; a missing file is skipped; a present file without
// a locatable version errors. Returned paths are repo-relative and sorted for a clean summary.
func updateManifests(wd string, res *versioning.Resolver) ([]string, error) {
	det, err := detect.Detect(os.DirFS(wd), lookupTool)
	if err != nil {
		return nil, err
	}
	var changed []string
	seen := map[string]bool{}       // a manifest path is rewritten at most once
	changedSet := map[string]bool{} // dedupe across the version: pass and the workspace pass
	units := make([]versioning.ManifestUnit, 0, len(det.Languages))
	add := func(rel string) {
		if !changedSet[rel] {
			changedSet[rel] = true
			changed = append(changed, rel)
		}
	}
	for _, lang := range det.Languages {
		units = append(units, versioning.ManifestUnit{Dir: lang.Dir, Manifests: lang.VersionManifests})
		// Each unit is set to *its own* resolved target version (per-package in a monorepo,
		// canonical otherwise). A unit already at its target is a no-op below and skipped, so a
		// per-package bump touches only the one package whose version actually changed.
		lit := res.ForDir(lang.Dir).Version
		if lit == "" {
			continue // no version configured for this unit
		}
		next, err := versioning.Parse(lit)
		if err != nil {
			return changed, fmt.Errorf("version for %s: %w", lang.Dir, err)
		}
		for _, fname := range lang.VersionManifests {
			m, ok := versioning.ManagerFor(fname)
			if !ok {
				continue // declared location with no writer yet (native-only/future format)
			}
			rel := filepath.ToSlash(filepath.Join(lang.Dir, fname))
			if seen[rel] {
				continue
			}
			seen[rel] = true
			full := filepath.Join(wd, filepath.FromSlash(rel))
			content, err := os.ReadFile(full)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return changed, err
			}
			out, did, err := m.SetVersion(content, next)
			if err != nil {
				return changed, fmt.Errorf("%s: %w", rel, err)
			}
			if !did {
				continue
			}
			if err := os.WriteFile(full, out, 0o644); err != nil {
				return changed, err
			}
			add(rel)
		}
	}
	// Multi-package repos move member-to-member constraints in lockstep with the versions just
	// written. Handled generically: any manifest format that opts into version.Workspace
	// participates, identified by member name so a sibling pinned at any stale version is
	// repaired — not only one that matched the previous version. No language named here.
	wsChanged, err := versioning.RewriteWorkspace(wd, res, units)
	if err != nil {
		return changed, err
	}
	for _, p := range wsChanged {
		add(p)
	}
	sort.Strings(changed)
	return changed, nil
}

// updateCanonical advances project.canonical_version in cairn.yaml to next via a targeted
// substitution, preserving quoting and surrounding formatting. It reports whether the file
// changed; a missing cairn.yaml or already-correct value is a no-op.
func updateCanonical(wd string, next versioning.Version) (bool, error) {
	path := filepath.Join(wd, "cairn.yaml")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	updated := canonicalRe.ReplaceAll(content, []byte("${1}"+next.String()+"${3}"))
	if string(updated) == string(content) {
		return false, nil
	}
	return true, os.WriteFile(path, updated, 0o644)
}

// findPackage returns the index of the project.packages entry whose path matches pkgArg
// (compared cleaned, so a trailing slash or "./" prefix still matches), or -1 when none does.
func findPackage(pkgs []config.PackageVersion, pkgArg string) int {
	want := path.Clean(pkgArg)
	for i, p := range pkgs {
		if path.Clean(p.Path) == want {
			return i
		}
	}
	return -1
}

// declaredPaths lists the declared package paths for the "no such package" error, so an operator
// who mistypes a package sees the valid choices.
func declaredPaths(pkgs []config.PackageVersion) string {
	if len(pkgs) == 0 {
		return "none — project.packages is empty"
	}
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = path.Clean(p.Path)
	}
	return strings.Join(names, ", ")
}

// pkgListItemRe matches a YAML sequence-item marker, capturing its leading indent so the bounds
// of one project.packages entry can be found.
var pkgListItemRe = regexp.MustCompile(`^(\s*)-\s`)

// pkgPathLineRe matches a `path: <value>` line (whether the list-item marker line or a
// continuation), capturing the path value with optional quotes stripped.
var pkgPathLineRe = regexp.MustCompile(`^\s*(?:-\s+)?path:\s*"?([^"\s]+)"?\s*$`)

// pkgVersionLineRe matches a `version: X.Y.Z` line within a package entry, capturing the prefix,
// the version literal, and any trailing quote/space so quoting and layout are preserved on rewrite.
var pkgVersionLineRe = regexp.MustCompile(`^(\s*(?:-\s+)?version:\s*"?)(v?\d+\.\d+\.\d+)("?\s*)$`)

// updatePackageVersion advances one project.packages entry's version: line in cairn.yaml to next
// via a targeted line edit, preserving quoting, ordering, and the other entries. It locates the
// entry by its cleaned path, scopes the rewrite to that one list item (so a sibling entry on the
// same version is untouched), and reports whether the file changed. A missing cairn.yaml, an
// absent entry, or an already-correct value is a no-op.
func updatePackageVersion(wd, pkgPath string, next versioning.Version) (bool, error) {
	file := filepath.Join(wd, "cairn.yaml")
	content, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(content), "\n")

	pathIdx := -1
	for i, ln := range lines {
		if m := pkgPathLineRe.FindStringSubmatch(ln); m != nil && path.Clean(m[1]) == pkgPath {
			pathIdx = i
			break
		}
	}
	if pathIdx == -1 {
		return false, nil
	}

	// Walk back to the entry's list-item marker, then forward to where the item ends (a sibling
	// item or a dedent), so the version rewrite stays inside this one package entry.
	start := pathIdx
	for start > 0 && !pkgListItemRe.MatchString(lines[start]) {
		start--
	}
	marker := pkgListItemRe.FindStringSubmatch(lines[start])
	if marker == nil {
		return false, nil // not a sequence item — leave a malformed file untouched
	}
	indent := len(marker[1])
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if lead := len(lines[i]) - len(strings.TrimLeft(lines[i], " ")); lead <= indent {
			end = i
			break
		}
	}

	for i := start; i < end; i++ {
		m := pkgVersionLineRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		if m[2] == next.String() {
			return false, nil // already correct
		}
		lines[i] = m[1] + next.String() + m[3]
		return true, os.WriteFile(file, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return false, nil
}

// canPrompt reports whether r is an interactive terminal we can read a wizard answer from,
// so a no-argument bump in a pipe or CI fails fast instead of reading EOF mid-prompt.
func canPrompt(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// palette paints text with ANSI codes when enabled; a no-op palette keeps piped/NO_COLOR
// output clean. Codes mirror the reference bump-version.sh wizard.
type palette struct{ on bool }

const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cCyan   = "\033[36m"
	cBgRed  = "\033[41m\033[97m"
)

func (p palette) paint(code, s string) string {
	if !p.on {
		return s
	}
	return code + s + cReset
}

// hr draws a dim horizontal rule framing the wizard sections.
func hr(out io.Writer, p palette) {
	fmt.Fprintln(out, "  "+p.paint(cDim, strings.Repeat("─", 52)))
}

// prompt writes label and reads one trimmed line. An EOF with no input surfaces as an error
// so a closed/empty stdin ends the wizard instead of looping forever.
func prompt(r *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// confirm asks a yes/no question, defaulting to no on anything but y/yes (or EOF).
func confirm(r *bufio.Reader, out io.Writer, p palette, q string) bool {
	ans, err := prompt(r, out, q+" "+p.paint(cDim, "[y/N]")+" ")
	if err != nil {
		return false
	}
	ans = strings.ToLower(ans)
	return ans == "y" || ans == "yes"
}
