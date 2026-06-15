package quality

import (
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// firstArg returns the first argument of the single recorded call, or "" if none.
func firstArg(f *runner.Fake) string {
	if len(f.Calls) != 1 || len(f.Calls[0].Args) == 0 {
		return ""
	}
	return f.Calls[0].Args[0]
}

// Standard switch: eslint uses prettier+eslint, biome uses biome for both; empty defaults
// to eslint. Format/lint share the npx gate, so the invoked binary is the first npx arg.
func TestJSStandardSelectsTools(t *testing.T) {
	cases := []struct {
		standard   string
		wantFormat string // first arg to npx for the format stage
		wantLint   string // first arg to npx for the lint stage
	}{
		{"eslint", "prettier", "eslint"},
		{"biome", "@biomejs/biome", "@biomejs/biome"},
		{"", "prettier", "eslint"}, // empty defaults to eslint
	}
	for _, c := range cases {
		ff := &runner.Fake{}
		runStepWithStandard("javascript", c.standard, ff, Format, ModeCheck)
		if got := firstArg(ff); got != c.wantFormat {
			t.Errorf("standard %q format: want npx %q, got %q (calls %+v)", c.standard, c.wantFormat, got, ff.Calls)
		}
		lf := &runner.Fake{}
		runStepWithStandard("javascript", c.standard, lf, Lint, ModeCheck)
		if got := firstArg(lf); got != c.wantLint {
			t.Errorf("standard %q lint: want npx %q, got %q (calls %+v)", c.standard, c.wantLint, got, lf.Calls)
		}
	}
}

// Fix mode rewrites in place: prettier uses --write (not --check) and biome adds --write.
func TestJSFormatFixUsesWriteFlag(t *testing.T) {
	pf := &runner.Fake{}
	runStepWithStandard("javascript", "eslint", pf, Format, ModeFix)
	if args := pf.Calls[0].Args; !contains(args, "--write") || contains(args, "--check") {
		t.Errorf("prettier fix should use --write and not --check, got %v", args)
	}
	bf := &runner.Fake{}
	runStepWithStandard("javascript", "biome", bf, Format, ModeFix)
	if args := bf.Calls[0].Args; !contains(args, "--write") {
		t.Errorf("biome fix should use --write, got %v", args)
	}
}

// Every stage gates on npx (matching detection), so a missing toolchain degrades the
// right stage regardless of which underlying tool the standard selects.
func TestJSStagesGateOnNpx(t *testing.T) {
	for _, std := range []string{"eslint", "biome"} {
		a, ok := AdapterFor("javascript", &runner.Fake{}, std)
		if !ok {
			t.Fatalf("AdapterFor(javascript, %q) failed", std)
		}
		for _, s := range a.Steps() {
			if got := s.Tool(); got != "npx" {
				t.Errorf("standard %q, stage %v: want gate tool npx, got %q", std, s.Kind(), got)
			}
		}
	}
}

// Exit-code mapping for the JS stages: a clean toolchain passes, a non-zero exit fails.
// The test stage runs `npm test`, distinct from the npx-driven format/lint stages.
func TestJSExitCodeDrivesResult(t *testing.T) {
	npxFail := &runner.Fake{Results: map[string]runner.Result{"npx": {ExitCode: 1, Stdout: "issues"}}}
	if got := runStepWithStandard("javascript", "eslint", npxFail, Format, ModeCheck); got.Status != StatusFail {
		t.Errorf("prettier --check non-zero: want Fail, got %v", got.Status)
	}
	if got := runStepWithStandard("javascript", "eslint", &runner.Fake{}, Lint, ModeCheck); got.Status != StatusPass {
		t.Errorf("eslint clean: want Pass, got %v", got.Status)
	}

	testFail := &runner.Fake{Results: map[string]runner.Result{"npm": {ExitCode: 1, Stdout: "tests failed"}}}
	got := runStep("javascript", testFail, Test, ModeCheck)
	if got.Status != StatusFail {
		t.Errorf("npm test non-zero: want Fail, got %v", got.Status)
	}
	if len(testFail.Calls) != 1 || testFail.Calls[0].Name != "npm" || firstArg(testFail) != "test" {
		t.Errorf("test stage should run `npm test`, got %+v", testFail.Calls)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
