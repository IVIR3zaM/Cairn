// Package golang adapts Go's standard tools (gofumpt, golangci-lint, go test) to the
// quality Step ports. It is thin: each step shells out through a ToolRunner and maps
// the tool's exit code and output to a StepResult. No Go-specific policy lives in the
// QualityGate core.
package golang

import (
	"context"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/quality"
	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Adapter provides Go's quality steps, all sharing one ToolRunner.
type Adapter struct{ run runner.ToolRunner }

// New returns the Go quality adapter backed by run.
func New(run runner.ToolRunner) Adapter { return Adapter{run: run} }

// step is one Go stage: a kind, the tool it needs, and how to run it. Folding the
// stages into one type (rather than a struct per stage) keeps the adapter terse.
type step struct {
	tr   runner.ToolRunner
	kind quality.Kind
	tool string
	exec func(ctx context.Context, run runner.ToolRunner, unit quality.LangUnit, mode quality.Mode) quality.StepResult
}

func (s step) Kind() quality.Kind { return s.kind }
func (s step) Tool() string       { return s.tool }
func (s step) Run(ctx context.Context, unit quality.LangUnit, mode quality.Mode) quality.StepResult {
	return s.exec(ctx, s.tr, unit, mode)
}

// bind attaches the ToolRunner when Steps() builds each step, so Run needs no adapter.
func (s step) bind(r runner.ToolRunner) step { s.tr = r; return s }

// Steps lists Go's steps; the gate decides the run order and which to skip.
func (a Adapter) Steps() []quality.Step {
	return []quality.Step{
		step{kind: quality.Format, tool: "gofumpt", exec: runFormat}.bind(a.run),
		step{kind: quality.Lint, tool: "golangci-lint", exec: runLint}.bind(a.run),
		step{kind: quality.Test, tool: "go", exec: runTest}.bind(a.run),
	}
}

func runFormat(ctx context.Context, run runner.ToolRunner, unit quality.LangUnit, mode quality.Mode) quality.StepResult {
	args := []string{"-l", "."}
	if mode == quality.ModeFix {
		args = []string{"-w", "."}
	}
	res, err := run.Run(ctx, runner.Command{Name: "gofumpt", Args: args, Dir: unit.Dir})
	if r, bad := startOrExitFailure(res, err); bad {
		return r
	}
	// In check mode gofumpt exits 0 but lists unformatted files on stdout.
	if mode == quality.ModeCheck && strings.TrimSpace(res.Stdout) != "" {
		return quality.StepResult{Status: quality.StatusFail, Detail: "needs formatting:\n" + strings.TrimSpace(res.Stdout)}
	}
	return quality.StepResult{Status: quality.StatusPass}
}

func runLint(ctx context.Context, run runner.ToolRunner, unit quality.LangUnit, _ quality.Mode) quality.StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "golangci-lint", Args: []string{"run"}, Dir: unit.Dir})
	return passOrFail(res, err)
}

func runTest(ctx context.Context, run runner.ToolRunner, unit quality.LangUnit, _ quality.Mode) quality.StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "go", Args: []string{"test", "./..."}, Dir: unit.Dir})
	return passOrFail(res, err)
}

// passOrFail maps a completed run to pass (exit 0) or fail (anything else).
func passOrFail(res runner.Result, err error) quality.StepResult {
	if r, bad := startOrExitFailure(res, err); bad {
		return r
	}
	return quality.StepResult{Status: quality.StatusPass}
}

// startOrExitFailure returns a Fail result when the command could not start, timed
// out, or exited non-zero. The bool reports whether a failure was found.
func startOrExitFailure(res runner.Result, err error) (quality.StepResult, bool) {
	switch {
	case err != nil:
		return quality.StepResult{Status: quality.StatusFail, Detail: err.Error()}, true
	case res.TimedOut:
		return quality.StepResult{Status: quality.StatusFail, Detail: "timed out"}, true
	case res.ExitCode != 0:
		return quality.StepResult{Status: quality.StatusFail, Detail: output(res)}, true
	default:
		return quality.StepResult{}, false
	}
}

func output(res runner.Result) string {
	return strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
}
