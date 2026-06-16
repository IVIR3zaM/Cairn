package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// run executes the root command with args and returns combined stdout/stderr.
func run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(%v) returned error: %v", args, err)
	}
	return buf.String()
}

func TestVersionFlagPrints(t *testing.T) {
	out := run(t, "--version")
	if !strings.Contains(out, version) {
		t.Errorf("--version output %q does not contain version %q", out, version)
	}
}

// TestReportError pins the error-surfacing contract the CLI depends on: a real failure
// (e.g. bump's unset-canonical guard) must reach the user, while an already-reported
// failure (verify's rendered summary) stays silent so it isn't printed twice. Regression
// for bump exiting non-zero with no message at all.
func TestReportError(t *testing.T) {
	var real bytes.Buffer
	reportError(&real, fmt.Errorf("boom"))
	if !strings.Contains(real.String(), "boom") {
		t.Errorf("real error not surfaced: %q", real.String())
	}

	var silent bytes.Buffer
	reportError(&silent, fmt.Errorf("wrapped: %w", errSilent))
	if silent.Len() != 0 {
		t.Errorf("already-reported error should be silent, got %q", silent.String())
	}

	var none bytes.Buffer
	reportError(&none, nil)
	if none.Len() != 0 {
		t.Errorf("nil error should print nothing, got %q", none.String())
	}
}

// doctor wires the Detection context to the real filesystem/PATH; the detection
// logic itself is exercised in internal/detect. Here we only confirm the command
// runs and renders without error (run fatals on a non-nil error).
func TestDoctorRuns(t *testing.T) {
	out := run(t, "doctor")
	if strings.TrimSpace(out) == "" {
		t.Errorf("doctor produced no output")
	}
}
