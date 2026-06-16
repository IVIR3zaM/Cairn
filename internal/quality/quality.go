// Package quality is the QualityGate bounded context: it builds an ordered verify
// plan for a language and runs each step through its adapter, collecting results.
// The core knows the run order and how a missing tool degrades a stage; adapters
// (e.g. internal/quality/go) know how to invoke a concrete tool. The core imports no
// tool — only config and the runner port via its adapters.
package quality

import (
	"context"
	"fmt"
	"time"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

// Kind is a verify stage. The order slice below is the canonical run order.
type Kind int

const (
	Format Kind = iota
	Lint
	Typecheck
	Test
	Build
)

func (k Kind) String() string {
	switch k {
	case Format:
		return "format"
	case Lint:
		return "lint"
	case Typecheck:
		return "typecheck"
	case Test:
		return "test"
	case Build:
		return "build"
	default:
		return "?"
	}
}

// order is the canonical sequence stages run in: format → lint → typecheck → test → build.
var order = []Kind{Format, Lint, Typecheck, Test, Build}

// Mode selects whether a formatter checks or rewrites.
type Mode int

const (
	ModeCheck Mode = iota
	ModeFix
)

// Status is how a stage ended (domain-side; the CLI maps it to report glyphs).
type Status int

const (
	StatusPass Status = iota
	StatusFail
	StatusSkip
	StatusWarn
)

// LangUnit is the language instance under verification. Color asks adapters to force
// their tools' colored output (set by the CLI for a verbose run on a color TTY); the
// per-tool knob that honors it lives in each lang_<name>.go, not in the gate core.
// Strict asks adapters to run their tools at maximum severity — promoting analyzer
// infos / linter warnings to failures — where the toolchain offers such a switch;
// like Color, the per-tool flag lives in each lang_<name>.go. The gate core resolves
// neither knob, it only carries them.
type LangUnit struct {
	Name   string
	Dir    string
	Color  bool
	Strict bool
}

// StepResult is what a single Step.Run reports.
type StepResult struct {
	Status Status
	Detail string
}

// Step is one quality stage for a language. Adapters implement it per tool.
type Step interface {
	Kind() Kind
	Tool() string // the executable this stage needs; matched against ToolInfo
	Run(ctx context.Context, unit LangUnit, mode Mode) StepResult
}

// Adapter is a language's full set of quality steps.
type Adapter interface {
	Steps() []Step
}

// ToolInfo tells the gate whether a stage's tool is installed and how to get it.
type ToolInfo struct {
	Installed bool
	Hint      string
}

// Result is one line of the verify outcome. Dir is the unit's directory, so callers can
// distinguish same-language units in different parts of the repo.
type Result struct {
	Kind   Kind
	Lang   string
	Dir    string
	Status Status
	Detail string
}

// Observer is notified as each stage begins and ends so the caller can render live
// progress (a long tool never looks frozen). A nil Observer disables progress.
type Observer interface {
	Begin(unit LangUnit, k Kind) // a stage is about to run
	End(r Result)                // a stage produced a result
}

// Run builds the ordered plan for unit and executes it. A stage disabled in config,
// or one the adapter does not provide, is omitted. A missing tool fails the stage when
// required, otherwise warns and skips it (with an install hint). Each executed stage is
// announced to obs before it runs and reported after, so callers can show progress.
func Run(ctx context.Context, v config.Verify, a Adapter, unit LangUnit, tools map[string]ToolInfo, obs Observer) []Result {
	byKind := make(map[Kind]Step, len(a.Steps()))
	for _, s := range a.Steps() {
		byKind[s.Kind()] = s
	}

	emit := func(r Result) Result {
		if obs != nil {
			obs.End(r)
		}
		return r
	}

	var results []Result
	for _, k := range order {
		step, ok := byKind[k]
		if !ok {
			continue // the language has no tool for this stage
		}
		sc := stageConfig(v, k)
		if !sc.Enabled {
			continue
		}
		if info := tools[step.Tool()]; !info.Installed {
			results = append(results, emit(missing(k, unit, step.Tool(), info.Hint, sc.Required)))
			continue
		}
		if obs != nil {
			obs.Begin(unit, k)
		}
		mode := ModeCheck
		if k == Format && sc.Mode == "fix" {
			mode = ModeFix
		}
		r := runWithTimeout(ctx, v.StepTimeout(), step, unit, mode)
		results = append(results, emit(Result{Kind: k, Lang: unit.Name, Dir: unit.Dir, Status: r.Status, Detail: r.Detail}))
	}
	return results
}

// runWithTimeout bounds a single stage with timeout when set, so a hung tool (e.g. a
// build downloading dependencies) is cancelled and reported instead of freezing verify.
// The runner.Exec adapter turns the cancelled context into a "timed out" failure.
func runWithTimeout(ctx context.Context, timeout time.Duration, step Step, unit LangUnit, mode Mode) StepResult {
	if timeout <= 0 {
		return step.Run(ctx, unit, mode)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return step.Run(ctx, unit, mode)
}

// missing renders the result for a stage whose tool is not installed.
func missing(k Kind, unit LangUnit, tool, hint string, required bool) Result {
	detail := fmt.Sprintf("%s not installed", tool)
	if hint != "" {
		detail += " — install: " + hint
	}
	status := StatusWarn
	if required {
		status = StatusFail
	}
	return Result{Kind: k, Lang: unit.Name, Dir: unit.Dir, Status: status, Detail: detail}
}

func stageConfig(v config.Verify, k Kind) config.Step {
	switch k {
	case Format:
		return v.Format
	case Lint:
		return v.Lint
	case Typecheck:
		return v.Typecheck
	case Test:
		return v.Test
	case Build:
		return v.Build
	default:
		return config.Step{}
	}
}

// Failed reports whether any result is a hard failure (drives the exit code).
func Failed(results []Result) bool {
	for _, r := range results {
		if r.Status == StatusFail {
			return true
		}
	}
	return false
}
