package quality

import (
	"context"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Go's quality stages wrap gofumpt, golangci-lint, and `go test`. Each shells out and
// maps the tool's exit code to a result; no Go-specific policy lives in the gate core.
// This file is the whole source of truth for verifying Go — drop a sibling lang_<x>.go
// to add a language, with no edits to the gate or the CLI.
func init() {
	register("go", func(run runner.ToolRunner, _ string) Adapter {
		return adapter{run: run, specs: []stepSpec{
			{kind: Format, tool: "gofumpt", exec: goFormat},
			{kind: Lint, tool: "golangci-lint", exec: goLint},
			{kind: Test, tool: "go", exec: goTest},
		}}
	})
}

func goFormat(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"-l", "."}
	if mode == ModeFix {
		args = []string{"-w", "."}
	}
	res, err := run.Run(ctx, runner.Command{Name: "gofumpt", Args: args, Dir: unit.Dir})
	if r, bad := startOrExitFailure(res, err); bad {
		return r
	}
	// In check mode gofumpt exits 0 but lists unformatted files on stdout.
	if mode == ModeCheck && strings.TrimSpace(res.Stdout) != "" {
		return StepResult{Status: StatusFail, Detail: "needs formatting:\n" + strings.TrimSpace(res.Stdout)}
	}
	return StepResult{Status: StatusPass}
}

func goLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "golangci-lint", Args: []string{"run"}, Dir: unit.Dir})
	return passOrFail(res, err)
}

func goTest(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "go", Args: []string{"test", "./..."}, Dir: unit.Dir})
	return passOrFail(res, err)
}
