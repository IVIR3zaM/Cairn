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
				{kind: Format, tool: "npx", fix: "npx @biomejs/biome format --write .", exec: jsBiomeFormat},
				{kind: Lint, tool: "npx", fix: "npx @biomejs/biome lint --write .", exec: jsBiomeLint},
				{kind: Test, tool: "npx", exec: jsTest},
			}
		default: // "eslint" and any unknown standard fall back to prettier + eslint
			specs = []stepSpec{
				{kind: Format, tool: "npx", fix: "npx prettier --write .", exec: jsPrettierFormat},
				{kind: Lint, tool: "npx", fix: "npx eslint . --fix", exec: jsEslintLint},
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
// make a single warning fail the stage. In fix mode it applies eslint's fixable rules
// in place first, then still exits non-zero on whatever remains.
func jsEslintLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"eslint", "."}
	if mode == ModeFix {
		args = append(args, "--fix")
	}
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
// to fail on them too. In fix mode --write applies biome's safe fixes in place first,
// then it still exits non-zero on the findings it could not repair.
func jsBiomeLint(ctx context.Context, run runner.ToolRunner, unit LangUnit, mode Mode) StepResult {
	args := []string{"@biomejs/biome", "lint", "."}
	if mode == ModeFix {
		args = []string{"@biomejs/biome", "lint", "--write", "."}
	}
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
