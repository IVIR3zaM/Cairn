package quality

import (
	"context"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// JavaScript/TypeScript quality stages run local dev-dependencies through `npx`, so every
// stage gates on `npx` (matching detection) — its presence implies the whole node/npm
// toolchain. The standard choice selects the tools: `eslint` uses prettier (format) +
// eslint (lint); `biome` uses biome for both. Testing always runs via `npm test`.
func init() {
	register("javascript", func(run runner.ToolRunner, standard string) Adapter {
		if standard == "" {
			standard = "eslint"
		}

		var specs []stepSpec
		switch standard {
		case "biome":
			specs = []stepSpec{
				{kind: Format, tool: "npx", exec: jsBiomeFormat},
				{kind: Lint, tool: "npx", exec: jsBiomeLint},
				{kind: Test, tool: "npx", exec: jsTest},
			}
		default: // "eslint" and any unknown standard fall back to prettier + eslint
			specs = []stepSpec{
				{kind: Format, tool: "npx", exec: jsPrettierFormat},
				{kind: Lint, tool: "npx", exec: jsEslintLint},
				{kind: Test, tool: "npx", exec: jsTest},
			}
		}

		return adapter{run: run, specs: specs}
	})
}

// jsPrettierFormat checks formatting (`npx prettier --check .`, non-zero ⇒ unformatted) or
// rewrites in place (`npx prettier --write .`) in fix mode.
func jsPrettierFormat(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"prettier", "--check", "."}
	if mode == ModeFix {
		args = []string{"prettier", "--write", "."}
	}
	res, err := run.Run(ctx, runner.Command{Name: "npx", Args: args, Dir: unit.Dir, Env: jsColor(unit)})
	return passOrFail(res, err)
}

// jsColor forces colored output for the node toolchain (eslint/prettier/biome/npm all
// honor FORCE_COLOR) when color is requested.
func jsColor(unit LangUnit) []string { return colorEnv(unit, "FORCE_COLOR=1") }

// jsEslintLint runs eslint; any reported error fails the stage via its exit code.
// eslint exits 0 on warnings by default, so strict mode adds --max-warnings=0 to
// make a single warning fail the stage.
func jsEslintLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	args := []string{"eslint", "."}
	if unit.Strict {
		args = append(args, "--max-warnings=0")
	}
	res, err := run.Run(ctx, runner.Command{Name: "npx", Args: args, Dir: unit.Dir, Env: jsColor(unit)})
	return passOrFail(res, err)
}

// jsBiomeFormat checks formatting (`npx biome format .`, non-zero ⇒ unformatted) or
// rewrites in place (`npx biome format --write .`) in fix mode.
func jsBiomeFormat(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"@biomejs/biome", "format", "."}
	if mode == ModeFix {
		args = []string{"@biomejs/biome", "format", "--write", "."}
	}
	res, err := run.Run(ctx, runner.Command{Name: "npx", Args: args, Dir: unit.Dir, Env: jsColor(unit)})
	return passOrFail(res, err)
}

// jsBiomeLint runs biome's linter; a non-zero exit (issues found) fails the stage.
// biome exits 0 when only warnings are present, so strict mode adds --error-on-warnings
// to fail on them too.
func jsBiomeLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	args := []string{"@biomejs/biome", "lint", "."}
	if unit.Strict {
		args = append(args, "--error-on-warnings")
	}
	res, err := run.Run(ctx, runner.Command{Name: "npx", Args: args, Dir: unit.Dir, Env: jsColor(unit)})
	return passOrFail(res, err)
}

// jsTest runs the package's test script (`npm test`); its exit code drives the result.
func jsTest(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	res, err := run.Run(ctx, runner.Command{Name: "npm", Args: []string{"test"}, Dir: unit.Dir, Env: jsColor(unit)})
	return passOrFail(res, err)
}
