package cli

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/changelog"
	"github.com/IVIR3zaM/Cairn/internal/commit"
	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/IVIR3zaM/Cairn/internal/wiring"
)

// isInteractive reports whether r is a terminal we can prompt on — a char-device *os.File. When
// false (a pipe, a test buffer, CI), init refuses the wizard and points at --yes instead, so the
// guided flow never blocks on input that will never come.
func isInteractive(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// runInitWizard is the guided onboarding: detect the repo's facts, show the findings, let the user
// confirm or change the standards (each menu sourced from a registry, so a newly registered
// standard appears with no edit here), then write the config and run Wiring through the shared
// tail. It builds on the same discovered `base` as `--yes` and only overlays the user's choices,
// so the file stays facts-plus-decisions, never a wall of defaults.
func runInitWizard(wd string, in io.Reader, out io.Writer) error {
	res, err := detect.Detect(os.DirFS(wd), exec.LookPath)
	if err != nil {
		return err
	}
	base := discoverConfig(wd, os.DirFS(wd), res)
	units := discoverUnits(os.DirFS(wd), res)

	fmt.Fprintln(out, "Cairn init — a few quick choices (Enter keeps the default).")
	printFindings(out, base, languageNames(res), units)

	p := newPrompter(in, out)
	base = chooseVersion(p, os.DirFS(wd), res, base)
	base = chooseStandards(p, base)
	dirs := chooseDirectories(p, os.DirFS(wd), units, base)
	doHooks, doCI := chooseFeatures(p)

	base.Languages = pruneDisabledLanguages(base.Languages, res, dirs)
	if err := writeInitConfig(wd, base, dirs, enabledLanguageNames(base.Languages), out); err != nil {
		return err
	}
	return finishInit(wd, out, doHooks, doCI)
}

// printFindings prints the concise "here's what I found" block: the languages detected, the seeded
// version, and which honesty signals (version_sync/commits/changelog) discovery picked up. It is
// the orienting summary the wizard's questions then refine.
func printFindings(out io.Writer, base config.Directory, langs []string, units []unit) {
	fmt.Fprintln(out, "\nFindings:")
	if len(langs) == 0 {
		fmt.Fprintln(out, "  • languages: none detected")
	} else {
		fmt.Fprintf(out, "  • languages: %s\n", strings.Join(langs, ", "))
	}
	if base.Version != nil {
		fmt.Fprintf(out, "  • version: %s\n", *base.Version)
	}
	if base.VersionSync != nil {
		fmt.Fprintf(out, "  • version_sync: %d pattern(s) in %s\n", len(base.VersionSync.Files[0].Patterns), versionSyncDoc)
	}
	if len(units) > 0 {
		fmt.Fprintf(out, "  • sub-directories: %d found — each can take its own rules\n", len(units))
		for _, u := range units {
			line := fmt.Sprintf("      %s (%s)", u.Dir, strings.Join(u.Langs, ", "))
			if u.Version != "" {
				line += " — version " + u.Version
			}
			fmt.Fprintln(out, line)
		}
	}
	fmt.Fprintln(out)
}

// versioningSchemes are the version schemes a directory can pick from — the same set the config
// validator accepts. Kept tiny and explicit (not a registry) because the scheme space is closed.
var versioningSchemes = []string{"semver", "calver"}

// chooseDirectories walks the detected sub-units and lets the user give any of them its own full
// rule set — the complete per-directory override the schema allows, not just version/disable:
// disablement, independent versioning + scheme, a different changelog standard, and per-language
// strictness. Defaults come from per-directory detection (a sub-unit that already keeps its own
// CHANGELOG format or manifest version is offered configured-on), so Enter-through preserves the
// facts `--yes` would record while the questions let the user override anything. Only directories
// that end up with a non-empty block produce a `directories.<path>` entry, so a plain monorepo
// stays a flat baseline. Returns nil when nothing was customized.
func chooseDirectories(p *prompter, fsys fs.FS, units []unit, baseline config.Directory) map[string]config.Directory {
	if len(units) == 0 {
		return nil
	}
	dirs := map[string]config.Directory{}
	for _, u := range units {
		detected := detectDirOverride(fsys, u, baseline)
		if !p.confirm(fmt.Sprintf("Configure rules for %s", u.Dir), !dirIsEmpty(detected)) {
			continue
		}
		if d := configureDirectory(p, u, detected, baseline); !dirIsEmpty(d) {
			dirs[u.Dir] = d
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	return dirs
}

// configureDirectory runs the per-directory question set for one sub-unit, seeded with what
// detection found, and returns the resulting override block. Disabling short-circuits (a pruned
// subtree needs no further rules); otherwise it offers independent versioning (+ scheme), the
// changelog standard, and per-language strictness, recording each only when it differs from the
// baseline so the block stays to the deltas.
func configureDirectory(p *prompter, u unit, detected, baseline config.Directory) config.Directory {
	if !p.confirm(fmt.Sprintf("  Enable %s", u.Dir), true) {
		off := false
		return config.Directory{Enabled: &off}
	}
	var d config.Directory

	if p.confirm(fmt.Sprintf("  Version %s independently", u.Dir), detected.Version != nil) {
		v := "0.1.0"
		switch {
		case detected.Version != nil:
			v = *detected.Version
		case u.Version != "":
			v = u.Version
		}
		d.Version = &v
		if scheme := p.selectOne(fmt.Sprintf("  Versioning scheme for %s", u.Dir), versioningSchemes, baselineVersioning(baseline)); scheme != baselineVersioning(baseline) {
			d.Versioning = &scheme
		}
	}

	clDefault := baselineChangelogStandard(baseline)
	if detected.Changelog != nil {
		clDefault = detected.Changelog.Standard
	}
	if cl := p.selectOne(fmt.Sprintf("  Changelog standard for %s", u.Dir), changelog.Standards(), clDefault); cl != baselineChangelogStandard(baseline) {
		d.Changelog = &config.Changelog{Standard: cl, File: changelogDoc}
	}

	if langs := chooseDirStrict(p, u, baseline); langs != nil {
		d.Languages = langs
	}
	return d
}

// chooseDirStrict asks, per language in the unit, whether its checks run stricter (or looser) than
// the baseline, recording a `languages.<name>.strict` override only where it differs. The language
// entry is written enabled so the strict flag never accidentally disables the language (an override
// replaces the whole per-key Language value). Returns nil when no language deviates.
func chooseDirStrict(p *prompter, u unit, baseline config.Directory) map[string]config.Language {
	baseStrict := baseline.VerifyOrDefault().Strict
	langs := map[string]config.Language{}
	for _, lang := range u.Langs {
		strict := p.confirm(fmt.Sprintf("  Strict %s checks in %s", lang, u.Dir), baseStrict)
		if strict != baseStrict {
			s := strict
			langs[lang] = config.Language{Enabled: true, Strict: &s}
		}
	}
	if len(langs) == 0 {
		return nil
	}
	return langs
}

// chooseVersion confirms the repo baseline version with the user rather than letting init silently
// commit a guess. When detection found the version with confidence (a root manifest, or a unanimous
// version across the monorepo's packages) it is offered as the default; when it did not, the prompt
// makes clear the suggested number is a placeholder the user should correct. Either way the user's
// answer wins, so a wrong version never lands unseen.
func chooseVersion(p *prompter, fsys fs.FS, res *detect.Result, base config.Directory) config.Directory {
	v, confident := detectBaselineVersion(fsys, res)
	label := "Repo version"
	if !confident {
		v = fallbackInitVersion
		label = "Repo version (couldn't detect one — set the project's actual version)"
	}
	answer := p.ask(label, v)
	base.Version = &answer
	// Re-scan the README against the chosen version: discoverConfig wired version_sync to the
	// pre-prompt guess, so a corrected version would otherwise leave the patterns matching a version
	// the project no longer uses (or none at all). Recomputing keeps the docs honest from the answer.
	base.VersionSync = detectVersionSync(fsys, answer)
	return base
}

// chooseStandards walks the user through the registry-backed standard choices — commit convention,
// changelog standard, CI provider — defaulting to what discovery found (or the in-code default) and
// recording the result on base. Each block is written complete so config resolves it as a unit.
func chooseStandards(p *prompter, base config.Directory) config.Directory {
	def := config.Default()

	convDefault := def.Commits.Convention
	signoff := false
	if base.Commits != nil {
		convDefault, signoff = base.Commits.Convention, base.Commits.Signoff
	}
	conv := p.selectOne("Commit convention", commit.Conventions(), convDefault)
	signoff = p.confirm("Enforce DCO sign-off", signoff)
	base.Commits = &config.Commits{Convention: conv, Signoff: signoff, ValidateHook: true}

	clDefault, clFile := def.Changelog.Standard, def.Changelog.File
	if base.Changelog != nil {
		clDefault, clFile = base.Changelog.Standard, base.Changelog.File
	}
	cl := p.selectOne("Changelog standard", changelog.Standards(), clDefault)
	base.Changelog = &config.Changelog{Standard: cl, File: clFile}

	provider := p.selectOne("CI provider", wiring.Providers(), def.CI.Provider)
	base.CI = &config.CI{Provider: provider, Jobs: def.CI.Jobs}

	base = chooseStrict(p, base)

	return base
}

// chooseStrict asks for the repo-wide strict default — whether verify promotes advisory
// diagnostics (analyzer infos, linter warnings) to failures across the repo. It is asked before the
// per-directory pass so directories inherit this answer as their default and only record a
// `languages.<name>.strict` override when the user deliberately deviates (Enter keeps the repo
// default — "don't overwrite"). The block is seeded from the in-code default so the stage toggles
// survive (an override replaces the whole verify unit), and is written only when strict is enabled
// so a non-strict repo stays a flat baseline riding the default.
func chooseStrict(p *prompter, base config.Directory) config.Directory {
	baseStrict := base.VerifyOrDefault().Strict
	if strict := p.confirm("Strict checks repo-wide (promote warnings to failures)", baseStrict); strict != baseStrict {
		v := config.Default().Verify
		if base.Verify != nil {
			v = *base.Verify
		}
		v.Strict = strict
		base.Verify = &v
	}
	return base
}

// chooseFeatures asks which wiring steps to perform. Declining is honored by the shared finishInit
// tail (it skips the step) rather than writing an empty block, keeping the config clean.
func chooseFeatures(p *prompter) (doHooks, doCI bool) {
	return p.confirm("Install git hooks", true), p.confirm("Generate CI workflow", true)
}

// prompter drives the wizard over a line-based reader, writing prompts to out. It is the single
// seam the tests drive with canned input — no real TTY needed.
type prompter struct {
	in  *bufio.Scanner
	out io.Writer
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	return &prompter{in: bufio.NewScanner(in), out: out}
}

// selectOne renders a numbered menu and returns the chosen option. Blank input, EOF, or an
// unrecognised entry keeps def, so the wizard always moves forward. The user may answer by number
// or by name (case-insensitive).
func (p *prompter) selectOne(label string, options []string, def string) string {
	fmt.Fprintf(p.out, "%s:\n", label)
	for i, o := range options {
		tag := ""
		if o == def {
			tag = " (default)"
		}
		fmt.Fprintf(p.out, "  %d) %s%s\n", i+1, o, tag)
	}
	fmt.Fprintf(p.out, "› [%s] ", def)

	line := p.read()
	if line == "" {
		return def
	}
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(options) {
		return options[n-1]
	}
	for _, o := range options {
		if strings.EqualFold(o, line) {
			return o
		}
	}
	return def
}

// ask prompts for a free-text value, returning the trimmed input or def on blank input/EOF.
func (p *prompter) ask(label, def string) string {
	fmt.Fprintf(p.out, "%s [%s] ", label, def)
	if line := p.read(); line != "" {
		return line
	}
	return def
}

// confirm asks a yes/no question, returning def on blank input or EOF.
func (p *prompter) confirm(label string, def bool) bool {
	hint := "Y/n"
	if !def {
		hint = "y/N"
	}
	fmt.Fprintf(p.out, "%s? [%s] ", label, hint)
	switch strings.ToLower(p.read()) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

// read returns the next trimmed input line, or "" at EOF.
func (p *prompter) read() string {
	if !p.in.Scan() {
		return ""
	}
	return strings.TrimSpace(p.in.Text())
}
