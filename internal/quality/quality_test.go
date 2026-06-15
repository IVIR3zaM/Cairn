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
		LangUnit{Name: "go", Dir: "."}, allInstalled("go", "fmt"), nil)

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
		LangUnit{Name: "go"}, allInstalled("linter"), nil)

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
		}, nil)

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
		LangUnit{Name: "go"}, allInstalled("fmt"), nil)

	if fmtStep.gotMode != ModeFix {
		t.Errorf("formatter mode: want ModeFix, got %v", fmtStep.gotMode)
	}
}

// ctxStep blocks until its context is cancelled, recording whether it was handed a
// deadline — standing in for a tool that would otherwise hang verify forever.
type ctxStep struct {
	kind     Kind
	tool     string
	deadline bool
}

func (s *ctxStep) Kind() Kind   { return s.kind }
func (s *ctxStep) Tool() string { return s.tool }
func (s *ctxStep) Run(ctx context.Context, _ LangUnit, _ Mode) StepResult {
	_, s.deadline = ctx.Deadline()
	<-ctx.Done() // released only when the configured timeout fires
	return StepResult{Status: StatusFail, Detail: "timed out"}
}

// A stage that would block forever is bounded by verify.timeout: its context gets a
// deadline and Run returns a failure instead of freezing.
func TestRunBoundsStageWithTimeout(t *testing.T) {
	step := &ctxStep{kind: Test, tool: "go"}
	v := enabledVerify()
	v.Timeout = "10ms"

	got := Run(context.Background(), v, fakeAdapter{steps: []Step{step}},
		LangUnit{Name: "go"}, allInstalled("go"), nil)

	if len(got) != 1 || got[0].Status != StatusFail {
		t.Fatalf("timed-out stage should fail: %+v", got)
	}
	if !step.deadline {
		t.Error("step was not handed a deadline-bounded context")
	}
}

// recordObserver captures the progress callbacks Run makes.
type recordObserver struct {
	began int
	ended []Status
}

func (o *recordObserver) Begin(LangUnit, Kind) { o.began++ }
func (o *recordObserver) End(r Result)         { o.ended = append(o.ended, r.Status) }

// Run announces every executed stage to the Observer (Begin then End) so a caller can
// show live progress; a missing-tool stage is reported via End only (it never runs).
func TestRunReportsProgressToObserver(t *testing.T) {
	steps := []Step{
		&fakeStep{kind: Format, tool: "fmt", res: StepResult{Status: StatusPass}},
		&fakeStep{kind: Test, tool: "tester"}, // tool missing below ⇒ End only
	}
	obs := &recordObserver{}
	v := enabledVerify()
	v.Test.Required = false

	Run(context.Background(), v, fakeAdapter{steps: steps}, LangUnit{Name: "go"},
		map[string]ToolInfo{"fmt": {Installed: true}, "tester": {Installed: false}}, obs)

	if obs.began != 1 {
		t.Errorf("Begin should fire once (only the runnable stage), got %d", obs.began)
	}
	if len(obs.ended) != 2 || obs.ended[0] != StatusPass || obs.ended[1] != StatusWarn {
		t.Errorf("End should report both stages [pass, warn], got %v", obs.ended)
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
