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

func TestDoctorStub(t *testing.T) {
	out := run(t, "doctor")
	if !strings.Contains(out, "not implemented") {
		t.Errorf("doctor output %q missing stub marker", out)
	}
}
