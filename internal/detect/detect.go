// Package detect is the Detection bounded context: it scans a repository to find
// which languages are present (and in which dirs), the package manager each uses,
// and which of that language's standard tools are installed on the machine.
//
// It depends only on an fs.FS (the repo) and a LookupFunc (tool resolution) so it
// stays pure and is testable with a fake filesystem and lookup. The language
// registry below is the single source of truth shared by detection and docs.
package detect

import (
	"io/fs"
	"path"
	"sort"
)

// LookupFunc resolves an executable name to a path, matching exec.LookPath. A
// non-nil error means the tool is not installed.
type LookupFunc func(string) (string, error)

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

// langSpec is a registry entry: how to detect a language and what it expects.
type langSpec struct {
	name    string
	markers []marker
	tools   []Tool
}

// registry is the single source of truth for language detection. Adding a language
// is one entry here (see docs/ARCHITECTURE.md "Adding a language or standard").
var registry = []langSpec{
	{
		name:    "go",
		markers: []marker{{"go.mod", "go modules"}},
		tools: []Tool{
			{"gofumpt", "go install mvdan.cc/gofumpt@latest"},
			{"golangci-lint", "https://golangci-lint.run/usage/install/"},
			{"go", "https://go.dev/dl/"},
		},
	},
	{
		name: "python",
		markers: []marker{
			{"pyproject.toml", "pip"},
			{"setup.py", "pip"},
			{"requirements.txt", "pip"},
		},
		tools: []Tool{
			{"ruff", "pip install ruff"},
			{"python3", "https://www.python.org/downloads/"},
		},
	},
	{
		name:    "rust",
		markers: []marker{{"Cargo.toml", "cargo"}},
		tools: []Tool{
			{"cargo", "https://rustup.rs"},
			{"rustfmt", "rustup component add rustfmt"},
			{"clippy-driver", "rustup component add clippy"},
		},
	},
	{
		name:    "javascript",
		markers: []marker{{"package.json", "npm"}},
		tools: []Tool{
			{"node", "https://nodejs.org"},
			{"npx", "https://nodejs.org"},
		},
	},
	{
		name: "java",
		markers: []marker{
			{"pom.xml", "maven"},
			{"build.gradle", "gradle"},
			{"build.gradle.kts", "gradle"},
		},
		tools: []Tool{{"java", "https://adoptium.net"}},
	},
	{
		name:    "dart",
		markers: []marker{{"pubspec.yaml", "pub"}},
		tools:   []Tool{{"dart", "https://dart.dev/get-dart"}},
	},
}

// skipDirs are never descended into during a scan: VCS metadata and build/dep output.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"target":       true,
	"build":        true,
	".dart_tool":   true,
}

// ToolStatus pairs a language's tool with whether it was found on the machine.
type ToolStatus struct {
	Tool      Tool
	Installed bool
}

// Language is one detected language unit: where it lives, its package manager, and
// the install status of each of its standard tools.
type Language struct {
	Name           string
	Dir            string
	PackageManager string
	Tools          []ToolStatus
}

// Result is the full detection outcome, languages sorted by (name, dir).
type Result struct {
	Languages []Language
}

// Detect scans fsys for language markers and resolves each language's tools with
// look. The same tool is looked up at most once. Common build/dep dirs are skipped.
func Detect(fsys fs.FS, look LookupFunc) (*Result, error) {
	type key struct{ name, dir string }
	pmByUnit := map[key]string{}

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		for _, spec := range registry {
			for _, m := range spec.markers {
				if m.file != d.Name() {
					continue
				}
				k := key{spec.name, path.Dir(p)}
				if _, seen := pmByUnit[k]; !seen {
					pmByUnit[k] = m.pm
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	installed := map[string]bool{}
	lookCached := func(name string) bool {
		if v, ok := installed[name]; ok {
			return v
		}
		_, lookErr := look(name)
		v := lookErr == nil
		installed[name] = v
		return v
	}

	specByName := map[string]langSpec{}
	for _, s := range registry {
		specByName[s.name] = s
	}

	langs := make([]Language, 0, len(pmByUnit))
	for k, pm := range pmByUnit {
		spec := specByName[k.name]
		tools := make([]ToolStatus, 0, len(spec.tools))
		for _, t := range spec.tools {
			tools = append(tools, ToolStatus{Tool: t, Installed: lookCached(t.Name)})
		}
		langs = append(langs, Language{
			Name:           k.name,
			Dir:            k.dir,
			PackageManager: pm,
			Tools:          tools,
		})
	}
	sort.Slice(langs, func(i, j int) bool {
		if langs[i].Name != langs[j].Name {
			return langs[i].Name < langs[j].Name
		}
		return langs[i].Dir < langs[j].Dir
	})
	return &Result{Languages: langs}, nil
}
