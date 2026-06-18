package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/IVIR3zaM/Cairn/internal/changelog"
	"github.com/IVIR3zaM/Cairn/internal/commit"
	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/IVIR3zaM/Cairn/internal/report"
	versioning "github.com/IVIR3zaM/Cairn/internal/version"
	"github.com/spf13/cobra"
)

// baselineVersionRe locates the top-level (column-0) `version:` value in a schema-2 cairn.yaml
// so a repo-wide bump can advance the repo baseline. The `(?m)^` anchors it to an unindented
// line, so nested `directories.<path>.version:` entries are never matched. Group 1 is the key
// plus any opening quote, group 2 the version literal, group 3 the optional closing quote and
// trailing whitespace — so re-quoting and layout are preserved.
var baselineVersionRe = regexp.MustCompile(`(?m)^(version:[ \t]*"?)(v?\d+\.\d+\.\d+)("?[ \t]*)$`)

func newBumpCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "bump [pkg] [level|version]",
		Short: "Bump the version (interactive wizard, or pass a level/version), updating manifests + docs",
		Long: "Bump computes the next version from the repo baseline `version` and applies it: " +
			"it updates every registered manifest in the repo and each language dir, rewrites " +
			"version-sync doc patterns, and updates the baseline `version` in cairn.yaml. Run it with " +
			"no argument for a guided, colorful wizard (patch/minor/major/custom with a " +
			"downgrade safeguard), or pass a level (major|minor|patch) or an explicit X.Y.Z to " +
			"apply directly. In a monorepo with independently-versioned directories, pass " +
			"`cairn bump <pkg> <level|version>` to advance a single directory from its own version " +
			"line — updating only that directory's manifests, its dependents' interdependency " +
			"constraints, and its directories.<path>.version entry, leaving the others (and the " +
			"baseline) untouched; the no-argument wizard stays repo-wide. A direct bump refuses to go " +
			"backwards; pass --force to allow an explicit downgrade (the wizard double-confirms one " +
			"instead). It prints a suggested commit and tag but never commits.",
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			// config owns the per-directory cascade: bump asks the Tree for the resolved baseline
			// and per-directory settings instead of reading cairn.yaml or re-deriving precedence.
			tree, err := config.LoadTree(os.DirFS(wd))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			color := report.Detect(out, false, false).Color
			switch len(args) {
			case 2:
				return runPackageBump(wd, tree, args[0], args[1], time.Now(), out, color, force)
			case 1:
				// `cairn bump <pkg>` (an independently-versioned directory, no level) infers that
				// package's level from its own path-scoped history and advances only it; otherwise
				// the lone arg is a repo-wide level/version.
				if dir := findIndependent(tree, args[0]); dir != "" {
					return runInferredPackageBump(wd, tree, dir, time.Now(), out, color, force)
				}
				return runBump(wd, tree, args[0], time.Now(), out, color, force)
			}
			// No argument. In a monorepo with independent directories, levels are per-package, so
			// show a per-package inferred summary rather than a single repo-wide choice.
			if len(tree.Independent()) > 0 {
				return runMonorepoSummary(wd, tree, out, color)
			}
			// Repo-wide (no independent dirs): infer one level from the commit history since the
			// last tag, using the configured commit convention. The wizard preselects it; a
			// non-interactive run applies it directly (and errors only if nothing could be inferred).
			inferred := inferLevel(wd, tree)
			in := cmd.InOrStdin()
			if canPrompt(in) {
				return runBumpWizard(wd, tree, in, out, color, inferred)
			}
			if inferred == "" {
				return fmt.Errorf("bump needs a level or version when not run interactively (e.g. `cairn bump patch`); none could be inferred from commit history since the last tag")
			}
			return runBump(wd, tree, inferred, time.Now(), out, color, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "allow an explicit downgrade in a direct (non-interactive) bump")
	return cmd
}

// resolvedCommits returns the repo baseline commit convention config, falling back to the
// in-code defaults when no layer set it — so bump reads the convention from the resolved Tree
// rather than re-deriving it.
func resolvedCommits(tree *config.Tree) config.Commits {
	root, _ := tree.Resolve(".")
	if root.Commits != nil {
		return *root.Commits
	}
	return config.Default().Commits
}

