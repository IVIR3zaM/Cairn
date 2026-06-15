package quality

import (
	"context"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Rust's quality stages all run through cargo (cargo fmt / clippy / test). The gating
// tool per stage matches detection (rustfmt/clippy-driver/cargo) so a missing component
// degrades the right stage even though the invocation is always cargo.
func init() {
	register("rust", func(run runner.ToolRunner, _ string) Adapter {
		return adapter{run: run, specs: []stepSpec{
			{kind: Format, tool: "rustfmt", exec: rustFormat},
			{kind: Lint, tool: "clippy-driver", exec: rustLint},
			{kind: Test, tool: "cargo", exec: rustTest},
		}}
	})
}

// rustFormat checks formatting (`cargo fmt --check`, non-zero ⇒ unformatted) or rewrites
// in place (`cargo fmt`) in fix mode.
func rustFormat(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"fmt", "--check"}
	if mode == ModeFix {
		args = []string{"fmt"}
	}
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: args, Dir: unit.Dir})
	return passOrFail(res, err)
}

// rustLint runs clippy with warnings promoted to errors so any lint fails the stage.
func rustLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: []string{"clippy", "--", "-D", "warnings"}, Dir: unit.Dir})
	return passOrFail(res, err)
}

func rustTest(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: []string{"test"}, Dir: unit.Dir})
	return passOrFail(res, err)
}
