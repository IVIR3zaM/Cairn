package cli

import (
	"bytes"
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

// doctor wires the Detection context to the real filesystem/PATH; the detection
// logic itself is exercised in internal/detect. Here we only confirm the command
// runs and renders without error (run fatals on a non-nil error).
func TestDoctorRuns(t *testing.T) {
	out := run(t, "doctor")
	if strings.TrimSpace(out) == "" {
		t.Errorf("doctor produced no output")
	}
}