// resolvedVersionSyncFiles returns the repo baseline version_sync files, or none when unset.
func resolvedVersionSyncFiles(tree *config.Tree) []config.VersionSyncFile {
	root, _ := tree.Resolve(".")
	if root.VersionSync != nil {
		return root.VersionSync.Files
	}
	return nil
}

// inferLevel infers the bump level from commit history using the project's configured commit
// convention: it classifies every commit since the last tag and takes the highest implied
// bump (see commit.InferBump). It returns "" — "couldn't infer a level" — when the convention
// has no registered validator, the repo has no commits/tags/git, or nothing release-worthy was
// found; callers treat that as "ask for / require an explicit level".
func inferLevel(wd string, tree *config.Tree) string {
	v, ok := commit.ValidatorFor(resolvedCommits(tree).Convention)
	if !ok {
		return ""
	}
	return commit.InferBump(v, commitHistory(wd)).Level()
}

// commitHistory returns the commit message bodies that a repo-wide release would cover:
// everything since the most recent tag, or the entire history when the repo has no tags. It is
// the path-blind aggregate inference (8b) uses for the canonical/lockstep bump.
func commitHistory(wd string) []string {
	return gitLogMessages(wd, lastTag(wd), "")
}

// commitHistoryFor returns the commit message bodies a single package's release would cover:
// commits since the package's own last tag (`<pkg>-v*`, the form a package-scoped bump prints)
// that actually touched its directory. With no tag yet it degrades to the package's whole
// history, so a never-released package still infers from everything that built it.
func commitHistoryFor(wd, pkgPath string) []string {
	return gitLogMessages(wd, lastPackageTag(wd, pkgPath), pkgPath)
}

// gitLogMessages shells out to git (never reinventing the walk) for the commit bodies in
// tag..HEAD (or all of history when tag is ""), optionally restricted to a pathspec so a
// monorepo package only sees the commits that touched it. It degrades to an empty slice on any
// failure — no git, no commits, not a repo — so inference finds nothing rather than erroring.
// `-z` separates commits with NUL so multi-line bodies survive intact.
func gitLogMessages(wd, tag, pathspec string) []string {
	args := []string{"-C", wd, "log", "-z", "--format=%B"}
	if tag != "" {
		args = append(args, tag+"..HEAD")
	}
	if pathspec != "" {
		args = append(args, "--", pathspec)
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil
	}
	var msgs []string
	for _, m := range strings.Split(string(out), "\x00") {
		if strings.TrimSpace(m) != "" {
			msgs = append(msgs, m)
		}
	}
	return msgs
}

