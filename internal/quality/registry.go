package quality

import "github.com/IVIR3zaM/Cairn/internal/runner"

// registry holds each language's adapter constructor, populated by the init() in its
// lang_<name>.go file. It mirrors the detection registry (internal/detect): adding a
// language is dropping one self-registering file — no central list, no edits here or in
// the CLI. register panics on a duplicate name to catch copy-paste mistakes at startup.
var registry = map[string]func(runner.ToolRunner, string) Adapter{}

// register wires a language's adapter constructor. Call it from a lang_<name>.go init().
func register(name string, ctor func(runner.ToolRunner, string) Adapter) {
	if _, dup := registry[name]; dup {
		panic("quality: duplicate adapter registered for " + name)
	}
	registry[name] = ctor
}

// AdapterFor returns the adapter for a detected language backed by run with the given
// standard (e.g. "ruff" for Python), or false when no adapter is registered for that
// language yet. Standard is language-specific and can be empty if not applicable.
func AdapterFor(name string, run runner.ToolRunner, standard string) (Adapter, bool) {
	ctor, ok := registry[name]
	if !ok {
		return nil, false
	}
	return ctor(run, standard), true
}
