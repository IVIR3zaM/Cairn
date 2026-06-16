package quality

import (
	"context"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Dart's quality stages all run through the single `dart` toolchain (dart format /
// analyze / test), so every stage gates on the same `dart` tool — matching detection.
func init() {
	register("dart", func(run runner.ToolRunner, _ string) Adapter {
		return adapter{run: run, specs: []stepSpec{
			{kind: Format, tool: "dart", exec: dartFormat},
			{kind: Lint, tool: "dart", exec: dartLint},
			{kind: Test, tool: "dart", exec: dartTest},
		}}
	})
}

// dartFormat checks formatting (`dart format --output=none --set-exit-if-changed`,
// non-zero ⇒ unformatted) or rewrites in place (`dart format`) in fix mode.
func dartFormat(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"format", "--output=none", "--set-exit-if-changed", "."}
	if mode == ModeFix {
		args = []string{"format", "."}
	}
	res, err := run.Run(ctx, runner.Command{Name: "dart", Args: args, Dir: unit.Dir})
	return passOrFail(res, err)
}

// dartLint runs the static analyzer; any diagnostic fails the stage.
func dartLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "dart", Args: []string{"analyze"}, Dir: unit.Dir})
	return passOrFail(res, err)
}

func dartTest(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	args := []string{"test"}
	if unit.Color {
		args = append(args, "--color") // package:test forces color even when piped
	}
	res, err := run.Run(ctx, runner.Command{Name: "dart", Args: args, Dir: unit.Dir})
	return passOrFail(res, err)
}
