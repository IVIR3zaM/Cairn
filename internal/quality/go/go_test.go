package golang

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

// gofumpt exits 0 in check mode but listing files on stdout means unformatted code,
// which must fail; an empty listing passes.
func TestFormatCheckFailsWhenFilesListed(t *testing.T) {
	bad := &runner.Fake{Results: map[string]runner.Result{"gofumpt": {Stdout: "main.go\n"}}}
	if got := run(bad, quality.Format, quality.ModeCheck); got.Status != quality.StatusFail {
		t.Errorf("listed file: want Fail, got %v", got.Status)
	}
	clean := &runner.Fake{} // unknown command → zero Result → exit 0, empty stdout
	if got := run(clean, quality.Format, quality.ModeCheck); got.Status != quality.StatusPass {
		t.Errorf("clean tree: want Pass, got %v (%s)", got.Status, got.Detail)
	}
}

// Fix mode rewrites in place: gofumpt is invoked with -w and stdout is irrelevant.
func TestFormatFixUsesWriteFlag(t *testing.T) {
	f := &runner.Fake{Results: map[string]runner.Result{"gofumpt": {Stdout: "main.go\n"}}}
	if got := run(f, quality.Format, quality.ModeFix); got.Status != quality.StatusPass {
		t.Errorf("fix mode: want Pass, got %v", got.Status)
	}
	if len(f.Calls) != 1 || f.Calls[0].Args[0] != "-w" {
		t.Errorf("fix mode should call gofumpt -w, got %+v", f.Calls)
	}
}

// Lint and test surface the tool's exit code: zero passes, non-zero fails.
func TestExitCodeDrivesLintAndTest(t *testing.T) {
	cases := []struct {
		kind quality.Kind
		tool string
	}{{quality.Lint, "golangci-lint"}, {quality.Test, "go"}}
	for _, c := range cases {
		fail := &runner.Fake{Results: map[string]runner.Result{c.tool: {ExitCode: 1, Stdout: "boom"}}}
		if got := run(fail, c.kind, quality.ModeCheck); got.Status != quality.StatusFail {
			t.Errorf("%v non-zero exit: want Fail, got %v", c.kind, got.Status)
		}
		if got := run(&runner.Fake{}, c.kind, quality.ModeCheck); got.Status != quality.StatusPass {
			t.Errorf("%v zero exit: want Pass, got %v", c.kind, got.Status)
		}
	}
}

// A command that fails to start (e.g. binary vanished) fails the stage with its error.
func TestStartErrorFails(t *testing.T) {
	f := &runner.Fake{Err: context.Canceled}
	if got := run(f, quality.Test, quality.ModeCheck); got.Status != quality.StatusFail {
		t.Errorf("start error: want Fail, got %v", got.Status)
	}
}
