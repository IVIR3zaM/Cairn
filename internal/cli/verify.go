package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/IVIR3zaM/Cairn/internal/quality"
	"github.com/IVIR3zaM/Cairn/internal/report"
	"github.com/IVIR3zaM/Cairn/internal/runner"
	versioning "github.com/IVIR3zaM/Cairn/internal/version"
	"github.com/spf13/cobra"
)

// smartExec wraps runner.Exec to resolve tool paths using custom lookup first, and (for
// --verbose) announces and tees each tool's live output so you can see exactly what runs.
type smartExec struct {
	lookupTool func(string) (string, error)
	stream     io.Writer
	announce   func(runner.Command) // called with the resolved command before it runs
}

func (s smartExec) Run(ctx context.Context, cmd runner.Command) (runner.Result, error) {
	// Try to resolve the tool path using custom lookup
	if resolved, err := s.lookupTool(cmd.Name); err == nil {
		cmd.Name = resolved
	}
	if s.stream != nil {
		cmd.Stream = s.stream
	}
	if s.announce != nil {
		s.announce(cmd)
	}
	// Fall back to standard Exec
	return runner.Exec{}.Run(ctx, cmd)
}

// liveObserver renders each stage as it runs — a spinner with elapsed time on a TTY, so
// a long tool never looks frozen — and collects the step lines for the final summary.
type liveObserver struct {
	rep   report.Reporter
	done  func(report.Step)
	steps []report.Step
}

// stepName labels a stage; it appends the unit's directory when it isn't the repo root,
// so same-language units in different parts of the repo are distinguishable.
func stepName(lang, dir string, k quality.Kind) string {
	name := lang + " · " + k.String()
	if dir != "" && dir != "." {
		name += " (" + dir + ")"
	}
	return name
}

func (o *liveObserver) Begin(unit quality.LangUnit, k quality.Kind) {
	o.done = o.rep.Running(stepName(unit.Name, unit.Dir, k))
}

func (o *liveObserver) End(r quality.Result) {
	s := report.Step{
		Name: stepName(r.Lang, r.Dir, r.Kind), Status: toStatus(r.Status), Detail: r.Detail,
		Fix: r.Fix, FixPartial: r.FixPartial, FixApplied: r.FixApplied,
	}
	if o.done != nil {
		o.done(s) // resolve the running indicator started in Begin
		o.done = nil
	} else {
		o.rep.Step(s) // a missing-tool result has no running phase
	}
	o.steps = append(o.steps, s)
}

// lookupTool extends exec.LookPath to also check GOPATH/bin and GOBIN, where Go tools
// are commonly installed but may not be in the shell's PATH.
func lookupTool(name string) (string, error) {
	// First try standard PATH lookup
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	// Check GOBIN if set
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		candidate := filepath.Join(gobin, name)
		if isExecutable(candidate) {
			return candidate, nil
		}
	}

	// Check GOPATH/bin (from env or queried from go)
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Query go env GOPATH if not set
		out, err := exec.Command("go", "env", "GOPATH").Output()
		if err == nil && len(out) > 0 {
			gopath = strings.TrimSpace(string(out))
		}
	}

	if gopath != "" {
		candidate := filepath.Join(gopath, "bin", name)
		if isExecutable(candidate) {
			return candidate, nil
		}
	}

	// Fallback to standard lookup error
	return exec.LookPath(name)
}

// isExecutable checks if a file exists and is executable.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// On Unix, check if executable bit is set; on Windows, just check it exists
	return info.Mode()&0o111 != 0 || os.Getenv("GOOS") == "windows"
}

