package quality

import (
	"context"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

// fakeStep is a scripted Step recording whether it ran and in which mode.
type fakeStep struct {
	kind    Kind
	tool    string
	res     StepResult
	ran     bool
	gotMode Mode
}

func (f *fakeStep) Kind() Kind   { return f.kind }
func (f *fakeStep) Tool() string { return f.tool }
func (f *fakeStep) Run(_ context.Context, _ LangUnit, m Mode) StepResult {
	f.ran = true
	f.gotMode = m
	return f.res
}

type fakeAdapter struct{ steps []Step }

func (a fakeAdapter) Steps() []Step { return a.steps }

// allInstalled marks every named tool present with no hint.
func allInstalled(tools ...string) map[string]ToolInfo {
	m := map[string]ToolInfo{}
	for _, t := range tools {
		m[t] = ToolInfo{Installed: true}
	}
	return m
}

// enabledVerify enables every stage so a test can pick what the adapter provides.
func enabledVerify() config.Verify {
	on := config.Step{Enabled: true, Required: true}
	return config.Verify{Format: on, Lint: on, Typecheck: on, Test: on, Build: on}
}

// The plan runs stages in canonical order regardless of adapter ordering, and only
// the stages the adapter actually provides appear.
func TestRunOrdersStagesAndOmitsUnprovided(t *testing.T) {
	adapter := fakeAdapter{steps: []Step{
		&fakeStep{kind: Test, tool: "go", res: StepResult{Status: StatusPass}},
		&fakeStep{kind: Format, tool: "fmt", res: StepResult{Status: StatusPass}},
	}}
	got := Run(context.Background(), enabledVerify(), adapter,
		LangUnit{Name: "go", Dir: "."}, allInstalled("go", "fmt"))

	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(got), got)
	}
	if got[0].Kind != Format || got[1].Kind != Test {
		t.Errorf("stages out of order: %v then %v", got[0].Kind, got[1].Kind)
	}
}

// A stage disabled in config is skipped entirely — its Step never runs.
func TestRunSkipsDisabledStage(t *testing.T) {
	lint := &fakeStep{kind: Lint, tool: "linter", res: StepResult{Status: StatusPass}}
	v := enabledVerify()
	v.Lint.Enabled = false

	got := Run(context.Background(), v, fakeAdapter{steps: []Step{lint}},
		LangUnit{Name: "go"}, allInstalled("linter"))

	if len(got) != 0 {
		t.Errorf("disabled stage produced results: %+v", got)
	}
	if lint.ran {
		t.Error("disabled stage's Step.Run was called")
	}
}

// A missing tool fails a required stage and warns (without running) an optional one;
// in neither case does the Step execute.
func TestRunMissingToolDegradesByRequired(t *testing.T) {
	required := &fakeStep{kind: Format, tool: "fmt"}
	optional := &fakeStep{kind: Lint, tool: "linter"}
	v := enabledVerify()
	v.Lint.Required = false

	got := Run(context.Background(), v, fakeAdapter{steps: []Step{required, optional}},
		LangUnit{Name: "go"}, map[string]ToolInfo{
			"fmt":    {Installed: false, Hint: "go install fmt"},
			"linter": {Installed: false},
		})

	if len(got) != 2 {
		t.Fatalf("want 2 results, got %+v", got)
	}
	if got[0].Status != StatusFail {
		t.Errorf("required missing tool: want Fail, got %v", got[0].Status)
	}
	if got[1].Status != StatusWarn {
		t.Errorf("optional missing tool: want Warn, got %v", got[1].Status)
	}
	if required.ran || optional.ran {
		t.Error("a step ran despite its tool being missing")
	}
}

// Format honors the configured fix mode; other stages always run in check mode.
func TestRunPassesFixModeToFormatter(t *testing.T) {
	fmtStep := &fakeStep{kind: Format, tool: "fmt", res: StepResult{Status: StatusPass}}
	v := enabledVerify()
	v.Format.Mode = "fix"

	Run(context.Background(), v, fakeAdapter{steps: []Step{fmtStep}},
		LangUnit{Name: "go"}, allInstalled("fmt"))

	if fmtStep.gotMode != ModeFix {
		t.Errorf("formatter mode: want ModeFix, got %v", fmtStep.gotMode)
	}
}

func TestFailedReportsAnyHardFailure(t *testing.T) {
	if Failed([]Result{{Status: StatusPass}, {Status: StatusWarn}}) {
		t.Error("warn/pass should not count as failed")
	}
	if !Failed([]Result{{Status: StatusPass}, {Status: StatusFail}}) {
		t.Error("a Fail result should mark the run failed")
	}
}
