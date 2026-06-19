package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/changelog"
	"github.com/IVIR3zaM/Cairn/internal/commit"
	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	versioning "github.com/IVIR3zaM/Cairn/internal/version"
	"github.com/IVIR3zaM/Cairn/internal/wiring"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up Cairn in this repo: write cairn.yaml, install git hooks, generate CI",
		Long: "Init onboards a repo onto Cairn: it detects the languages present, writes a " +
			"schema-2 cairn.yaml with smart defaults (the detected languages enabled, the standard " +
			"verify/commit/changelog conventions), installs the configured git hooks, and generates " +
			"the CI workflow — so the same `cairn verify` runs locally and in CI. It never clobbers " +
			"an existing cairn.yaml. On a terminal it runs a short guided wizard; run with --yes to " +
			"accept the smart defaults non-interactively.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case yes:
				return runInit(wd, out)
			case isInteractive(cmd.InOrStdin()):
				return runInitWizard(wd, cmd.InOrStdin(), out)
			default:
				return errors.New("init needs a terminal for the guided wizard; re-run with --yes to accept smart defaults")
			}
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: accept smart defaults from detection")
	return cmd
}

// runInit performs the non-interactive (`--yes`) onboarding and is the testable core of the
// command: detect languages, write a schema-2 cairn.yaml (kept untouched if one already exists),
// install the configured git hooks, generate the CI workflow, and print next steps. config owns
// the cascade, so hooks/CI are read from the resolved Tree rather than re-derived — honoring a
// pre-existing config.
func runInit(wd string, out io.Writer) error {
	res, err := detect.Detect(os.DirFS(wd), exec.LookPath)
	if err != nil {
		return err
	}
	base := discoverConfig(wd, os.DirFS(wd), res)
	// --yes records per-directory facts the same way it records repo-baseline ones: every sub-unit
	// whose CHANGELOG format or manifest version differs from the baseline gets a `directories.<path>`
	// override. Decisions a repo can't detect (disabling a subtree, a strictness/scheme override) are
	// the wizard's to collect.
	dirs := detectDirOverrides(os.DirFS(wd), discoverUnits(os.DirFS(wd), res), base)
	base.Languages = pruneDisabledLanguages(base.Languages, res, dirs)
	if err := writeInitConfig(wd, base, dirs, enabledLanguageNames(base.Languages), out); err != nil {
		return err
	}
	return finishInit(wd, out, true, true)
}

// finishInit runs Wiring from the resolved Tree and prints next steps — the shared tail of both
// the `--yes` and the interactive paths. doHooks/doCI let the wizard skip a step the user
// declined; with both true it is the full non-interactive onboarding. config owns the cascade, so
// the hooks/CI shapes come from the resolved Tree (honoring a pre-existing or wizard-written
// config) rather than being re-derived here.
func finishInit(wd string, out io.Writer, doHooks, doCI bool) error {
	tree, err := config.LoadTree(os.DirFS(wd))
	if err != nil {
		return err
	}
	root, _ := tree.Resolve(".")
	cfg := &config.Config{Hooks: config.Default().Hooks, CI: config.Default().CI}
	if root.Hooks != nil {
		cfg.Hooks = *root.Hooks
	}
	if root.CI != nil {
		cfg.CI = *root.CI
	}

	if doHooks {
		hooks, err := wiring.InstallHooks(wd, cfg)
		if err != nil {
			return err
		}
		if len(hooks) > 0 {
			fmt.Fprintf(out, "✓ installed git hooks: %s\n", strings.Join(hooks, ", "))
		}
	}
	if doCI {
		ci, err := wiring.GenerateCI(wd, cfg)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "✓ generated CI workflow: %s\n", ci)
	}

	fmt.Fprintln(out, "\nNext steps:")
	fmt.Fprintln(out, "  • Review cairn.yaml and adjust standards/toggles")
	fmt.Fprintln(out, "  • Run `cairn verify` to check the repo")
	fmt.Fprintln(out, "  • Commit cairn.yaml, .cairn/hooks/, and the CI workflow")
	return nil
}

