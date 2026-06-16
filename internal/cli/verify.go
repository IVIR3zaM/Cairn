package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/config"
	"github.com/IVIR3zaM/Cairn/internal/detect"
	"github.com/IVIR3zaM/Cairn/internal/quality"
	"github.com/IVIR3zaM/Cairn/internal/report"
	"github.com/IVIR3zaM/Cairn/internal/runner"
	"github.com/spf13/cobra"
)

// errVerifyFailed makes verify exit non-zero. The compact summary already explains
// what failed, so the message itself stays silent (root sets SilenceErrors).
var errVerifyFailed = errors.New("verify failed")

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
	s := report.Step{Name: stepName(r.Lang, r.Dir, r.Kind), Status: toStatus(r.Status), Detail: r.Detail}
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
	var quiet, verbose bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Run the configured quality gate (format, lint, test, …) for each language",
		RunE: func(cmd *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := config.LoadOrDefault("cairn.yaml")
			if err != nil {
				return err
			}
			res, err := detect.Detect(os.DirFS(wd), lookupTool)
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
				standard := ""
				if l, ok := cfg.Languages[lang.Name]; ok {
					standard = l.Standard
				}
				adapter, ok := quality.AdapterFor(lang.Name, run, standard)
				if !ok {
					continue // no adapter registered for this language yet
				}
				if l, ok := cfg.Languages[lang.Name]; ok && !l.Enabled {
					continue // explicitly disabled in cairn.yaml
				}
				results := quality.Run(context.Background(), cfg.Verify, adapter,
					// Force tool color only when streaming to a color TTY (verbose); piped or
					// NO_COLOR runs stay clean so captured output never carries escape codes.
					quality.LangUnit{Name: lang.Name, Dir: lang.Dir, Color: verbose && opts.Color},
					toolInfo(lang), obs)
				all = append(all, results...)
			}

			rep.Summary(obs.steps)

			if quality.Failed(all) {
				return errVerifyFailed
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print the summary")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show full tool output")
	return cmd
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
