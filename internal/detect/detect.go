// Package detect is the Detection bounded context: it scans a repository to find
// which languages are present (and in which dirs), the package manager each uses,
// and which of that language's standard tools are installed on the machine.
//
// It depends only on an fs.FS (the repo) and a LookupFunc (tool resolution) so it
// stays pure and is testable with a fake filesystem and lookup. Languages are not
// hardcoded here: each self-registers from its own lang_<name>.go file via the
// registry (see registry.go and docs/ARCHITECTURE.md "Adding a language").
package detect

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

// LookupFunc resolves an executable name to a path, matching exec.LookPath. A
// non-nil error means the tool is not installed.
type LookupFunc func(string) (string, error)

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

	// nestedUnit reports whether dir sits under another detected unit of the same
	// language — i.e. a submodule of that unit's build root.
	nestedUnit := func(name, dir string) bool {
		for kk := range pmByUnit {
			if kk.name == name && kk.dir != dir && isUnder(dir, kk.dir) {
				return true
			}
		}
		return false
	}

	langs := make([]Language, 0, len(pmByUnit))
	for k, pm := range pmByUnit {
		spec := specByName[k.name]
		// For single-root build tools, a nested manifest is a submodule of an ancestor
		// unit's reactor, not its own unit — skip it so the build runs once at the root.
		if spec.singleRoot && nestedUnit(k.name, k.dir) {
			continue
		}
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

// isUnder reports whether child is nested inside parent (not equal). The repo root "."
// is an ancestor of every other dir.
func isUnder(child, parent string) bool {
	if child == parent {
		return false
	}
	if parent == "." {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}