// writeInitConfig writes the schema-2 cairn.yaml for the assembled baseline unless one already
// exists (which it leaves untouched), reporting what it recorded. Both the non-interactive and
// wizard paths build a `base` and hand it here, keeping the write + existing-file guard in one
// place.
func writeInitConfig(wd string, base config.Directory, dirs map[string]config.Directory, langs []string, out io.Writer) error {
	cfgPath := filepath.Join(wd, "cairn.yaml")
	switch _, statErr := os.Stat(cfgPath); {
	case errors.Is(statErr, fs.ErrNotExist):
		data, err := config.InitConfig(base, dirs, initComments())
		if err != nil {
			return err
		}
		if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
			return err
		}
		reportInit(out, base, langs)
		return nil
	case statErr != nil:
		return statErr
	default:
		fmt.Fprintln(out, "⊘ cairn.yaml exists — keeping it")
		return nil
	}
}

// initComments builds the header comment for each top-level block of the generated cairn.yaml, so
// the file explains why every row is there and what else it accepts — the option lists pulled from
// the live registries (conventions, changelog standards, CI providers, versioning schemes) so a
// newly registered standard shows up here with no edit. Keyed by the YAML key the comment sits
// above.
func initComments() map[string]string {
	or := func(opts []string) string { return strings.Join(opts, " | ") }
	return map[string]string{
		"schema":  "cairn.yaml schema version — do not edit.",
		"version": "Repo baseline version. `cairn bump` advances it; a sub-package can pin its own under `directories:`.",
		"languages": "Languages Cairn runs here (all detected ones are enabled). Per language you may set:\n" +
			"  enabled: <bool>, standard: <name>, strict: <bool> (promote that language's warnings to failures).",
		"verify":       "Verify stages and the repo-wide strict default (strict: promote warnings to failures).",
		"version_sync": "Docs whose version mentions `cairn verify` keeps honest. Patterns use {VERSION} as the placeholder.",
		"commits": "Commit-message policy the commit-msg hook enforces.\n" +
			"  convention: " + or(commit.Conventions()) + " · signoff: require a DCO Signed-off-by trailer.",
		"changelog": "Changelog format `cairn bump` promotes. standard: " + or(changelog.Standards()) + ".\n" +
			"  file defaults to " + changelogDoc + " (set it only to override).",
		"hooks": "Git hooks Cairn installs; each stage runs the listed `cairn` subcommands.",
		"ci":    "CI workflow Cairn generates. provider: " + or(wiring.Providers()) + " · jobs run the listed `cairn` subcommands.",
		"directories": "Per-directory overrides. Each path may set its own version, languages, changelog,\n" +
			"  versioning (" + or(versioningSchemes) + "), strictness, or `enabled: false` to prune the subtree.",
	}
}

// reportInit prints the one-line summary of what init detected and recorded.
func reportInit(out io.Writer, base config.Directory, langs []string) {
	if len(langs) == 0 {
		fmt.Fprintln(out, "✓ wrote cairn.yaml (no languages detected)")
	} else {
		fmt.Fprintf(out, "✓ wrote cairn.yaml (%s)\n", strings.Join(langs, ", "))
	}
	if base.VersionSync != nil {
		n := len(base.VersionSync.Files[0].Patterns)
		fmt.Fprintf(out, "  • version_sync: %d pattern(s) found in %s\n", n, versionSyncDoc)
	}
	if base.Commits != nil {
		fmt.Fprintf(out, "  • commits: %s convention", base.Commits.Convention)
		if base.Commits.Signoff {
			fmt.Fprint(out, ", sign-off enforced (your history signs off)")
		}
		fmt.Fprintln(out)
	}
	if base.Changelog != nil {
		fmt.Fprintf(out, "  • changelog: %s (%s)\n", base.Changelog.Standard, base.Changelog.File)
	}
}

// languageNames returns the unique detected language names, sorted — detection may report the
// same language in several dirs, but init enables it once.
func languageNames(res *detect.Result) []string {
	seen := map[string]bool{}
	var names []string
	for _, l := range res.Languages {
		if !seen[l.Name] {
			seen[l.Name] = true
			names = append(names, l.Name)
		}
	}
	sort.Strings(names)
	return names
}

