package quality

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// dartPkg returns a temp dir with a test/ subdir so the test stage actually runs (rather
// than being skipped for want of tests).
func dartPkg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "test"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runDart(f *runner.Fake, k Kind, dir string, mode Mode) StepResult {
	return stepOf("dart", f, k).Run(context.Background(), LangUnit{Dir: dir}, mode)
}

// Every stage runs through `dart`, so a non-zero dart exit fails it and exit 0 passes —
// covering format check, analyze, and test in one exit-code contract.
func TestDartExitCodeDrivesEveryStage(t *testing.T) {
	pkg := dartPkg(t)
	for _, k := range []Kind{Format, Lint, Test} {
		fail := &runner.Fake{Results: map[string]runner.Result{"dart": {ExitCode: 1, Stderr: "boom"}}}
		if got := runDart(fail, k, pkg, ModeCheck); got.Status != StatusFail {
			t.Errorf("%v non-zero exit: want Fail, got %v", k, got.Status)
		}
		if got := runDart(&runner.Fake{}, k, pkg, ModeCheck); got.Status != StatusPass {
			t.Errorf("%v zero exit: want Pass, got %v (%s)", k, got.Status, got.Detail)
		}
	}
}

// Check mode asserts formatting without rewriting (`--set-exit-if-changed`); fix mode
// rewrites in place (`dart format .`).
func TestDartFormatModeSelectsCheckOrWrite(t *testing.T) {
	check := &runner.Fake{}
	runStep("dart", check, Format, ModeCheck)
	if len(check.Calls) != 1 || check.Calls[0].Args[2] != "--set-exit-if-changed" {
		t.Errorf("check mode should call dart format --set-exit-if-changed, got %+v", check.Calls)
	}
	fix := &runner.Fake{}
	runStep("dart", fix, Format, ModeFix)
	if len(fix.Calls) != 1 || len(fix.Calls[0].Args) != 2 || fix.Calls[0].Args[1] != "." {
		t.Errorf("fix mode should call dart format ., got %+v", fix.Calls)
	}
}

// A package without a test/ directory is skipped, not failed: `dart test` would exit
// non-zero on a missing default test dir, which must not fail a library that has no tests.
func TestDartTestSkipsWithoutTestDir(t *testing.T) {
	f := &runner.Fake{}
	got := runDart(f, Test, t.TempDir(), ModeCheck) // temp dir has no test/ subdir
	if got.Status != StatusSkip {
		t.Errorf("missing test/: want Skip, got %v", got.Status)
	}
	if len(f.Calls) != 0 {
		t.Errorf("dart test must not run when test/ is absent, got %+v", f.Calls)
	}
}

// Every stage gates on the single `dart` tool, matching detection.
func TestDartStepToolsMatchDetection(t *testing.T) {
	for _, k := range []Kind{Format, Lint, Test} {
		if got := stepOf("dart", &runner.Fake{}, k).Tool(); got != "dart" {
			t.Errorf("%v tool: want %q, got %q", k, "dart", got)
		}
	}
}
