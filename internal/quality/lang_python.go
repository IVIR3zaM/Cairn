package quality

import (
	"context"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Python's quality stages can use either ruff (modern, single tool) or black+flake8
// (traditional pair). The standard choice selects which tools are used for formatting
// and linting; testing always runs via python3 -m pytest.
func init() {
	register("python", func(run runner.ToolRunner, standard string) Adapter {
		// Default to ruff if no standard specified
		if standard == "" {
			standard = "ruff"
		}

		var specs []stepSpec
		switch standard {
		case "ruff":
			specs = []stepSpec{
				{kind: Format, tool: "ruff", exec: pythonRuffFormat},
				{kind: Lint, tool: "ruff", exec: pythonRuffLint},
				{kind: Test, tool: "python3", exec: pythonTest},
			}
		case "black+flake8":
			specs = []stepSpec{
				{kind: Format, tool: "black", exec: pythonBlackFormat},
				{kind: Lint, tool: "flake8", exec: pythonFlake8Lint},
				{kind: Test, tool: "python3", exec: pythonTest},
			}
		default:
			// Unknown standard; fall back to ruff
			specs = []stepSpec{
				{kind: Format, tool: "ruff", exec: pythonRuffFormat},
				{kind: Lint, tool: "ruff", exec: pythonRuffLint},
				{kind: Test, tool: "python3", exec: pythonTest},
			}
		}

		return adapter{run: run, specs: specs}
	})
}

// pythonRuffFormat checks formatting (`ruff format --check`) or rewrites in place
// (`ruff format`) in fix mode.
func pythonRuffFormat(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"format", "--check", "."}
	if mode == ModeFix {
		args = []string{"format", "."}
	}
	res, err := run.Run(ctx, runner.Command{Name: "ruff", Args: args, Dir: unit.Dir, Env: pyColor(unit)})
	if r, bad := startOrExitFailure(res, err); bad {
		return r
	}
	// In check mode ruff exits 0 but lists files on stdout if formatting needed.
	if mode == ModeCheck && strings.TrimSpace(res.Stdout) != "" {
		return StepResult{Status: StatusFail, Detail: "needs formatting:\n" + strings.TrimSpace(res.Stdout)}
	}
	return StepResult{Status: StatusPass}
}

// pythonRuffLint runs ruff check to find linting issues.
func pythonRuffLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "ruff", Args: []string{"check", "."}, Dir: unit.Dir, Env: pyColor(unit)})
	return passOrFail(res, err)
}

// pyColor forces colored output for the Python tools that honor it (ruff, black, pytest);
// PY_COLORS is pytest's own switch, FORCE_COLOR covers the click/rich based tools.
func pyColor(unit LangUnit) []string { return colorEnv(unit, "FORCE_COLOR=1", "PY_COLORS=1") }

// pythonBlackFormat checks formatting (`black --check`) or rewrites in place (`black`)
// in fix mode.
func pythonBlackFormat(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"--check", "."}
	if mode == ModeFix {
		args = []string{"."}
	}
	res, err := run.Run(ctx, runner.Command{Name: "black", Args: args, Dir: unit.Dir, Env: pyColor(unit)})
	if r, bad := startOrExitFailure(res, err); bad {
		return r
	}
	// In check mode black exits 0 but lists files on stdout/stderr if reformatting needed.
	if mode == ModeCheck && strings.TrimSpace(res.Stdout+res.Stderr) != "" {
		return StepResult{Status: StatusFail, Detail: "needs formatting:\n" + strings.TrimSpace(res.Stdout+res.Stderr)}
	}
	return StepResult{Status: StatusPass}
}

// pythonFlake8Lint runs flake8 to find linting issues.
func pythonFlake8Lint(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "flake8", Args: []string{"."}, Dir: unit.Dir})
	return passOrFail(res, err)
}

// pythonTest runs tests via pytest.
func pythonTest(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "python3", Args: []string{"-m", "pytest"}, Dir: unit.Dir, Env: pyColor(unit)})
	return passOrFail(res, err)
}