// fallbackInitVersion is the placeholder used only when init cannot determine the repo version with
// confidence and has no one to ask (the `--yes` path). The wizard never silently writes it — it
// asks instead — so a guessed version never lands without the user seeing it.
const fallbackInitVersion = "0.1.0"

// detectBaselineVersion determines the repo baseline version *with confidence*, returning ok=false
// when it cannot. A root-level (".") manifest's version is authoritative. Failing that — a monorepo
// whose root carries no version-bearing manifest (e.g. a Dart pub workspace's `workspace:` pubspec)
// — it accepts the sub-packages' version only when *several* packages declare the same one: a
// version shared across the workspace is plainly the project's release line, whereas a lone
// sub-package's version is just that package's (it must not leak into the repo baseline — it is
// recorded separately as a per-directory override) and divergent versions mean there is no single
// baseline to infer. In those cases the caller must ask (wizard) or fall back (--yes). It never
// guesses a number the repo doesn't state.
func detectBaselineVersion(fsys fs.FS, res *detect.Result) (string, bool) {
	var rootUnits []versioning.ManifestUnit
	for _, l := range res.Languages {
		if l.Dir == "." || l.Dir == "" {
			rootUnits = append(rootUnits, versioning.ManifestUnit{Dir: ".", Manifests: l.VersionManifests})
		}
	}
	sort.Slice(rootUnits, func(i, j int) bool {
		return strings.Join(rootUnits[i].Manifests, ",") < strings.Join(rootUnits[j].Manifests, ",")
	})
	if v, ok := versioning.DetectVersion(fsys, rootUnits); ok {
		return v, true
	}

	// Versions declared by sub-packages, deduped by directory so two languages in one dir count once.
	byDir := map[string]string{}
	for _, l := range res.Languages {
		if l.Dir == "." || l.Dir == "" {
			continue
		}
		if v, ok := versioning.DetectVersion(fsys, []versioning.ManifestUnit{{Dir: l.Dir, Manifests: l.VersionManifests}}); ok {
			byDir[l.Dir] = v
		}
	}
	// A version shared across two or more packages is the repo's release line. With several declared
	// versions we adopt the *dominant* one (the most common), so a workspace where most packages move
	// in lockstep but a reference port lags (e.g. dart packages at 0.1.2 alongside a java mirror at
	// 0.3.1) still seeds the baseline most packages agree on. A lone package's version is not enough
	// to define the baseline (it rides separately as a per-directory override).
	if len(byDir) >= 2 {
		if v, ok := dominantVersion(byDir); ok {
			return v, true
		}
	}
	return "", false
}

// dominantVersion returns the most frequently declared version across the packages, breaking ties by
// the greatest version string (deterministic, and prefers the newer line). ok is false only when no
// version was declared.
func dominantVersion(byDir map[string]string) (string, bool) {
	counts := map[string]int{}
	for _, v := range byDir {
		counts[v]++
	}
	best, bestN := "", 0
	for v, n := range counts {
		if n > bestN || (n == bestN && v > best) {
			best, bestN = v, n
		}
	}
	return best, bestN > 0
}

// initVersion seeds the `--yes` path's repo baseline version from the project's real one (see
// detectBaselineVersion), falling back to the placeholder only when no confident version exists and
// there is no terminal to ask on.
func initVersion(fsys fs.FS, res *detect.Result) string {
	if v, ok := detectBaselineVersion(fsys, res); ok {
		return v
	}
	return fallbackInitVersion
}

// versionSyncDoc is the doc init scans for version occurrences. README is where a project
// advertises its version (badges, install snippets, coordinates), so it is the highest-value
// version_sync target to wire up automatically.
const versionSyncDoc = "README.md"