func newVerifyCmd() *cobra.Command {
	var quiet, verbose, fix bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run the configured quality gate (format, lint, test, …) for each language",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			fsys := os.DirFS(wd)
			// config owns the per-directory cascade: verify asks the Tree for each unit's
			// resolved settings (languages standard/strict, verify toggles, version_sync,
			// enabled gate, target version) and never re-derives precedence itself.
			tree, err := config.LoadTree(fsys)
			if err != nil {
				return err
			}
			res, err := detect.Detect(fsys, lookupTool)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			opts := report.Detect(out, quiet, verbose)
			rep := report.New(out, opts)
			rep.Start("cairn verify")

			run := smartExec{lookupTool: lookupTool}
			if verbose {
				// Under --verbose, print the exact command (with its directory) and stream
				// the tool's live output — the visibility CI runs and debugging need.
				run.stream = out
				run.announce = func(c runner.Command) {
					dir := c.Dir
					if dir == "" {
						dir = "."
					}
					fmt.Fprintf(out, "  $ cd %s && %s %s\n", dir, c.Name, strings.Join(c.Args, " "))
				}
			}
			obs := &liveObserver{rep: rep}
			var all []quality.Result
			for _, lang := range res.Languages {
				resolved, active := tree.Resolve(lang.Dir)
				if !active {
					continue // directory pruned by the absolute disable gate
				}
				standard := ""
				if l, ok := resolved.Languages[lang.Name]; ok {
					standard = l.Standard
				}
				adapter, ok := quality.AdapterFor(lang.Name, run, standard)
				if !ok {
					continue // no adapter registered for this language yet
				}
				if l, ok := resolved.Languages[lang.Name]; ok && !l.Enabled {
					continue // explicitly disabled in cairn.yaml
				}
				results := quality.Run(context.Background(), resolved.VerifyOrDefault(), adapter,
					// Force tool color only when streaming to a color TTY (verbose); piped or
					// NO_COLOR runs stay clean so captured output never carries escape codes.
					quality.LangUnit{Name: lang.Name, Dir: lang.Dir, Color: verbose && opts.Color, Strict: resolved.StrictFor(lang.Name), Fix: fix},
					toolInfo(lang), obs)
				all = append(all, results...)
			}

			// version_sync honesty check (Cairn's signature): every documented version must
			// quote the canonical one. A drift fails verify just like a quality stage.
			syncFailed, err := checkVersionSync(fsys, tree, rep, obs)
			if err != nil {
				rep.Error(err)
				return errSilent
			}

			// Same honesty check, language-owned: every detected manifest (Cargo.toml,
			// package.json, pyproject.toml, pubspec.yaml, …) must state the canonical
			// version — drift in the files bump writes fails verify, no version_sync needed.
			manifestsFailed, err := checkManifestSync(fsys, tree, res, rep, obs)
			if err != nil {
				rep.Error(err)
				return errSilent
			}

			rep.Summary(obs.steps)

			if quality.Failed(all) || syncFailed || manifestsFailed {
				return errSilent
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print the summary")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show full tool output")
	cmd.Flags().BoolVar(&fix, "fix", false, "auto-fix what each language's tools can repair (format, fixable lints) before reporting")
	return cmd
}

// checkVersionSync runs the non-mutating doc-honesty assertion and renders it as one
// report step appended after the language stages. It returns whether any doc drifted (so
// verify exits non-zero) and surfaces config/read errors. With no version_sync configured
// it adds no step at all, keeping the summary clean for projects that don't use it.
func checkVersionSync(fsys fs.FS, tree *config.Tree, rep report.Reporter, obs *liveObserver) (bool, error) {
	root, _ := tree.Resolve(".")
	var files []config.VersionSyncFile
	if root.VersionSync != nil {
		files = root.VersionSync.Files
	}
	if len(files) == 0 {
		return false, nil
	}
	res := versioning.NewResolverFromTree(tree)
	drifts, err := versioning.Check(fsys, res, files)
	if err != nil {
		return false, err
	}
	step := report.Step{Name: "version-sync", Status: report.Pass}
	if len(drifts) > 0 {
		reasons := make([]string, len(drifts))
		for i, d := range drifts {
			reasons[i] = d.Reason()
		}
		step.Status = report.Fail
		step.Detail = strings.Join(reasons, "\n") + versionFixHint(rootVersion(root))
	}
	rep.Step(step)
	obs.steps = append(obs.steps, step)
	return step.Status == report.Fail, nil
}

