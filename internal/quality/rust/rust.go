// Package rust adapts Rust's standard tools (cargo fmt, cargo clippy, cargo test) to
// the quality Step ports. Like the Go adapter it is thin: each step shells out through
// a ToolRunner and maps the tool's exit code to a StepResult. The gating tool per stage
// matches detection's rustfmt/clippy-driver/cargo so a missing component degrades the
// right stage; the actual invocation always goes through cargo.
package rust

import (
	"context"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/quality"
	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Adapter provides Rust's quality steps, all sharing one ToolRunner.
type Adapter struct{ run runner.ToolRunner }

// New returns the Rust quality adapter backed by run.
func New(run runner.ToolRunner) Adapter { return Adapter{run: run} }

// step mirrors the Go adapter's terse one-type-per-stage shape.
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

func (s step) bind(r runner.ToolRunner) step { s.tr = r; return s }

// Steps lists Rust's steps; the gate decides the run order and which to skip. The tool
// strings match detection (rustfmt/clippy-driver/cargo) so missing-tool messages land
// on the right stage even though every command runs through cargo.
func (a Adapter) Steps() []quality.Step {
	return []quality.Step{
		step{kind: quality.Format, tool: "rustfmt", exec: runFormat}.bind(a.run),
		step{kind: quality.Lint, tool: "clippy-driver", exec: runLint}.bind(a.run),
		step{kind: quality.Test, tool: "cargo", exec: runTest}.bind(a.run),
	}
}

// runFormat checks formatting (`cargo fmt --check`, exit non-zero ⇒ unformatted) or
// rewrites in place (`cargo fmt`) in fix mode.
func runFormat(ctx context.Context, run runner.ToolRunner, unit quality.LangUnit, mode quality.Mode) quality.StepResult {
	args := []string{"fmt", "--check"}
	if mode == quality.ModeFix {
		args = []string{"fmt"}
	}
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: args, Dir: unit.Dir})
	return passOrFail(res, err)
}

// runLint runs clippy with warnings promoted to errors so any lint fails the stage.
func runLint(ctx context.Context, run runner.ToolRunner, unit quality.LangUnit, _ quality.Mode) quality.StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: []string{"clippy", "--", "-D", "warnings"}, Dir: unit.Dir})
	return passOrFail(res, err)
}

func runTest(ctx context.Context, run runner.ToolRunner, unit quality.LangUnit, _ quality.Mode) quality.StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: []string{"test"}, Dir: unit.Dir})
	return passOrFail(res, err)
}

// passOrFail maps a completed run to pass (exit 0) or fail (start error, timeout, or
// non-zero exit), reusing the same exit-code policy as the Go adapter.
func passOrFail(res runner.Result, err error) quality.StepResult {
	switch {
	case err != nil:
		return quality.StepResult{Status: quality.StatusFail, Detail: err.Error()}
	case res.TimedOut:
		return quality.StepResult{Status: quality.StatusFail, Detail: "timed out"}
	case res.ExitCode != 0:
		return quality.StepResult{Status: quality.StatusFail, Detail: output(res)}
	default:
		return quality.StepResult{Status: quality.StatusPass}
	}
}

func output(res runner.Result) string {
	return strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
}
