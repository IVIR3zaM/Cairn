package quality

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// The standard selects the build tool and its non-interactive verification command:
// maven (default) runs `mvn -B verify`, gradle runs `gradle --console=plain check`. Both
// gate on `java` so a wrapper-only project still verifies without a global mvn/gradle.
func TestJavaRunsBuildLifecycleByStandard(t *testing.T) {
	cases := map[string]struct {
		bin   string
		batch string
		phase string
	}{
		"":       {"mvn", "-B", "verify"},
		"maven":  {"mvn", "-B", "verify"},
		"gradle": {"gradle", "--console=plain", "check"},
	}
	for standard, want := range cases {
		f := &runner.Fake{}
		step := stepOfWithStandard("java", standard, f, Test)
		if step == nil || step.Tool() != "java" {
			t.Fatalf("standard %q: want gate tool java, got %v", standard, step)
		}
		step.Run(context.Background(), LangUnit{Dir: t.TempDir()}, ModeCheck) // no wrapper ⇒ bare tool
		if len(f.Calls) != 1 || f.Calls[0].Name != want.bin {
			t.Fatalf("standard %q: ran %+v, want %s", standard, f.Calls, want.bin)
		}
		if !argHas(f.Calls[0].Args, want.batch) || !argHas(f.Calls[0].Args, want.phase) {
			t.Errorf("standard %q: args %v missing %q/%q", standard, f.Calls[0].Args, want.batch, want.phase)
		}
	}
}

// The build tool's exit code drives the stage: non-zero fails, zero passes.
func TestJavaExitCodeDrivesStage(t *testing.T) {
	fail := &runner.Fake{Results: map[string]runner.Result{"mvn": {ExitCode: 1, Stderr: "boom"}}}
	if got := runStepWithStandard("java", "maven", fail, Test, ModeCheck); got.Status != StatusFail {
		t.Errorf("non-zero exit: want Fail, got %v", got.Status)
	}
	if got := runStepWithStandard("java", "maven", &runner.Fake{}, Test, ModeCheck); got.Status != StatusPass {
		t.Errorf("zero exit: want Pass, got %v (%s)", got.Status, got.Detail)
	}
}

// A committed wrapper is preferred over the global tool so the project's pinned build
// version is used; its absolute path is invoked.
func TestJavaPrefersCommittedWrapper(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "mvnw")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &runner.Fake{}
	stepOfWithStandard("java", "maven", f, Test).Run(context.Background(), LangUnit{Dir: dir}, ModeCheck)
	if len(f.Calls) != 1 || f.Calls[0].Name != wrapper {
		t.Fatalf("want wrapper %q invoked, got %+v", wrapper, f.Calls)
	}
}

func argHas(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