// rootVersion is the repo baseline version (the `.` resolution), used to name the resync
// command in a drift hint. Empty when no baseline version is configured.
func rootVersion(root config.Directory) string {
	if root.Version != nil {
		return *root.Version
	}
	return ""
}

// versionFixHint points a version drift at the command that rewrites every doc and manifest
// back to canonical. Unlike the language stages, --fix does not resync versions (it has no
// target to write), so the hint names `cairn bump` explicitly rather than `verify --fix`.
func versionFixHint(canonical string) string {
	return fmt.Sprintf("\n↳ resync: run `cairn bump %s` to rewrite every file back to canonical", canonical)
}

// checkManifestSync runs the non-mutating honesty assertion over language-owned manifests:
// every detected unit's declared manifest (resolved from detection, not cairn.yaml) must
// state the canonical version. It renders one "version-manifests" step, returns whether any
// manifest drifted, and surfaces config errors. When nothing version-bearing is found (no
// canonical, or a repo whose languages own no writable manifest) it adds no step, keeping
// the summary clean for projects this doesn't apply to.
func checkManifestSync(fsys fs.FS, tree *config.Tree, res *detect.Result, rep report.Reporter, obs *liveObserver) (bool, error) {
	units := make([]versioning.ManifestUnit, 0, len(res.Languages))
	for _, lang := range res.Languages {
		units = append(units, versioning.ManifestUnit{Dir: lang.Dir, Manifests: lang.VersionManifests})
	}
	// The Tree-backed resolver answers each unit's target version from the cascade; a unit
	// with no version resolves to empty and is skipped, so an unversioned repo asserts nothing.
	resolver := versioning.NewResolverFromTree(tree)
	drifts, checked, err := versioning.CheckManifests(fsys, resolver, units)
	if err != nil {
		return false, err
	}
	// Multi-package repos also carry member-to-member constraints (a workspace/reactor pinning
	// a sibling at `^X.Y.Z`) that must track each member's version — a stale pin looks honest to
	// the per-file version: check above. The workspace pass catches it generically: any manifest
	// format that opts into version.Workspace participates, no language named here.
	wsDrifts, err := versioning.CheckWorkspace(fsys, resolver, units)
	if err != nil {
		return false, err
	}
	drifts = append(drifts, wsDrifts...)
	if checked == 0 && len(drifts) == 0 {
		return false, nil // no language-owned manifest present — nothing to assert
	}
	step := report.Step{Name: "version-manifests", Status: report.Pass}
	if len(drifts) > 0 {
		reasons := make([]string, len(drifts))
		for i, d := range drifts {
			reasons[i] = d.Reason()
		}
		root, _ := tree.Resolve(".")
		step.Status = report.Fail
		step.Detail = strings.Join(reasons, "\n") + versionFixHint(rootVersion(root))
	}
	rep.Step(step)
	obs.steps = append(obs.steps, step)
	return step.Status == report.Fail, nil
}

// toolInfo turns a detected language's tool statuses into the gate's lookup map.
func toolInfo(l detect.Language) map[string]quality.ToolInfo {
	m := make(map[string]quality.ToolInfo, len(l.Tools))
	for _, t := range l.Tools {
		m[t.Tool.Name] = quality.ToolInfo{Installed: t.Installed, Hint: t.Tool.Hint}
	}
	return m
}

func toStatus(s quality.Status) report.Status {
	switch s {
	case quality.StatusPass:
		return report.Pass
	case quality.StatusFail:
		return report.Fail
	case quality.StatusWarn:
		return report.Warn
	default:
		return report.Skip
	}
}
