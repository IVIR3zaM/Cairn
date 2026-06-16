package detect

import "fmt"

// This file is the language extension point. It defines the shape of a language
// entry and the register() hook that language files call from their init(). To
// add a language, drop a new lang_<name>.go file that calls register(...) — no
// edits here or in the detection engine are needed (see docs/ARCHITECTURE.md
// "Adding a language or standard").

// Tool is one standard tool for a language, with an install hint shown when missing.
type Tool struct {
	Name string
	Hint string
}

// marker is a filename that signals a language, plus the package manager it implies.
type marker struct {
	file string
	pm   string
}

// langSpec is one self-contained language definition: how to detect it, what tools
// it expects, and which generated dirs a repo scan must not descend into. Everything
// Cairn needs to know about a language lives in its own lang_<name>.go file.
type langSpec struct {
	name     string
	markers  []marker
	tools    []Tool
	skipDirs []string // generated dirs (deps/build output) to ignore while scanning
	// singleRoot marks languages whose build tool owns the whole tree from one root
	// manifest (e.g. a Maven reactor / Gradle multi-project): nested manifests are
	// submodules of that build, not independent units, so detection collapses them to
	// the outermost dir and the tool's own reactor builds the rest.
	singleRoot bool
	// workspace, when set, inspects a candidate manifest and reports whether it is an
	// aggregator root (e.g. a Dart pub workspace's `workspace:` pubspec) that owns no code
	// of its own. Such a root defers to the member units nested beneath it: detection drops
	// the aggregator and keeps the members, so each is verified in its own dir. It is the
	// mirror of singleRoot, which instead collapses members into the root.
	workspace func(manifest []byte) bool
}

// registry is built at init time from each language's register() call; it is the
// single in-memory source of truth detection iterates over.
var registry []langSpec

// skipDirs is the union of every language's generated dirs plus VCS metadata. A scan
// never descends into these. It is assembled from registrations so each language owns
// the dirs it creates.
var skipDirs = map[string]bool{".git": true}

// register adds one language to the registry. It panics on a duplicate name so a
// copy-paste mistake fails loudly at startup rather than silently shadowing a language.
func register(spec langSpec) {
	for _, existing := range registry {
		if existing.name == spec.name {
			panic(fmt.Sprintf("detect: language %q registered twice", spec.name))
		}
	}
	registry = append(registry, spec)
	for _, d := range spec.skipDirs {
		skipDirs[d] = true
	}
}
