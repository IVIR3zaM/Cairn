package quality

import (
	"context"
	"os"
	"path/filepath"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Java verification is owned by the project's build tool, so Cairn shells out to its
// standard lifecycle instead of guessing plugin goals. (An earlier version invoked
// `mvn spotless:check`, which hung resolving a Spotless plugin most projects never
// declare, then failed.) The `standard` selects the build tool — `maven` (default) or
// `gradle` — and each stage prefers the committed wrapper (`mvnw`/`gradlew`) so the
// project's pinned version is used, running non-interactively so a prompt can never
// stall verify. Stages gate on `java`: the wrapper fetches Maven/Gradle itself, so the
// JDK is the only hard requirement (and matches detection).
func init() {
	register("java", func(run runner.ToolRunner, standard string) Adapter {
		exec := mavenVerify
		if standard == "gradle" {
			exec = gradleCheck
		}
		return adapter{run: run, specs: []stepSpec{
			{kind: Test, tool: "java", exec: exec},
		}}
	})
}

// mavenVerify runs the Maven verification lifecycle (`verify`: compile, configured checks
// such as checkstyle, and tests) in batch (non-interactive) mode via the wrapper if present.
func mavenVerify(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	args := []string{"-B", "verify"}
	if unit.Color {
		args = append(args, "-Dstyle.color=always") // Maven's own ANSI override
	}
	res, err := run.Run(ctx, runner.Command{
		Name: buildTool(unit.Dir, "mvnw", "mvn"),
		Args: args,
		Dir:  unit.Dir,
	})
	return passOrFail(res, err)
}

// gradleCheck runs Gradle's `check` task (compile, linters, and tests) with a plain,
// non-interactive console via the wrapper if present.
func gradleCheck(ctx context.Context, run runner.ToolRunner, unit LangUnit, _ Mode) StepResult {
	// "rich" keeps the non-interactive guarantee while forcing color; "plain" is colorless.
	console := "--console=plain"
	if unit.Color {
		console = "--console=rich"
	}
	res, err := run.Run(ctx, runner.Command{
		Name: buildTool(unit.Dir, "gradlew", "gradle"),
		Args: []string{console, "check"},
		Dir:  unit.Dir,
	})
	return passOrFail(res, err)
}

// buildTool returns the absolute path to a committed wrapper script in dir when present,
// else the bare tool name to be resolved from PATH.
func buildTool(dir, wrapper, fallback string) string {
	p := filepath.Join(dir, wrapper)
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	return fallback
}
