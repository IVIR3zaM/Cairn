package quality

import (
	"context"
	"os"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// colorEnv returns the inherited process environment plus the given color-forcing
// variables when the unit asked for color (a verbose run on a color TTY); otherwise nil,
// leaving the command to inherit the environment unchanged. The variable names are
// tool-specific and supplied by each lang_<name>.go — this helper holds no tool knowledge,
// only the merge. A command keeps capturing into a pipe, so without an explicit override
// every tool would auto-disable color; these vars tell it to emit color anyway.
func colorEnv(unit LangUnit, vars ...string) []string {
	if !unit.Color || len(vars) == 0 {
		return nil
	}
	return append(os.Environ(), vars...)
}

// execFunc runs one stage by shelling out through run and maps the outcome to a result.
type execFunc func(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult

// stepSpec is a language's declaration of one stage: its kind, the tool that gates it
// (matched against detection's ToolInfo), and how to run it. Language files build a
// slice of these; the shared adapter binds them to a ToolRunner.
type stepSpec struct {
	kind Kind
	tool string
	exec execFunc
}

// adapter is the one concrete Adapter every language shares: a list of stepSpecs and the
// ToolRunner they run through. Languages differ only in their specs (in lang_<name>.go),
// so there is no per-language adapter type.
type adapter struct {
	run   runner.ToolRunner
	specs []stepSpec
}

func (a adapter) Steps() []Step {
	steps := make([]Step, len(a.specs))
	for i, sp := range a.specs {
		steps[i] = step{spec: sp, run: a.run}
	}
	return steps
}

// step binds one stepSpec to a runner, satisfying the Step port.
type step struct {
	spec stepSpec
	run  runner.ToolRunner
}

func (s step) Kind() Kind   { return s.spec.kind }
func (s step) Tool() string { return s.spec.tool }
func (s step) Run(ctx context.Context, unit LangUnit, mode Mode) StepResult {
	return s.spec.exec(ctx, s.run, unit, mode)
}

// passOrFail maps a completed run to pass (exit 0) or fail (start error, timeout, or
// non-zero exit). Most stages are a thin wrapper over this; only stages that signal
// failure other than via exit code (e.g. gofumpt listing files) inspect the output.
func passOrFail(res runner.Result, err error) StepResult {
	if r, bad := startOrExitFailure(res, err); bad {
		return r
	}
	return StepResult{Status: StatusPass}
}

// startOrExitFailure returns a Fail result when the command could not start, timed out,
// or exited non-zero. The bool reports whether a failure was found.
func startOrExitFailure(res runner.Result, err error) (StepResult, bool) {
	switch {
	case err != nil:
		return StepResult{Status: StatusFail, Detail: err.Error()}, true
	case res.TimedOut:
		return StepResult{Status: StatusFail, Detail: "timed out"}, true
	case res.ExitCode != 0:
		return StepResult{Status: StatusFail, Detail: output(res)}, true
	default:
		return StepResult{}, false
	}
}

func output(res runner.Result) string {
	return strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
}
