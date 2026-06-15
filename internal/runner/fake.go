package runner

import "context"

// Fake is a scripted ToolRunner for tests. It returns canned Results keyed by the
// command Name and records every invocation so callers can assert on what ran.
type Fake struct {
	Results map[string]Result // keyed by Command.Name
	Err     error             // if set, returned for every command
	Calls   []Command
}

func (f *Fake) Run(_ context.Context, cmd Command) (Result, error) {
	f.Calls = append(f.Calls, cmd)
	if f.Err != nil {
		return Result{}, f.Err
	}
	return f.Results[cmd.Name], nil
}
