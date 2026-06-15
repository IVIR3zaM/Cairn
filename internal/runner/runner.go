// Package runner is the ToolRunner port: the single way the core shells out to
// external tools. The core depends only on the interface; the Exec adapter wraps
// os/exec, and Fake serves tests. Per ARCHITECTURE, adapters never live in the core.
package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

// Command describes one external invocation. An empty Dir means the current working
// directory; a zero Timeout means no deadline.
type Command struct {
	Name    string
	Args    []string
	Dir     string
	Env     []string // nil inherits the parent environment
	Timeout time.Duration
	Stream  io.Writer // if set, stdout+stderr are also written here live (still captured)
}

// Result captures everything a caller needs to report on a run. A non-zero ExitCode
// or TimedOut is an outcome, not a Run error — only a failure to *start* the command
// (e.g. not on PATH) returns an error from Run.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// ToolRunner runs an external command and captures its outcome.
type ToolRunner interface {
	Run(ctx context.Context, cmd Command) (Result, error)
}

// Exec is the real ToolRunner backed by os/exec.
type Exec struct{}

func (Exec) Run(ctx context.Context, cmd Command) (Result, error) {
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	c.Dir = cmd.Dir
	if cmd.Env != nil {
		c.Env = cmd.Env
	}
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if cmd.Stream != nil {
		// Tee live output to the stream so a long command shows progress, while still
		// capturing it for the result Detail.
		c.Stdout = io.MultiWriter(&stdout, cmd.Stream)
		c.Stderr = io.MultiWriter(&stderr, cmd.Stream)
	}

	err := c.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		// The command could not start (missing binary, permission, etc.).
		return res, err
	}
	return res, nil
}