// lastTag returns the most recent tag reachable from HEAD, or "" when the repo has none (or
// isn't a git repo) — in which case the whole history is considered.
func lastTag(wd string) string {
	out, err := exec.Command("git", "-C", wd, "describe", "--tags", "--abbrev=0").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// lastPackageTag returns the most recent tag matching this package's scheme (`<pkg>-v*`, what
// applyBump prints for a package-scoped bump), or "" when the package has never been tagged —
// in which case the package's whole history is considered. label is the cleaned package path.
func lastPackageTag(wd, label string) string {
	out, err := exec.Command("git", "-C", wd, "describe", "--tags", "--abbrev=0", "--match", label+"-v*").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// inferPackageLevel infers a single package directory's bump level from the commits that touched
// it since its own last tag, classified via the project's configured commit convention (see
// commit.InferBump). It returns "" — "nothing release-worthy" — when the convention has no
// validator, or the package has no qualifying commits; callers treat that as "require/skip".
func inferPackageLevel(wd string, tree *config.Tree, dir string) string {
	v, ok := commit.ValidatorFor(resolvedCommits(tree).Convention)
	if !ok {
		return ""
	}
	return commit.InferBump(v, commitHistoryFor(wd, path.Clean(dir))).Level()
}

// runBump applies a non-interactive bump from arg (a level or explicit version), guarding
// an unset baseline version and a non-increase before touching anything. force lifts only the
// downgrade guard, the same escape hatch the wizard offers via its double-confirm. It
// leaves I/O at the edges so tests drive it on a temp tree; the wizard shares the same
// applyBump core.
func runBump(wd string, tree *config.Tree, arg string, now time.Time, out io.Writer, color, force bool) error {
	cur, next, err := computeNext(tree, arg, now, force)
	if err != nil {
		return err
	}
	return applyBump(wd, tree, repoPlan(tree, cur, next, now), out, palette{on: color})
}

// runPackageBump advances a single independently-versioned directory in a monorepo: it computes
// the directory's next version from its own resolved version (honoring its versioning scheme),
// then applies it only to that directory — its manifests, its dependents' interdependency
// constraints, and its directories.<path>.version entry — leaving every other directory and the
// baseline alone. The resolver it builds reflects the post-bump state (the target at next, all
// others unchanged), so the shared engine naturally skips honest manifests and repairs only the
// stale sibling pins that point at the bumped package.
func runPackageBump(wd string, tree *config.Tree, pkgArg, arg string, now time.Time, out io.Writer, color, force bool) error {
	dir := findIndependent(tree, pkgArg)
	if dir == "" {
		return fmt.Errorf("no independently-versioned directory %q (declared: %s)", pkgArg, declaredPaths(tree))
	}
	cur := versioning.NewResolverFromTree(tree).ForDir(dir)
	curVer, next, err := computeNextFrom(
		fmt.Sprintf("directories.%s.version", dir), cur.Version, cur.Versioning, arg, now, force,
	)
	if err != nil {
		return err
	}

	plan := bumpPlan{
		cur:   curVer,
		next:  next,
		date:  now,
		res:   versioning.NewResolverFromTree(tree.WithVersion(dir, next.String())),
		label: dir,
		updateConfig: func(wd string) (string, error) {
			did, err := updateDirectoryVersion(wd, dir, next)
			if err != nil || !did {
				return "", err
			}
			return fmt.Sprintf("cairn.yaml (directories/%s)", dir), nil
		},
	}
	return applyBump(wd, tree, plan, out, palette{on: color})
}

// runInferredPackageBump advances one independent directory whose level was not given: it infers
// the level from the commits that touched that directory since its own last tag, then applies it
// via the shared per-package path. It fails fast (nothing written) when no release-worthy change
// can be inferred, so the operator gets the same "say what to bump" feedback the repo-wide path
// gives.
func runInferredPackageBump(wd string, tree *config.Tree, dir string, now time.Time, out io.Writer, color, force bool) error {
	level := inferPackageLevel(wd, tree, dir)
	if level == "" {
		return fmt.Errorf("bump %s needs a level or version (e.g. `cairn bump %s patch`); none could be inferred from commit history touching %s since its last tag", dir, dir, dir)
	}
	return runPackageBump(wd, tree, dir, level, now, out, color, force)
}

// runMonorepoSummary is the no-argument flow for a monorepo with independent directories: instead
// of one repo-wide choice, it prints each independent directory with the level inferred from its
// own scoped history and the projected next version, then points at `cairn bump <pkg>` to apply
// one. It is read-only — a monorepo release advances directories one explicit command at a time.
func runMonorepoSummary(wd string, tree *config.Tree, out io.Writer, color bool) error {
	p := palette{on: color}
	res := versioning.NewResolverFromTree(tree)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+p.paint(cBold+cCyan, "cairn — per-package bump"))
	hr(out, p)
	for _, dir := range tree.Independent() {
		level := inferPackageLevel(wd, tree, dir)
		fmt.Fprintf(out, "  %s  %s\n", p.paint(cBold, dir), summarizeLevel(p, res.ForDir(dir).Version, level))
	}
	hr(out, p)
	fmt.Fprintln(out, "  Apply one with "+p.paint(cBold, "cairn bump <pkg>")+" (inferred level) or "+p.paint(cBold, "cairn bump <pkg> <level|version>")+".")
	return nil
}

// summarizeLevel renders one package's inferred bump for the monorepo summary: the level and the
// projected current→next jump, or a dim "no release-worthy changes" when nothing was inferred (or
// the current version / level can't be projected, e.g. a malformed literal).
func summarizeLevel(p palette, curLit, level string) string {
	if level == "" {
		return p.paint(cDim, "no release-worthy changes")
	}
	cur, err := versioning.Parse(curLit)
	if err != nil {
		return level
	}
	next, err := cur.Next(level)
	if err != nil {
		return level
	}
	return fmt.Sprintf("%s  %s → %s", level, cur, p.paint(cBold, next.String()))
}

// repoPlan is the repo-wide bump plan: every lockstep unit moves to one version, so a resolver
// over the post-bump Tree (the baseline set to next) drives the manifest, workspace, and
// version-sync passes, and the config update advances the baseline `version`.
func repoPlan(tree *config.Tree, cur, next versioning.Version, now time.Time) bumpPlan {
	return bumpPlan{
		cur:   cur,
		next:  next,
		date:  now,
		res:   versioning.NewResolverFromTree(tree.WithVersion(".", next.String())),
		label: "",
		updateConfig: func(wd string) (string, error) {
			did, err := updateBaselineVersion(wd, next)
			if err != nil || !did {
				return "", err
			}
			return "cairn.yaml (version)", nil
		},
	}
}

// bumpPlan is one fully-decided bump: the version transition, the resolver that maps each unit
// to its target version, an optional package label (empty = repo-wide), and how to update
// cairn.yaml (baseline version vs a single directories entry). It lets applyBump be the one
// shared tail for the direct, wizard, and per-package paths — they differ only in how the plan
// is built.
type bumpPlan struct {
	cur, next    versioning.Version
	date         time.Time
	res          *versioning.Resolver
	label        string
	updateConfig func(wd string) (string, error)
}

// runBumpWizard is the interactive front-end: it shows the current version, offers
// patch/minor/major/custom with their target versions, explains the jump, and confirms —
// double-confirming a downgrade in loud red — before applying. A quit or declined prompt
// aborts cleanly (nil, nothing written). Downgrades are allowed here (unlike runBump's
// guard) precisely because the operator has just confirmed twice.
func runBumpWizard(wd string, tree *config.Tree, in io.Reader, out io.Writer, color bool, inferred string) error {
	p := palette{on: color}
	root, _ := tree.Resolve(".")
	cur, err := versioning.Parse(rootVersion(root))
	if err != nil {
		return fmt.Errorf("version: %w (set it before bumping)", err)
	}
	r := bufio.NewReader(in)

	nextPatch, _ := cur.Next("patch")
	nextMinor, _ := cur.Next("minor")
	nextMajor, _ := cur.Next("major")
	byLevel := map[string]versioning.Version{"patch": nextPatch, "minor": nextMinor, "major": nextMajor}

	// rec marks the level inferred from commit history so the operator sees (and can accept
	// with a bare Enter) the bump the commits imply.
	rec := func(level string) string {
		if inferred != "" && level == inferred {
			return "  " + p.paint(cBold+cCyan, "← inferred from commits")
		}
		return ""
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+p.paint(cBold+cCyan, "cairn — version bump"))
	hr(out, p)
	fmt.Fprintf(out, "  current version: %s\n", p.paint(cBold+cGreen, cur.String()))
	hr(out, p)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  How do you want to bump the version?")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "    %s) %s  (bug fixes)        %s → %s%s\n", p.paint(cBold, "1"), p.paint(cGreen, "patch"), cur, p.paint(cBold, nextPatch.String()), rec("patch"))
	fmt.Fprintf(out, "    %s) %s  (new features)     %s → %s%s\n", p.paint(cBold, "2"), p.paint(cYellow, "minor"), cur, p.paint(cBold, nextMinor.String()), rec("minor"))
	fmt.Fprintf(out, "    %s) %s  (breaking changes) %s → %s%s\n", p.paint(cBold, "3"), p.paint(cRed, "major"), cur, p.paint(cBold, nextMajor.String()), rec("major"))
	fmt.Fprintf(out, "    %s) %s (type an exact version)\n", p.paint(cBold, "4"), p.paint(cCyan, "custom"))
	fmt.Fprintf(out, "    %s) quit\n", p.paint(cBold, "q"))
	fmt.Fprintln(out)

	hint := "[1/2/3/4/q]"
	if inferred != "" {
		hint = "[1/2/3/4/q] (Enter = " + inferred + ")"
	}
	var next versioning.Version
	for {
		choice, err := prompt(r, out, "  "+p.paint(cBold, "choice")+" "+p.paint(cDim, hint)+" ")
		if err != nil {
			return err
		}
		switch strings.ToLower(choice) {
		case "":
			if inferred == "" {
				fmt.Fprintln(out, "  "+p.paint(cRed, "please choose 1, 2, 3, 4 or q."))
				continue
			}
			next = byLevel[inferred]
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
	return applyBump(wd, tree, repoPlan(tree, cur, next, time.Now()), out, p)
}

// applyBump writes the plan's next version into the resolved manifests, the version-sync docs,
// and cairn.yaml, then prints a colorful per-file summary and the suggested (never executed)
// commit/tag. It is the shared tail of the direct, interactive, and per-package paths; the
// version decision, the resolver, and the config-update strategy are all decided in the plan
// before it is called. A package-scoped plan (non-empty label) reports and tags the package.
func applyBump(wd string, tree *config.Tree, plan bumpPlan, out io.Writer, p palette) error {
	cur, next := plan.cur, plan.next

	// Pre-flight the changelogs before mutating anything: a release with an empty `[Unreleased]`
	// section is refused here, so the bump fails with nothing written rather than shipping a
	// version with no notes. The actual promotions are applied below after the version writes.
	clWrites, err := planChangelogs(wd, tree, plan)
	if err != nil {
		return err
	}

	var changed []string
	mans, err := updateManifests(wd, tree, plan.res)
	if err != nil {
		return err
	}
	changed = append(changed, mans...)

	docs, err := versioning.Rewrite(wd, plan.res, resolvedVersionSyncFiles(tree))
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

	for _, w := range clWrites {
		if err := os.WriteFile(filepath.Join(wd, filepath.FromSlash(w.file)), w.content, 0o644); err != nil {
			return err
		}
		changed = append(changed, w.file+" (changelog)")
	}

	commits := resolvedCommits(tree)
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
	commitCmd := "git commit"
	if commits.Signoff {
		commitCmd += " -s"
	}
	fmt.Fprintf(out, "    %s -am %q\n", commitCmd, releaseCommitMessage(commits.Convention, relSubject))
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

// computeNext resolves the current and next version from the repo baseline and the bump
// argument: an explicit X.Y.Z is honored directly; otherwise the arg is a level interpreted per
// the baseline versioning scheme (semver math, or a date-based CalVer step). It guards the two
// failure modes the math layer can't see on its own: an unset baseline and a non-increase.
// force lifts the downgrade guard so an operator can deliberately set a lower version; a
// no-op (next equal to current) is still refused since there is nothing to apply.
func computeNext(tree *config.Tree, arg string, now time.Time, force bool) (versioning.Version, versioning.Version, error) {
	root, _ := tree.Resolve(".")
	scheme := ""
	if root.Versioning != nil {
		scheme = *root.Versioning
	}
	return computeNextFrom("version", rootVersion(root), scheme, arg, now, force)
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

// updateManifests sets each detected language's version-owned manifest to *its* resolved target
// version, writing only files that changed. It discovers locations from detection — every detected
// unit (repo root and each sub-package, including pub-workspace members) contributes its declared
// manifest filename(s), resolved to a writer via versioning.ManagerFor — so a manifest is updated
// because the language owns it, not because a dir is listed in cairn.yaml. A declared file with no
// writer registered yet is skipped; a missing file is skipped; a present file without a locatable
// version errors. Returned paths are repo-relative and sorted for a clean summary.
func updateManifests(wd string, tree *config.Tree, res *versioning.Resolver) ([]string, error) {
	langs, err := detectedEnabled(wd, tree)
	if err != nil {
		return nil, err
	}
	var changed []string
	seen := map[string]bool{}       // a manifest path is rewritten at most once
	changedSet := map[string]bool{} // dedupe across the version: pass and the workspace pass
	units := make([]versioning.ManifestUnit, 0, len(langs))
	add := func(rel string) {
		if !changedSet[rel] {
			changedSet[rel] = true
			changed = append(changed, rel)
		}
	}
	for _, lang := range langs {
		units = append(units, versioning.ManifestUnit{Dir: lang.Dir, Manifests: lang.VersionManifests})
		// Each unit is set to *its own* resolved target version (per-package in a monorepo,
		// baseline otherwise). A unit already at its target is a no-op below and skipped, so a
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

// updateBaselineVersion advances the top-level `version` in cairn.yaml to next via a targeted
// substitution, preserving quoting and surrounding formatting. It reports whether the file
// changed; a missing cairn.yaml or already-correct value is a no-op.
func updateBaselineVersion(wd string, next versioning.Version) (bool, error) {
	file := filepath.Join(wd, "cairn.yaml")
	content, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	updated := baselineVersionRe.ReplaceAll(content, []byte("${1}"+next.String()+"${3}"))
	if string(updated) == string(content) {
		return false, nil
	}
	return true, os.WriteFile(file, updated, 0o644)
}

// changelogWrite is one pre-computed changelog promotion: the repo-relative file and its new
// content. Writes are computed up front (in planChangelogs) so the bump can refuse an empty
// `[Unreleased]` before touching any file, then applied only once everything else succeeds.
type changelogWrite struct {
	file    string
	content []byte
}

// changelogTarget names a changelog to promote and to which version, in which standard's style.
type changelogTarget struct {
	file     string
	standard string
	version  versioning.Version
}

// planChangelogs computes every changelog promotion for this bump and refuses the bump if any
// targeted changelog has an empty `[Unreleased]` section — so an empty changelog fails the bump
// (nothing written) instead of cutting a notes-less release. A missing changelog file is skipped
// (a package needn't keep one); a standard with no registered writer yet is skipped.
func planChangelogs(wd string, tree *config.Tree, plan bumpPlan) ([]changelogWrite, error) {
	targets, err := changelogTargets(wd, tree, plan)
	if err != nil {
		return nil, err
	}
	var writes []changelogWrite
	var empty []string
	for _, t := range targets {
		w, ok := changelog.WriterFor(t.standard)
		if !ok {
			continue // standard whose writer is still a future one-file addition
		}
		content, err := os.ReadFile(filepath.Join(wd, filepath.FromSlash(t.file)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		res, err := w.Promote(content, t.version, plan.date)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.file, err)
		}
		if !res.Found {
			continue // file present but doesn't use the unreleased convention — skip, don't fail
		}
		if res.Empty {
			empty = append(empty, t.file)
			continue
		}
		if res.Changed {
			writes = append(writes, changelogWrite{file: t.file, content: res.Content})
		}
	}
	if len(empty) > 0 {
		sort.Strings(empty)
		return nil, fmt.Errorf("refusing to bump: the changelog [Unreleased] section is empty in %s — add release notes there before bumping", strings.Join(empty, ", "))
	}
	return writes, nil
}

// changelogTargets lists the changelogs a bump promotes. A repo-wide bump promotes the repo
// baseline changelog (lockstep units share it). A package-scoped bump promotes only the bumped
// directory's own changelog (resolved via its per-directory override block — its own standard
// and file, falling back to the baseline), at that package's resolved version — leaving the
// root and the other packages alone. This is the schema-2 successor to the old
// changelog.packages style: a directory keeps its own changelog through its override block.
func changelogTargets(_ string, tree *config.Tree, plan bumpPlan) ([]changelogTarget, error) {
	if plan.label == "" {
		cl := resolvedDirChangelog(tree, ".")
		if cl.File == "" {
			return nil, nil
		}
		return []changelogTarget{{file: cl.File, standard: cl.Standard, version: plan.next}}, nil
	}
	dir := plan.label
	lit := plan.res.ForDir(dir).Version
	if lit == "" {
		return nil, nil
	}
	v, err := versioning.Parse(lit)
	if err != nil {
		return nil, fmt.Errorf("version for %s: %w", dir, err)
	}
	cl := resolvedDirChangelog(tree, dir)
	if cl.File == "" {
		return nil, nil
	}
	return []changelogTarget{{
		file:     filepath.ToSlash(filepath.Join(dir, cl.File)),
		standard: cl.Standard,
		version:  v,
	}}, nil
}

// resolvedDirChangelog returns dir's effective changelog (standard + file) from the Tree's
// cascade, filling any field a partial override left unset from the in-code defaults — so a
// directory that overrides only `changelog.standard` still resolves a file, and vice versa.
func resolvedDirChangelog(tree *config.Tree, dir string) config.Changelog {
	cl := config.Default().Changelog
	if d, ok := tree.Resolve(dir); ok && d.Changelog != nil {
		if d.Changelog.Standard != "" {
			cl.Standard = d.Changelog.Standard
		}
		if d.Changelog.File != "" {
			cl.File = d.Changelog.File
		}
	}
	return cl
}

// detectedEnabled returns the detected languages minus any pruned by the disable gate or
// explicitly disabled in the resolved per-directory config (`languages.<name>.enabled: false`),
// so bump — like verify — never bumps or promotes a tree the project opted out of. The
// enabled/active decision is resolved entirely by config's Tree, not re-derived here.
func detectedEnabled(wd string, tree *config.Tree) ([]detect.Language, error) {
	det, err := detect.Detect(os.DirFS(wd), lookupTool)
	if err != nil {
		return nil, err
	}
	var langs []detect.Language
	for _, lang := range det.Languages {
		resolved, active := tree.Resolve(lang.Dir)
		if !active {
			continue // directory pruned by the absolute disable gate
		}
		if l, ok := resolved.Languages[lang.Name]; ok && !l.Enabled {
			continue // explicitly disabled in cairn.yaml
		}
		langs = append(langs, lang)
	}
	return langs, nil
}

// findIndependent returns the cleaned directory of the independently-versioned package matching
// pkgArg (compared cleaned, so a trailing slash or "./" prefix still matches), or "" when none
// of the Tree's Independent() directories match.
func findIndependent(tree *config.Tree, pkgArg string) string {
	want := path.Clean(pkgArg)
	for _, dir := range tree.Independent() {
		if dir == want {
			return dir
		}
	}
	return ""
}

// declaredPaths lists the independently-versioned directories for the "no such package" error,
// so an operator who mistypes a package sees the valid choices.
func declaredPaths(tree *config.Tree) string {
	dirs := tree.Independent()
	if len(dirs) == 0 {
		return "none — no directory declares its own version"
	}
	return strings.Join(dirs, ", ")
}

// dirKeyRe matches a YAML mapping key line, capturing its leading indent and key name so the
// bounds of one directories.<path> entry can be found.
var dirKeyRe = regexp.MustCompile(`^(\s*)([^\s:]+):\s*$`)

// dirVersionLineRe matches a `version: X.Y.Z` line, capturing the prefix, the version literal,
// and any trailing quote/space so quoting and layout are preserved on rewrite.
var dirVersionLineRe = regexp.MustCompile(`^(\s*version:\s*"?)(v?\d+\.\d+\.\d+)("?\s*)$`)

// updateDirectoryVersion advances one directories.<path>.version line in the root cairn.yaml to
// next via a targeted line edit, preserving quoting, ordering, and the other entries. It locates
// the top-level `directories:` map, then the entry whose key (cleaned) matches dir, scopes the
// rewrite to that entry's block (so a sibling on the same version is untouched), and reports
// whether the file changed. A missing cairn.yaml, an absent entry, or an already-correct value
// is a no-op (the version may live in the directory's own cairn.yaml, which this does not edit).
func updateDirectoryVersion(wd, dir string, next versioning.Version) (bool, error) {
	file := filepath.Join(wd, "cairn.yaml")
	content, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(content), "\n")

	// Find the top-level `directories:` key (column 0).
	dirsIdx := -1
	for i, ln := range lines {
		if ln == "directories:" || (strings.HasPrefix(ln, "directories:") && !strings.HasPrefix(ln, " ")) {
			dirsIdx = i
			break
		}
	}
	if dirsIdx == -1 {
		return false, nil
	}

	// Walk the entries indented under `directories:`, find the one whose key matches dir.
	entryStart, entryIndent := -1, -1
	for i := dirsIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		lead := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		if lead == 0 {
			break // back to a top-level key — out of the directories block
		}
		m := dirKeyRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		if entryIndent != -1 && lead > entryIndent {
			continue // a nested key inside an entry's block, not an entry key
		}
		entryIndent = lead
		if path.Clean(strings.Trim(m[2], `"'`)) == dir {
			entryStart = i
			break
		}
	}
	if entryStart == -1 {
		return false, nil
	}

	// Scope to this entry's block: lines indented deeper than the entry key.
	for i := entryStart + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if lead := len(lines[i]) - len(strings.TrimLeft(lines[i], " ")); lead <= entryIndent {
			break
		}
		m := dirVersionLineRe.FindStringSubmatch(lines[i])
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
