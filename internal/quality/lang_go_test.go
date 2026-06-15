package quality

import (
	"context"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// stepOf returns the registered adapter's step of the given kind, also exercising the
// self-registration: an unregistered language would make AdapterFor return false.
func stepOf(lang string, f *runner.Fake, k Kind) Step {
	a, ok := AdapterFor(lang, f, "")
	if !ok {
		return nil
	}
	for _, s := range a.Steps() {
		if s.Kind() == k {
			return s
		}
	}
	return nil
}

func runStep(lang string, f *runner.Fake, k Kind, mode Mode) StepResult {
	return stepOf(lang, f, k).Run(context.Background(), LangUnit{Dir: "."}, mode)
}

// gofumpt exits 0 in check mode but listing files on stdout means unformatted code,
// which must fail; an empty listing passes.
func TestGoFormatCheckFailsWhenFilesListed(t *testing.T) {
	bad := &runner.Fake{Results: map[string]runner.Result{"gofumpt": {Stdout: "main.go\n"}}}
	if got := runStep("go", bad, Format, ModeCheck); got.Status != StatusFail {
		t.Errorf("listed file: want Fail, got %v", got.Status)
	}
	clean := &runner.Fake{} // unknown command → zero Result → exit 0, empty stdout
	if got := runStep("go", clean, Format, ModeCheck); got.Status != StatusPass {
		t.Errorf("clean tree: want Pass, got %v (%s)", got.Status, got.Detail)
	}
}

// Fix mode rewrites in place: gofumpt is invoked with -w and stdout is irrelevant.
func TestGoFormatFixUsesWriteFlag(t *testing.T) {
	f := &runner.Fake{Results: map[string]runner.Result{"gofumpt": {Stdout: "main.go\n"}}}
	if got := runStep("go", f, Format, ModeFix); got.Status != StatusPass {
		t.Errorf("fix mode: want Pass, got %v", got.Status)
	}
	if len(f.Calls) != 1 || f.Calls[0].Args[0] != "-w" {
		t.Errorf("fix mode should call gofumpt -w, got %+v", f.Calls)
	}
}

// Lint and test surface the tool's exit code: zero passes, non-zero fails.
func TestGoExitCodeDrivesLintAndTest(t *testing.T) {
	cases := []struct {
		kind Kind
		tool string
	}{{Lint, "golangci-lint"}, {Test, "go"}}
	for _, c := range cases {
		fail := &runner.Fake{Results: map[string]runner.Result{c.tool: {ExitCode: 1, Stdout: "boom"}}}
		if got := runStep("go", fail, c.kind, ModeCheck); got.Status != StatusFail {
			t.Errorf("%v non-zero exit: want Fail, got %v", c.kind, got.Status)
		}
		if got := runStep("go", &runner.Fake{}, c.kind, ModeCheck); got.Status != StatusPass {
			t.Errorf("%v zero exit: want Pass, got %v", c.kind, got.Status)
		}
	}
}

// A command that fails to start (e.g. binary vanished) fails the stage with its error.
func TestGoStartErrorFails(t *testing.T) {
	f := &runner.Fake{Err: context.Canceled}
	if got := runStep("go", f, Test, ModeCheck); got.Status != StatusFail {
		t.Errorf("start error: want Fail, got %v", got.Status)
	}
}
