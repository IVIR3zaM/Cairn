package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
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
		Use:   "bump [level|version]",
		Short: "Bump the version (interactive wizard, or pass a level/version), updating manifests + docs",
		Long: "Bump computes the next version from project.canonical_version and applies it: " +
			"it updates every registered manifest in the repo and each language dir, rewrites " +
			"version-sync doc patterns, and updates canonical_version in cairn.yaml. Run it with " +
			"no argument for a guided, colorful wizard (patch/minor/major/custom with a " +
			"downgrade safeguard), or pass a level (major|minor|patch) or an explicit X.Y.Z to " +
			"apply directly. A direct bump refuses to go backwards; pass --force to allow an " +
			"explicit downgrade (the wizard double-confirms one instead). It prints a suggested " +
			"commit and tag but never commits.",
		Args: cobra.MaximumNArgs(1),
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
			if len(args) == 1 {
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
	return applyBump(wd, cfg, cur, next, out, palette{on: color})
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
	return applyBump(wd, cfg, cur, next, out, p)
}

// applyBump writes the next version into manifests, version-sync docs, and canonical, then
// prints a colorful per-file summary and the suggested (never executed) commit/tag. It is
// the shared tail of both the direct and interactive paths; the version decision and any
// guards happen before it is called.
func applyBump(wd string, cfg *config.Config, cur, next versioning.Version, out io.Writer, p palette) error {
	var changed []string
	mans, err := updateManifests(wd, next)
	if err != nil {
		return err
	}
	changed = append(changed, mans...)

	docs, err := versioning.Rewrite(wd, next.String(), cfg.VersionSync.Files)
	if err != nil {
		return err
	}
	for _, d := range docs {
		changed = append(changed, d+" (version-sync)")
	}

	did, err := updateCanonical(wd, next)
	if err != nil {
		return err
	}
	if did {
		changed = append(changed, "cairn.yaml (canonical_version)")
	}

	fmt.Fprintln(out)
	if len(changed) == 0 {
		fmt.Fprintln(out, "  "+p.paint(cYellow, "! nothing to update (already at this version)"))
	}
	for _, c := range changed {
		fmt.Fprintf(out, "  %s %s\n", p.paint(cGreen, "✓"), c)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+p.paint(cBold+cGreen, fmt.Sprintf("Bumped %s → %s.", cur, next)))
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
	fmt.Fprintf(out, "    %s -am %q\n", commit, releaseCommitMessage(cfg.Commits.Convention, next.String()))
	fmt.Fprintf(out, "    git tag v%s\n", next.String())
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
	var zero versioning.Version
	cur, err := versioning.Parse(cfg.Project.CanonicalVersion)
	if err != nil {
		return zero, zero, fmt.Errorf("project.canonical_version: %w (set it before bumping)", err)
	}

	var next versioning.Version
	if v, perr := versioning.Parse(arg); perr == nil {
		next = v
	} else if cfg.Project.Versioning == "calver" {
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
func updateManifests(wd string, next versioning.Version) ([]string, error) {
	res, err := detect.Detect(os.DirFS(wd), lookupTool)
	if err != nil {
		return nil, err
	}
	var changed []string
	seen := map[string]bool{}       // a manifest path is rewritten at most once
	changedSet := map[string]bool{} // dedupe across the version: pass and the workspace pass
	units := make([]versioning.ManifestUnit, 0, len(res.Languages))
	add := func(rel string) {
		if !changedSet[rel] {
			changedSet[rel] = true
			changed = append(changed, rel)
		}
	}
	for _, lang := range res.Languages {
		units = append(units, versioning.ManifestUnit{Dir: lang.Dir, Manifests: lang.VersionManifests})
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
	wsChanged, err := versioning.RewriteWorkspace(wd, units, next)
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
