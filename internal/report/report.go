// Package report is the UX/Reporter port: colorful but concise rendering of progress
// and a compact summary. One implementation serves both the TTY and plain CI cases,
// parameterized by Options, so there is no abstraction the second case has not earned.
package report

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Status is how a single step ended. Glyphs follow AGENTS.md: ✓ ✗ ⊘ !.
type Status int

const (
	Pass Status = iota
	Fail
	Skip
	Warn
)

func (s Status) glyph() string {
	switch s {
	case Pass:
		return "✓"
	case Fail:
		return "✗"
	case Skip:
		return "⊘"
	case Warn:
		return "!"
	default:
		return "?"
	}
}

func (s Status) color() string {
	switch s {
	case Pass:
		return colorGreen
	case Fail:
		return colorRed
	case Skip:
		return colorDim
	case Warn:
		return colorYellow
	default:
		return colorReset
	}
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorDim    = "\033[2m"
)

// Step is one line in a report: a named unit of work and how it ended. Detail holds
// tool output or a hint, shown on failure or when Verbose is set.
type Step struct {
	Name   string
	Status Status
	Detail string
}

// Reporter renders progress and a compact summary.
type Reporter interface {
	Start(title string)
	Step(s Step)
	Summary(steps []Step)
	Error(err error)
}

// Options control rendering. Color toggles ANSI; Quiet suppresses per-step lines
// (the summary and errors still print); Verbose always shows Detail.
type Options struct {
	Color   bool
	Quiet   bool
	Verbose bool
}

// Detect derives Options from the writer and environment: color only on a TTY with
// NO_COLOR unset. Quiet and Verbose come from CLI flags.
func Detect(w io.Writer, quiet, verbose bool) Options {
	return Options{
		Color:   isTerminal(w) && os.Getenv("NO_COLOR") == "",
		Quiet:   quiet,
		Verbose: verbose,
	}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

type reporter struct {
	w   io.Writer
	opt Options
}

// New builds a Reporter writing to w with the given Options.
func New(w io.Writer, opt Options) Reporter {
	return &reporter{w: w, opt: opt}
}

func (r *reporter) paint(color, s string) string {
	if !r.opt.Color {
		return s
	}
	return color + s + colorReset
}

func (r *reporter) Start(title string) {
	if r.opt.Quiet {
		return
	}
	fmt.Fprintln(r.w, title)
}

func (r *reporter) Step(s Step) {
	if r.opt.Quiet {
		return
	}
	fmt.Fprintf(r.w, "  %s %s\n", r.paint(s.Status.color(), s.Status.glyph()), s.Name)
	if s.Detail != "" && (r.opt.Verbose || s.Status == Fail) {
		for _, line := range strings.Split(strings.TrimRight(s.Detail, "\n"), "\n") {
			fmt.Fprintf(r.w, "      %s\n", line)
		}
	}
}

func (r *reporter) Summary(steps []Step) {
	var pass, fail, skip, warn int
	for _, s := range steps {
		switch s.Status {
		case Pass:
			pass++
		case Fail:
			fail++
		case Skip:
			skip++
		case Warn:
			warn++
		}
	}

	parts := []string{fmt.Sprintf("%d passed", pass)}
	if fail > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", fail))
	}
	if warn > 0 {
		parts = append(parts, fmt.Sprintf("%d warned", warn))
	}
	if skip > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skip))
	}

	overall := Pass
	switch {
	case fail > 0:
		overall = Fail
	case warn > 0:
		overall = Warn
	}
	fmt.Fprintf(r.w, "%s %s\n", r.paint(overall.color(), overall.glyph()), strings.Join(parts, ", "))
}

func (r *reporter) Error(err error) {
	fmt.Fprintf(r.w, "%s %s\n", r.paint(colorRed, "✗"), err.Error())
}
