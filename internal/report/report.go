// Package report is the UX/Reporter port: colorful but concise rendering of progress
// and a compact summary. One implementation serves both the TTY and plain CI cases,
// parameterized by Options, so there is no abstraction the second case has not earned.
package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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

// Reporter renders progress and a compact summary. Running announces a step that is
// about to run and returns a function to call with its final result.
type Reporter interface {
	Start(title string)
	Running(name string) func(s Step)
	Step(s Step)
	Summary(steps []Step)
	Error(err error)
}

// Options control rendering. Color toggles ANSI; Quiet suppresses per-step lines
// (the summary and errors still print); Verbose always shows Detail; TTY enables the
// live elapsed-time indicator (off when output is piped, quiet, or verbose).
type Options struct {
	Color   bool
	Quiet   bool
	Verbose bool
	TTY     bool
}

// Detect derives Options from the writer and environment: color only on a TTY with
// NO_COLOR unset. Quiet and Verbose come from CLI flags.
func Detect(w io.Writer, quiet, verbose bool) Options {
	tty := isTerminal(w)
	return Options{
		Color:   tty && os.Getenv("NO_COLOR") == "",
		Quiet:   quiet,
		Verbose: verbose,
		TTY:     tty,
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

// Running announces that step `name` has begun and returns a done(Step) to call with its
// outcome. On an interactive TTY it animates a spinner with elapsed seconds in place
// ("⠙ name… 12s") so a long-running tool never looks frozen; done clears it and prints
// the final result line. When piped, quiet, or verbose, it falls back to printing the
// result line on done (verbose already streams the tool's own output).
func (r *reporter) Running(name string) func(Step) {
	if r.opt.Quiet {
		return func(Step) {}
	}
	if !r.opt.TTY || r.opt.Verbose {
		return r.Step
	}

	stop, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		tick := time.NewTicker(120 * time.Millisecond)
		defer tick.Stop()
		start := time.Now()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-tick.C:
				fmt.Fprintf(r.w, "\r\033[2K  %s %s… %ds",
					r.paint(colorDim, string(frames[i%len(frames)])), name, int(time.Since(start).Seconds()))
			}
		}
	}()

	return func(s Step) {
		close(stop)
		<-finished
		fmt.Fprint(r.w, "\r\033[2K") // clear the spinner line before the result
		r.Step(s)
	}
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
