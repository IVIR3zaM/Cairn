package quality

import (
	"context"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// Helper to get a specific step from an adapter with a given standard.
func stepOfWithStandard(lang, standard string, f *runner.Fake, k Kind) Step {
	a, ok := AdapterFor(lang, f, standard)
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

// Helper to run a step with a given standard.
func runStepWithStandard(lang, standard string, f *runner.Fake, k Kind, mode Mode) StepResult {
	s := stepOfWithStandard(lang, standard, f, k)
	if s == nil {
		return StepResult{Status: StatusFail, Detail: "step not found"}
	}
	return s.Run(context.Background(), LangUnit{Dir: "."}, mode)
}

// Ruff format check fails when files need formatting (ruff lists them on stdout).
func TestPythonRuffFormatCheckFailsWhenFilesListed(t *testing.T) {
	bad := &runner.Fake{Results: map[string]runner.Result{"ruff": {Stdout: "main.py\n"}}}
	if got := runStepWithStandard("python", "ruff", bad, Format, ModeCheck); got.Status != StatusFail {
		t.Errorf("ruff format check with listed files: want Fail, got %v", got.Status)
	}
	clean := &runner.Fake{}
	if got := runStepWithStandard("python", "ruff", clean, Format, ModeCheck); got.Status != StatusPass {
		t.Errorf("ruff format check clean: want Pass, got %v (%s)", got.Status, got.Detail)
	}
}

// Ruff format fix mode rewrites in place: ruff is invoked without --check.
func TestPythonRuffFormatFixUsesNoCheckFlag(t *testing.T) {
	f := &runner.Fake{Results: map[string]runner.Result{"ruff": {Stdout: "main.py\n"}}}
	if got := runStepWithStandard("python", "ruff", f, Format, ModeFix); got.Status != StatusPass {
		t.Errorf("ruff format fix: want Pass, got %v", got.Status)
	}
	if len(f.Calls) != 1 || len(f.Calls[0].Args) < 1 || f.Calls[0].Args[0] != "format" {
		t.Errorf("ruff format fix should call ruff format (no --check), got %+v", f.Calls)
	}
}

// Ruff check (lint) surfaces the tool's exit code: zero passes, non-zero fails.
func TestPythonRuffLintExitCodeDriveResult(t *testing.T) {
	fail := &runner.Fake{Results: map[string]runner.Result{"ruff": {ExitCode: 1, Stdout: "issues found"}}}
	if got := runStepWithStandard("python", "ruff", fail, Lint, ModeCheck); got.Status != StatusFail {
		t.Errorf("ruff lint non-zero exit: want Fail, got %v", got.Status)
	}
	pass := &runner.Fake{}
	if got := runStepWithStandard("python", "ruff", pass, Lint, ModeCheck); got.Status != StatusPass {
		t.Errorf("ruff lint zero exit: want Pass, got %v", got.Status)
	}
}

// Black format check fails when black outputs formatting suggestions.
func TestPythonBlackFormatCheckFailsWhenChangesNeeded(t *testing.T) {
	bad := &runner.Fake{Results: map[string]runner.Result{"black": {Stdout: "would reformat main.py\n"}}}
	if got := runStepWithStandard("python", "black+flake8", bad, Format, ModeCheck); got.Status != StatusFail {
		t.Errorf("black format check with changes needed: want Fail, got %v", got.Status)
	}
	clean := &runner.Fake{}
	if got := runStepWithStandard("python", "black+flake8", clean, Format, ModeCheck); got.Status != StatusPass {
		t.Errorf("black format check clean: want Pass, got %v", got.Status)
	}
}

// Black format fix mode rewrites in place: black is invoked without --check.
func TestPythonBlackFormatFixUsesNoCheckFlag(t *testing.T) {
	f := &runner.Fake{Results: map[string]runner.Result{"black": {Stdout: "would reformat main.py\n"}}}
	if got := runStepWithStandard("python", "black+flake8", f, Format, ModeFix); got.Status != StatusPass {
		t.Errorf("black format fix: want Pass, got %v", got.Status)
	}
	if len(f.Calls) != 1 || len(f.Calls[0].Args) < 1 || f.Calls[0].Args[0] == "--check" {
		t.Errorf("black format fix should call black without --check, got %+v", f.Calls)
	}
}

// Flake8 lint surfaces the tool's exit code: zero passes, non-zero fails.
func TestPythonFlake8LintExitCodeDrivesResult(t *testing.T) {
	fail := &runner.Fake{Results: map[string]runner.Result{"flake8": {ExitCode: 1, Stdout: "issues found"}}}
	if got := runStepWithStandard("python", "black+flake8", fail, Lint, ModeCheck); got.Status != StatusFail {
		t.Errorf("flake8 lint non-zero exit: want Fail, got %v", got.Status)
	}
	pass := &runner.Fake{}
	if got := runStepWithStandard("python", "black+flake8", pass, Lint, ModeCheck); got.Status != StatusPass {
		t.Errorf("flake8 lint zero exit: want Pass, got %v", got.Status)
	}
}

// Test via python3 -m pytest: exit code determines result.
func TestPythonTestExitCodeDrivesResult(t *testing.T) {
	fail := &runner.Fake{Results: map[string]runner.Result{"python3": {ExitCode: 1, Stdout: "tests failed"}}}
	if got := runStep("python", fail, Test, ModeCheck); got.Status != StatusFail {
		t.Errorf("python test non-zero exit: want Fail, got %v", got.Status)
	}
	pass := &runner.Fake{}
	if got := runStep("python", pass, Test, ModeCheck); got.Status != StatusPass {
		t.Errorf("python test zero exit: want Pass, got %v", got.Status)
	}
}

// Standard switch: ruff and black+flake8 select different tools.
func TestPythonStandardSelectsTools(t *testing.T) {
	cases := []struct {
		standard   string
		wantFormat string
		wantLint   string
	}{
		{"ruff", "ruff", "ruff"},
		{"black+flake8", "black", "flake8"},
		{"", "ruff", "ruff"}, // empty defaults to ruff
	}

	for _, c := range cases {
		f := &runner.Fake{}
		a, ok := AdapterFor("python", f, c.standard)
		if !ok {
			t.Errorf("standard %q: AdapterFor failed", c.standard)
			continue
		}
		steps := a.Steps()
		var format, lint *Step
		for _, s := range steps {
			if s.Kind() == Format {
				format = &s
			} else if s.Kind() == Lint {
				lint = &s
			}
		}
		if format == nil || lint == nil {
			t.Errorf("standard %q: missing format or lint step", c.standard)
			continue
		}
		if got := (*format).Tool(); got != c.wantFormat {
			t.Errorf("standard %q format tool: want %q, got %q", c.standard, c.wantFormat, got)
		}
		if got := (*lint).Tool(); got != c.wantLint {
			t.Errorf("standard %q lint tool: want %q, got %q", c.standard, c.wantLint, got)
		}
	}
}

// A command that fails to start (e.g. binary vanished) fails the stage with its error.
func TestPythonStartErrorFails(t *testing.T) {
	f := &runner.Fake{Err: context.Canceled}
	if got := runStep("python", f, Test, ModeCheck); got.Status != StatusFail {
		t.Errorf("start error: want Fail, got %v", got.Status)
	}
}
