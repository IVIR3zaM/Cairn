package quality

import (
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Every stage runs through `dart`, so a non-zero dart exit fails it and exit 0 passes —
// covering format check, analyze, and test in one exit-code contract.
func TestDartExitCodeDrivesEveryStage(t *testing.T) {
	for _, k := range []Kind{Format, Lint, Test} {
		fail := &runner.Fake{Results: map[string]runner.Result{"dart": {ExitCode: 1, Stderr: "boom"}}}
		if got := runStep("dart", fail, k, ModeCheck); got.Status != StatusFail {
			t.Errorf("%v non-zero exit: want Fail, got %v", k, got.Status)
		}
		if got := runStep("dart", &runner.Fake{}, k, ModeCheck); got.Status != StatusPass {
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

// Every stage gates on the single `dart` tool, matching detection.
func TestDartStepToolsMatchDetection(t *testing.T) {
	for _, k := range []Kind{Format, Lint, Test} {
		if got := stepOf("dart", &runner.Fake{}, k).Tool(); got != "dart" {
			t.Errorf("%v tool: want %q, got %q", k, "dart", got)
		}
	}
}
