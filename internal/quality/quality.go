// Package quality is the QualityGate bounded context: it builds an ordered verify
// plan for a language and runs each step through its adapter, collecting results.
// The core knows the run order and how a missing tool degrades a stage; adapters
// (e.g. internal/quality/go) know how to invoke a concrete tool. The core imports no
// tool — only config and the runner port via its adapters.
package quality

import (
	"context"
	"fmt"

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

// LangUnit is the language instance under verification.
type LangUnit struct {
	Name string
	Dir  string
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

// Result is one line of the verify outcome.
type Result struct {
	Kind   Kind
	Lang   string
	Status Status
	Detail string
}

// Run builds the ordered plan for unit and executes it. A stage disabled in config,
// or one the adapter does not provide, is omitted. A missing tool fails the stage when
// required, otherwise warns and skips it (with an install hint).
func Run(ctx context.Context, v config.Verify, a Adapter, unit LangUnit, tools map[string]ToolInfo) []Result {
	byKind := make(map[Kind]Step, len(a.Steps()))
	for _, s := range a.Steps() {
		byKind[s.Kind()] = s
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
			results = append(results, missing(k, unit.Name, step.Tool(), info.Hint, sc.Required))
			continue
		}
		mode := ModeCheck
		if k == Format && sc.Mode == "fix" {
			mode = ModeFix
		}
		r := step.Run(ctx, unit, mode)
		results = append(results, Result{Kind: k, Lang: unit.Name, Status: r.Status, Detail: r.Detail})
	}
	return results
}

// missing renders the result for a stage whose tool is not installed.
func missing(k Kind, lang, tool, hint string, required bool) Result {
	detail := fmt.Sprintf("%s not installed", tool)
	if hint != "" {
		detail += " — install: " + hint
	}
	status := StatusWarn
	if required {
		status = StatusFail
	}
	return Result{Kind: k, Lang: lang, Status: status, Detail: detail}
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
