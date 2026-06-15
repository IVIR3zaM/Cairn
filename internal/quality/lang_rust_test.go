package quality

import (
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Every stage runs through cargo, so a non-zero cargo exit fails it and exit 0 passes —
// covering format check, lint, and test in one exit-code contract.
func TestRustExitCodeDrivesEveryStage(t *testing.T) {
	for _, k := range []Kind{Format, Lint, Test} {
		fail := &runner.Fake{Results: map[string]runner.Result{"cargo": {ExitCode: 1, Stderr: "boom"}}}
		if got := runStep("rust", fail, k, ModeCheck); got.Status != StatusFail {
			t.Errorf("%v non-zero exit: want Fail, got %v", k, got.Status)
		}
		if got := runStep("rust", &runner.Fake{}, k, ModeCheck); got.Status != StatusPass {
			t.Errorf("%v zero exit: want Pass, got %v (%s)", k, got.Status, got.Detail)
		}
	}
}

// Check mode asserts formatting (`cargo fmt --check`); fix mode rewrites (`cargo fmt`).
func TestRustFormatModeSelectsCheckOrWrite(t *testing.T) {
	check := &runner.Fake{}
	runStep("rust", check, Format, ModeCheck)
	if len(check.Calls) != 1 || check.Calls[0].Args[len(check.Calls[0].Args)-1] != "--check" {
		t.Errorf("check mode should call cargo fmt --check, got %+v", check.Calls)
	}
	fix := &runner.Fake{}
	runStep("rust", fix, Format, ModeFix)
	if len(fix.Calls) != 1 || len(fix.Calls[0].Args) != 1 || fix.Calls[0].Args[0] != "fmt" {
		t.Errorf("fix mode should call cargo fmt, got %+v", fix.Calls)
	}
}

// The gating tool per stage matches detection so a missing component degrades the right
// stage; rustfmt gates format, clippy-driver gates lint, cargo gates test.
func TestRustStepToolsMatchDetection(t *testing.T) {
	want := map[Kind]string{Format: "rustfmt", Lint: "clippy-driver", Test: "cargo"}
	for k, tool := range want {
		if got := stepOf("rust", &runner.Fake{}, k).Tool(); got != tool {
			t.Errorf("%v tool: want %q, got %q", k, tool, got)
		}
	}
}
