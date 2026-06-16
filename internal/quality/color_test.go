package quality

import (
	"context"
	"slices"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/runner"
)

// colorEnv only overrides the environment when the unit asks for color, and then it layers
// the caller's vars on top of the inherited environment (so the tool still sees PATH etc.).
func TestColorEnvForcesOnlyWhenRequested(t *testing.T) {
	if env := colorEnv(LangUnit{Color: false}, "FORCE_COLOR=1"); env != nil {
		t.Errorf("no color: want nil env (inherit), got %v", env)
	}
	if env := colorEnv(LangUnit{Color: true}); env != nil {
		t.Errorf("no vars: want nil env, got %v", env)
	}
	env := colorEnv(LangUnit{Color: true}, "FORCE_COLOR=1")
	if !slices.Contains(env, "FORCE_COLOR=1") {
		t.Errorf("color: want FORCE_COLOR=1 in env, got %v", env)
	}
	if len(env) <= 1 {
		t.Errorf("color: env should extend the inherited environment, got %v", env)
	}
}

// A color run threads each tool's own knob through to the command — cargo via an env var,
// dart via a flag — proving both wiring styles reach the runner, and that without Color
// no knob is added (so captured output stays escape-code free).
func TestColorReachesToolInvocation(t *testing.T) {
	rust := &runner.Fake{}
	stepOf("rust", rust, Lint).Run(context.Background(), LangUnit{Dir: ".", Color: true}, ModeCheck)
	if !slices.Contains(rust.Calls[0].Env, "CARGO_TERM_COLOR=always") {
		t.Errorf("rust color: want CARGO_TERM_COLOR=always in env, got %v", rust.Calls[0].Env)
	}

	pkg := dartPkg(t) // a test/ dir so the test stage runs instead of skipping
	dartOn := &runner.Fake{}
	stepOf("dart", dartOn, Test).Run(context.Background(), LangUnit{Dir: pkg, Color: true}, ModeCheck)
	if !slices.Contains(dartOn.Calls[0].Args, "--color") {
		t.Errorf("dart color: want --color in args, got %v", dartOn.Calls[0].Args)
	}

	plain := &runner.Fake{}
	stepOf("dart", plain, Test).Run(context.Background(), LangUnit{Dir: pkg}, ModeCheck)
	if plain.Calls[0].Env != nil || slices.Contains(plain.Calls[0].Args, "--color") {
		t.Errorf("no color: dart should add no knob, got args=%v env=%v", plain.Calls[0].Args, plain.Calls[0].Env)
	}
}
