package rust

import (
	"context"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/quality"
	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// stepOf returns the adapter's step of the given kind.
func stepOf(a Adapter, k quality.Kind) quality.Step {
	for _, s := range a.Steps() {
		if s.Kind() == k {
			return s
		}
	}
	return nil
}

func run(f *runner.Fake, k quality.Kind, mode quality.Mode) quality.StepResult {
	return stepOf(New(f), k).Run(context.Background(), quality.LangUnit{Dir: "."}, mode)
}

// Every stage runs through cargo, so a non-zero cargo exit fails it and exit 0 passes —
// covering format check, lint, and test in one exit-code contract.
func TestExitCodeDrivesEveryStage(t *testing.T) {
	for _, k := range []quality.Kind{quality.Format, quality.Lint, quality.Test} {
		fail := &runner.Fake{Results: map[string]runner.Result{"cargo": {ExitCode: 1, Stderr: "boom"}}}
		if got := run(fail, k, quality.ModeCheck); got.Status != quality.StatusFail {
			t.Errorf("%v non-zero exit: want Fail, got %v", k, got.Status)
		}
		if got := run(&runner.Fake{}, k, quality.ModeCheck); got.Status != quality.StatusPass {
			t.Errorf("%v zero exit: want Pass, got %v (%s)", k, got.Status, got.Detail)
		}
	}
}

// Check mode asserts formatting (`cargo fmt --check`); fix mode rewrites (`cargo fmt`).
func TestFormatModeSelectsCheckOrWrite(t *testing.T) {
	check := &runner.Fake{}
	run(check, quality.Format, quality.ModeCheck)
	if len(check.Calls) != 1 || check.Calls[0].Args[len(check.Calls[0].Args)-1] != "--check" {
		t.Errorf("check mode should call cargo fmt --check, got %+v", check.Calls)
	}
	fix := &runner.Fake{}
	run(fix, quality.Format, quality.ModeFix)
	if len(fix.Calls) != 1 || len(fix.Calls[0].Args) != 1 || fix.Calls[0].Args[0] != "fmt" {
		t.Errorf("fix mode should call cargo fmt, got %+v", fix.Calls)
	}
}

// A command that fails to start fails the stage with its error.
func TestStartErrorFails(t *testing.T) {
	f := &runner.Fake{Err: context.Canceled}
	if got := run(f, quality.Test, quality.ModeCheck); got.Status != quality.StatusFail {
		t.Errorf("start error: want Fail, got %v", got.Status)
	}
}

// The gating tool per stage matches detection so a missing component degrades the right
// stage; clippy gates lint, rustfmt gates format, cargo gates test.
func TestStepToolsMatchDetection(t *testing.T) {
	want := map[quality.Kind]string{
		quality.Format: "rustfmt",
		quality.Lint:   "clippy-driver",
		quality.Test:   "cargo",
	}
	a := New(&runner.Fake{})
	for k, tool := range want {
		if got := stepOf(a, k).Tool(); got != tool {
			t.Errorf("%v tool: want %q, got %q", k, tool, got)
		}
	}
}
