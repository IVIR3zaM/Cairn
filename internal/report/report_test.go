package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestSummaryTallyIsCompactAndOrdered(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{}).Summary([]Step{
		{Name: "fmt", Status: Pass},
		{Name: "lint", Status: Fail},
		{Name: "type", Status: Skip},
		{Name: "test", Status: Pass},
	})
	const want = "✗ 2 passed, 1 failed, 1 skipped\n"
	if got := buf.String(); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestPlainModeHasNoANSI(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, Options{Color: false})
	r.Start("verify")
	r.Step(Step{Name: "lint", Status: Fail, Detail: "boom"})
	r.Summary([]Step{{Name: "lint", Status: Fail}})
	if strings.Contains(buf.String(), "\033") {
		t.Fatalf("plain mode must not emit ANSI: %q", buf.String())
	}
}

func TestColorModePaintsStatus(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{Color: true}).Step(Step{Name: "fmt", Status: Pass})
	if !strings.Contains(buf.String(), colorGreen) {
		t.Fatalf("color mode should paint a passing step green: %q", buf.String())
	}
}

func TestDetailShownOnFailHiddenForPass(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, Options{})
	r.Step(Step{Name: "ok", Status: Pass, Detail: "secret"})
	r.Step(Step{Name: "bad", Status: Fail, Detail: "trace"})
	out := buf.String()
	if strings.Contains(out, "secret") {
		t.Error("a passing step's detail should be hidden without Verbose")
	}
	if !strings.Contains(out, "trace") {
		t.Error("a failing step's detail should always be shown")
	}
}

func TestQuietSuppressesStepsButKeepsSummary(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, Options{Quiet: true})
	r.Start("verify")
	r.Step(Step{Name: "fmt", Status: Pass})
	r.Summary([]Step{{Name: "fmt", Status: Pass}})
	out := buf.String()
	if strings.Contains(out, "fmt") || strings.Contains(out, "verify") {
		t.Errorf("quiet should suppress start/step lines: %q", out)
	}
	if !strings.Contains(out, "1 passed") {
		t.Errorf("quiet should keep the summary: %q", out)
	}
}