// discoverConfig assembles the schema-2 baseline `cairn init` writes by *detecting* the repo's
// facts rather than emitting defaults: the real project version, the languages present, the
// version_sync patterns the README actually contains, and the commit policy history implies.
// Every field it cannot positively determine is left unset so it rides the in-code default.
func discoverConfig(wd string, fsys fs.FS, res *detect.Result) config.Directory {
	v := initVersion(fsys, res)
	base := config.Directory{Version: &v}
	// Hooks and CI ride the in-code default, but init writes them explicitly so the config shows
	// what it installs/generates (the hooks the commit-msg/pre-commit gates run, the CI jobs) —
	// otherwise they'd be installed with no row to see or edit. The wizard overrides CI with the
	// chosen provider.
	hooks := config.Default().Hooks
	base.Hooks = &hooks
	ci := config.Default().CI
	base.CI = &ci
	if langs := detectLanguages(res); langs != nil {
		base.Languages = langs
	}
	if vs := detectVersionSync(fsys, v); vs != nil {
		base.VersionSync = vs
	}
	if c := detectCommits(wd); c != nil {
		base.Commits = c
	}
	if cl := detectChangelog(fsys); cl != nil {
		base.Changelog = cl
	}
	return base
}

// unit is one sub-directory detection found below the repo root: a candidate for its own rule set
// (independent version, overridden standards, disablement). The root (".") is the baseline and is
// never a unit here.
type unit struct {
	Dir     string   // repo-relative directory
	Langs   []string // languages detected there, sorted
	Version string   // version its manifest declares, "" when none does
}

// baselineVersion is the repo baseline's version string, "" when none is seeded.
func baselineVersion(baseline config.Directory) string {
	if baseline.Version != nil {
		return *baseline.Version
	}
	return ""
}

// defaultVersioning is the scheme a directory inherits when no layer sets one — config seeds the
// baseline with semver, so init compares against the same default before recording an override.
const defaultVersioning = "semver"

// baselineVersioning is the repo baseline's versioning scheme, falling back to the in-code default
// so a directory's scheme is only recorded when it genuinely differs.
func baselineVersioning(baseline config.Directory) string {
	if baseline.Versioning != nil {
		return *baseline.Versioning
	}
	return defaultVersioning
}

// baselineChangelogStandard is the repo baseline's changelog standard, falling back to the in-code
// default — the value a per-directory changelog must differ from before init records an override.
func baselineChangelogStandard(baseline config.Directory) string {
	if baseline.Changelog != nil && baseline.Changelog.Standard != "" {
		return baseline.Changelog.Standard
	}
	return config.Default().Changelog.Standard
}

// detectDirOverride builds the per-directory override block init records for a sub-unit from
// detectable *facts*: a CHANGELOG in a different format than the repo baseline (the directory keeps
// its own promotion style — exactly the per-package pub.dev style a Dart workspace wants) and a
// manifest version that differs from the baseline (the directory is independently versioned).
// Decisions a repo can't detect — disabling a subtree, a strictness or scheme override — are left
// to the wizard. An all-inherit unit yields the zero block (dirIsEmpty == true).
func detectDirOverride(fsys fs.FS, u unit, baseline config.Directory) config.Directory {
	var d config.Directory
	if u.Version != "" && u.Version != baselineVersion(baseline) {
		v := u.Version
		d.Version = &v
	}
	if sub, err := fs.Sub(fsys, u.Dir); err == nil {
		if cl := detectChangelog(sub); cl != nil && cl.Standard != baselineChangelogStandard(baseline) {
			d.Changelog = cl
		}
	}
	return d
}

