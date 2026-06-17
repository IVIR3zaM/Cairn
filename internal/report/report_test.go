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

// A passing step never shows a fix hint, and a complete (formatter) fixer is advertised
// as a sure thing naming both the tool command and --fix.
func TestFixHintCompleteShownOnFailOnly(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, Options{})
	r.Step(Step{Name: "ok", Status: Pass, Fix: "gofumpt -w ."})
	r.Step(Step{Name: "fmt", Status: Fail, Detail: "trace", Fix: "gofumpt -w ."})
	out := buf.String()
	if strings.Count(out, "auto-fixable") != 1 {
		t.Errorf("fix hint should appear once (failing step only):\n%s", out)
	}
	if !strings.Contains(out, "gofumpt -w .") || !strings.Contains(out, "cairn verify --fix") {
		t.Errorf("fix hint should name both the tool command and --fix:\n%s", out)
	}
}

// A partial (linter) fixer is hedged — it must not promise a clean run, since findings
// like staticcheck SA* have no autofix.
func TestFixHintPartialIsHedged(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{}).Step(Step{Name: "lint", Status: Fail, Detail: "SA6001", Fix: "golangci-lint run --fix", FixPartial: true})
	out := buf.String()
	if !strings.Contains(out, "may be auto-fixable") || !strings.Contains(out, "the rest need a manual fix") {
		t.Errorf("partial fixer should be hedged, not promised:\n%s", out)
	}
}

// Once --fix has already run, a surviving failure points at a manual fix instead of
// re-suggesting the same command that just failed to resolve it.
func TestFixHintAppliedAsksForManualFix(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{}).Step(Step{Name: "lint", Status: Fail, Detail: "SA6001", Fix: "golangci-lint run --fix", FixPartial: true, FixApplied: true})
	out := buf.String()
	if !strings.Contains(out, "auto-fix already ran") || !strings.Contains(out, "manual fix") {
		t.Errorf("post-fix failure should ask for a manual fix:\n%s", out)
	}
	if strings.Contains(out, "cairn verify --fix") {
		t.Errorf("post-fix failure should not re-suggest --fix:\n%s", out)
	}
}

// Without a TTY, Running animates nothing — done(s) just prints the final result line,
// keeping piped/CI output free of cursor-control escapes.
func TestRunningFallsBackToResultLineWhenNotTTY(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf, Options{}) // TTY false
	r.Running("go · test")(Step{Name: "go · test", Status: Pass})
	out := buf.String()
	if !strings.Contains(out, "✓ go · test") {
		t.Errorf("done should print the result line: %q", out)
	}
	if strings.Contains(out, "\033") || strings.Contains(out, "\r") {
		t.Errorf("non-TTY Running must not emit cursor controls: %q", out)
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
