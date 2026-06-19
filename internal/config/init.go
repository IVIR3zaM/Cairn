package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// InitConfig renders the schema-2 cairn.yaml `cairn init` writes from the settings it
// discovered for this repo. It records only what detection positively determined — the project
// version, the languages present, version_sync patterns found in the docs, a commits policy
// learned from history — and never a blind default: any field the discovery left unset (a nil
// pointer or empty map on base) is omitted so it rides the in-code default via the resolved
// Tree, which keeps the file to the facts that matter. dirs carries the per-directory override
// blocks the wizard collected for sub-units that need their own rules (independent version,
// overridden standards, or disablement); an empty map omits the `directories:` key entirely so a
// single-package repo stays a flat baseline. comments maps a top-level key (version, languages,
// commits, …) to the explanatory header written above it, so the file is self-documenting; a nil
// map renders the bare YAML. The bytes always round-trip through LoadTree.
func InitConfig(base Directory, dirs map[string]Directory, comments map[string]string) ([]byte, error) {
	if len(dirs) == 0 {
		dirs = nil
	}
	base = omitDefaultChangelogFile(base)
	for d := range dirs {
		dirs[d] = omitDefaultChangelogFile(dirs[d])
	}

	var node yaml.Node
	if err := node.Encode(rootDoc{Schema: SchemaVersion, Directory: base, Directories: dirs}); err != nil {
		return nil, fmt.Errorf("render init config: %w", err)
	}
	annotate(&node, comments)

	data, err := yaml.Marshal(&node)
	if err != nil {
		return nil, fmt.Errorf("render init config: %w", err)
	}
	return data, nil
}

// omitDefaultChangelogFile blanks a changelog block's File when it equals the in-code default
// (CHANGELOG.md), so the rendered config carries the standard alone and lets the file ride the
// default — the resolver fills it back in (see resolvedDirChangelog). A non-default file is kept.
func omitDefaultChangelogFile(d Directory) Directory {
	if d.Changelog != nil && d.Changelog.File == Default().Changelog.File {
		cl := *d.Changelog
		cl.File = ""
		d.Changelog = &cl
	}
	return d
}

// annotate attaches each comment to the matching top-level key of the rendered mapping as a head
// comment, so the written cairn.yaml explains why every block is there and what else it accepts.
// Keys with no entry in comments are left bare. A nil/empty map is a no-op.
func annotate(doc *yaml.Node, comments map[string]string) {
	if len(comments) == 0 {
		return
	}
	m := doc
	if m.Kind == yaml.DocumentNode && len(m.Content) > 0 {
		m = m.Content[0]
	}
	if m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i]
		if c, ok := comments[key.Value]; ok {
			key.HeadComment = c
		}
	}
}