// detectDirOverrides assembles the `directories:` map the non-interactive (`--yes`) path writes:
// the detected override block for every sub-unit that differs from the baseline. A repo whose
// sub-units all inherit yields nil, keeping a single-package config a flat baseline.
func detectDirOverrides(fsys fs.FS, units []unit, baseline config.Directory) map[string]config.Directory {
	dirs := map[string]config.Directory{}
	for _, u := range units {
		if d := detectDirOverride(fsys, u, baseline); !dirIsEmpty(d) {
			dirs[u.Dir] = d
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	return dirs
}

// dirIsEmpty reports whether an override block sets nothing — a directory that fully inherits the
// baseline, so init writes no `directories.<path>` entry for it.
func dirIsEmpty(d config.Directory) bool {
	return reflect.DeepEqual(d, config.Directory{})
}

// discoverUnits groups detection's per-directory findings into the sub-units the wizard can give
// their own rules — every detected directory except the root, sorted by path, each carrying the
// languages found there and the version its manifest declares (so an independently-versioned
// member defaults to its real version rather than a placeholder).
func discoverUnits(fsys fs.FS, res *detect.Result) []unit {
	byDir := map[string]*unit{}
	var order []string
	for _, l := range res.Languages {
		if l.Dir == "." || l.Dir == "" {
			continue
		}
		u, ok := byDir[l.Dir]
		if !ok {
			u = &unit{Dir: l.Dir}
			byDir[l.Dir] = u
			order = append(order, l.Dir)
		}
		u.Langs = append(u.Langs, l.Name)
		if u.Version == "" {
			if v, ok := versioning.DetectVersion(fsys, []versioning.ManifestUnit{{Dir: l.Dir, Manifests: l.VersionManifests}}); ok {
				u.Version = v
			}
		}
	}
	sort.Strings(order)
	units := make([]unit, 0, len(order))
	for _, d := range order {
		u := byDir[d]
		sort.Strings(u.Langs)
		units = append(units, *u)
	}
	return units
}

// changelogDoc is the standard changelog path init inspects — the same default `cairn bump`
// promotes, so a recorded block points bump at the file it already found.
const changelogDoc = "CHANGELOG.md"

// detectChangelog records the changelog block when the repo carries a changelog Cairn
// recognises, so the config states the format `cairn bump` will promote instead of leaning on a
// silent default. It reads the file at the standard path and asks the changelog registry to
// identify the format; an absent or unrecognised file yields nil — a discovered fact or nothing.
// Both fields are always set: config resolves changelog as a unit, so a partial block would drop
// one.
func detectChangelog(fsys fs.FS) *config.Changelog {
	data, err := fs.ReadFile(fsys, changelogDoc)
	if err != nil {
		return nil
	}
	std, ok := changelog.Detect(data)
	if !ok {
		return nil
	}
	return &config.Changelog{Standard: std, File: changelogDoc}
}

// detectLanguages records the languages detection found present, each enabled, as an editable
// scaffold for their tool/standard knobs. Listing a present language is a fact, not a default;
// a language with no entry still runs (detection owns enablement), so the list never blocks a
// language added later. Returns nil when none were found.
func detectLanguages(res *detect.Result) map[string]config.Language {
	langs := map[string]config.Language{}
	for _, l := range res.Languages {
		langs[l.Name] = config.Language{Enabled: true}
	}
	if len(langs) == 0 {
		return nil
	}
	return langs
}

// pruneDisabledLanguages drops from the repo-wide language list any language whose only home is a
// disabled directory subtree, so init never enables a language at the baseline that nothing
// reachable actually uses (e.g. java present solely under a `reference/…` directory the wizard
// disabled). A language detected at the root or in any still-enabled directory is kept. Returns nil
// when nothing survives. Detection can't decide disablement, so this only ever changes the wizard's
// output (the `--yes` path disables nothing).
func pruneDisabledLanguages(langs map[string]config.Language, res *detect.Result, dirs map[string]config.Directory) map[string]config.Language {
	if len(langs) == 0 {
		return langs
	}
	disabled := disabledDirs(dirs)
	live := map[string]bool{}
	for _, l := range res.Languages {
		if !inDisabledSubtree(l.Dir, disabled) {
			live[l.Name] = true
		}
	}
	out := map[string]config.Language{}
	for name, v := range langs {
		if live[name] {
			out[name] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// disabledDirs collects the repo-relative paths of directory overrides explicitly turned off
// (enabled: false) — the pruned subtree roots a language must fall outside of to count as live.
func disabledDirs(dirs map[string]config.Directory) []string {
	var off []string
	for d, o := range dirs {
		if o.Enabled != nil && !*o.Enabled {
			off = append(off, d)
		}
	}
	return off
}

// inDisabledSubtree reports whether dir sits at or beneath any disabled directory, so a language
// found only there is not enabled repo-wide.
func inDisabledSubtree(dir string, disabled []string) bool {
	for _, d := range disabled {
		if dir == d || strings.HasPrefix(dir, d+"/") {
			return true
		}
	}
	return false
}

// enabledLanguageNames returns the sorted names of the languages recorded on the baseline — what
// init actually enabled repo-wide, for the one-line summary (so a language pruned because its only
// home is disabled is not reported as enabled).
func enabledLanguageNames(langs map[string]config.Language) []string {
	names := make([]string, 0, len(langs))
	for name := range langs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// detectVersionSync scans the README for the places the project version actually appears and
// wires them up as version_sync patterns, so `cairn verify` keeps those docs honest from the
// first run. It reads nothing into the config when the README is absent or carries no
// distinctive version occurrence — a discovered fact or nothing, never a guess.
func detectVersionSync(fsys fs.FS, version string) *config.VersionSync {
	data, err := fs.ReadFile(fsys, versionSyncDoc)
	if err != nil {
		return nil
	}
	pats := versioning.DetectSyncPatterns(string(data), version)
	if len(pats) == 0 {
		return nil
	}
	return &config.VersionSync{Files: []config.VersionSyncFile{{Path: versionSyncDoc, Patterns: pats}}}
}

// signoffMajority is the share of history that must carry a DCO trailer before init treats
// sign-off as the repo's norm. DCO projects sign every commit, so a simple majority tolerates
// a few unsigned merge/vendor commits without enabling sign-off for a repo that merely has a
// handful.
const signoffMajority = 0.5

// conventionalMajority is the share of non-merge commits that must conform to a convention
// before init records it. A handful of stragglers — a pre-adoption commit, a hand-edited hotfix
// — shouldn't mask a history that plainly follows the convention.
const conventionalMajority = 0.7

// detectCommits learns the commit policy to record from the repo's existing history instead of
// writing a blind default: the convention its messages follow and whether DCO sign-off is the
// norm. It records a commits block when it can positively determine the convention (a strong
// majority of non-merge commits conform to a registered validator) or when sign-off is the norm.
// An unrecognisable, sign-off-less history — or no history at all — yields nil so the block is
// omitted and the in-code default applies. The block is always written *complete* (convention,
// signoff, validate_hook) because config resolves commits as a unit, so a partial block would
// drop a field; validate_hook stays on so the commit-msg gate enforces the recorded convention.
func detectCommits(wd string) *config.Commits {
	history := gitLogMessages(wd, "", "")
	if len(history) == 0 {
		return nil
	}
	convention, known := detectConvention(history)
	signoff := signedOffIsNorm(history)
	if !known && !signoff {
		return nil
	}
	if !known {
		// Sign-off is the norm but the convention is unclear; record the default so the unit is
		// complete rather than claiming a convention the history doesn't back.
		convention = config.Default().Commits.Convention
	}
	return &config.Commits{Convention: convention, Signoff: signoff, ValidateHook: true}
}

// detectConvention reports the commit convention the history follows, when a strong majority of
// its non-merge commits conform to a registered validator. conventional is the only validator
// today, so it is the sole convention detectable now; an unrecognisable history yields ok=false
// and init records no convention (the in-code default applies, unwritten).
func detectConvention(history []string) (string, bool) {
	for _, name := range commit.Conventions() {
		v, ok := commit.ValidatorFor(name)
		if !ok {
			continue
		}
		total, conform := 0, 0
		for _, m := range history {
			if isMergeCommit(m) {
				continue
			}
			total++
			if v.Validate(m, false) == nil {
				conform++
			}
		}
		if total > 0 && float64(conform) >= conventionalMajority*float64(total) {
			return name, true
		}
	}
	return "", false
}

// signedOffIsNorm reports whether DCO sign-off is the repo's norm — at least signoffMajority of
// commits carry a Signed-off-by trailer. history must be non-empty.
func signedOffIsNorm(history []string) bool {
	signed := 0
	for _, m := range history {
		if commit.IsSignedOff(m) {
			signed++
		}
	}
	return float64(signed) >= signoffMajority*float64(len(history))
}

// isMergeCommit reports whether msg is a default git merge commit ("Merge branch …", "Merge pull
// request …"), which never follows a commit convention and so must not count against one when
// init samples history for convention detection.
func isMergeCommit(msg string) bool {
	return strings.HasPrefix(strings.TrimSpace(msg), "Merge ")
}
