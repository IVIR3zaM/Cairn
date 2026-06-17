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
			{kind: Format, tool: "rustfmt", fix: "cargo fmt", exec: rustFormat},
			{kind: Lint, tool: "clippy-driver", fix: "cargo clippy --fix", exec: rustLint},
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
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: args, Dir: unit.Dir, Env: cargoColor(unit)})
	return passOrFail(res, err)
}

// rustLint runs clippy with warnings promoted to errors so any lint fails the stage. In
// fix mode it applies clippy's machine-applicable suggestions in place first; --allow-dirty
// /--allow-staged let it write even with uncommitted changes (verify is run mid-edit), and
// it still exits non-zero on whatever could not be auto-fixed.
func rustLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"clippy"}
	if mode == ModeFix {
		args = append(args, "--fix", "--allow-dirty", "--allow-staged")
	}
	args = append(args, "--", "-D", "warnings")
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: args, Dir: unit.Dir, Env: cargoColor(unit)})
	return passOrFail(res, err)
}

func rustTest(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "cargo", Args: []string{"test"}, Dir: unit.Dir, Env: cargoColor(unit)})
	return passOrFail(res, err)
}

// cargoColor forces cargo's colored output across every stage when color is requested.
func cargoColor(unit LangUnit) []string { return colorEnv(unit, "CARGO_TERM_COLOR=always") }
