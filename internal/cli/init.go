package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
			"an existing cairn.yaml. Run with --yes to accept the smart defaults non-interactively; " +
			"the guided wizard lands in a later iteration.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			if !yes {
				return errors.New("interactive init is not available yet; re-run with --yes to accept smart defaults")
			}
			return runInit(wd, cmd.OutOrStdout())
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

	if err := writeInitConfig(wd, res, out); err != nil {
		return err
	}

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

	hooks, err := wiring.InstallHooks(wd, cfg)
	if err != nil {
		return err
	}
	if len(hooks) > 0 {
		fmt.Fprintf(out, "✓ installed git hooks: %s\n", strings.Join(hooks, ", "))
	}

	ci, err := wiring.GenerateCI(wd, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "✓ generated CI workflow: %s\n", ci)

	fmt.Fprintln(out, "\nNext steps:")
	fmt.Fprintln(out, "  • Review cairn.yaml and adjust standards/toggles")
	fmt.Fprintln(out, "  • Run `cairn verify` to check the repo")
	fmt.Fprintln(out, "  • Commit cairn.yaml, .cairn/hooks/, and the CI workflow")
	return nil
}

// writeInitConfig writes the discovered schema-2 cairn.yaml unless one already exists (which it
// leaves untouched), reporting what it detected. Splitting it out of runInit keeps the
// onboarding flow flat: discovery + write here, wiring in the caller.
func writeInitConfig(wd string, res *detect.Result, out io.Writer) error {
	cfgPath := filepath.Join(wd, "cairn.yaml")
	switch _, statErr := os.Stat(cfgPath); {
	case errors.Is(statErr, fs.ErrNotExist):
		base := discoverConfig(wd, os.DirFS(wd), res)
		data, err := config.InitConfig(base)
		if err != nil {
			return err
		}
		if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
			return err
		}
		reportInit(out, base, languageNames(res))
		return nil
	case statErr != nil:
		return statErr
	default:
		fmt.Fprintln(out, "⊘ cairn.yaml exists — keeping it")
		return nil
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

// initVersion seeds the new cairn.yaml's version from the project's real one: it reads the
// version declared in a detected language manifest (a pom's <version>, a package.json's
// "version", a Cargo.toml/pyproject.toml/pubspec.yaml version, …) so `cairn verify` agrees
// out of the box instead of drifting against a placeholder. It falls back to "0.1.0" when no
// manifest declares a version (a fresh repo, or a language with no writable manifest yet).
func initVersion(fsys fs.FS, res *detect.Result) string {
	units := make([]versioning.ManifestUnit, 0, len(res.Languages))
	for _, l := range res.Languages {
		units = append(units, versioning.ManifestUnit{Dir: l.Dir, Manifests: l.VersionManifests})
	}
	if v, ok := versioning.DetectVersion(fsys, units); ok {
		return v
	}
	return "0.1.0"
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