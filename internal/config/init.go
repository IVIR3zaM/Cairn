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
// Tree, which keeps the file to the facts that matter. The bytes always round-trip through
// LoadTree.
func InitConfig(base Directory) ([]byte, error) {
	data, err := yaml.Marshal(rootDoc{Schema: SchemaVersion, Directory: base})
	if err != nil {
		return nil, fmt.Errorf("render init config: %w", err)
	}
	return data, nil
}
