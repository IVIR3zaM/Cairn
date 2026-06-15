package runner

import (
	"context"
	"testing"
	"time"
)

func TestExecCapturesOutputAndExitZero(t *testing.T) {
	res, err := Exec{}.Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", "printf out; printf err 1>&2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stdout != "out" || res.Stderr != "err" {
		t.Fatalf("captured stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.ExitCode)
	}
}

func TestExecReportsNonZeroExit(t *testing.T) {
	res, err := Exec{}.Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", "exit 3"},
	})
	if err != nil {
		t.Fatalf("a non-zero exit is an outcome, not a Run error: %v", err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", res.ExitCode)
	}
}

func TestExecErrorsWhenCommandMissing(t *testing.T) {
	if _, err := (Exec{}).Run(context.Background(), Command{Name: "cairn-no-such-binary"}); err == nil {
		t.Fatal("expected an error when the command cannot start")
	}
}

func TestExecTimesOut(t *testing.T) {
	res, err := Exec{}.Run(context.Background(), Command{
		Name: "sh", Args: []string{"-c", "sleep 5"}, Timeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("a timeout is an outcome, not a Run error: %v", err)
	}
	if !res.TimedOut {
		t.Fatal("expected TimedOut to be true")
	}
}
