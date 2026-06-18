package version

import "regexp"

// tomlVersion captures a `version = "X.Y.Z"` assignment at the start of a line — the form
// used by Cargo's [package] and PEP 621's [project]. Anchoring to the line start (and only
// the first match, via setVia) avoids picking up a dependency's version pin. Shared by the
// two TOML manifests because a second real case earns the helper (AGENTS "keep it simple").
var tomlVersion = regexp.MustCompile(`(?m)^version\s*=\s*"(` + versionToken + `)"`)

type pyproject struct{}

func init() { register(pyproject{}) }

func (pyproject) Filename() string { return "pyproject.toml" }

func (pyproject) SetVersion(content []byte, v Version) ([]byte, bool, error) {
	return setVia(content, tomlVersion, v, `version in pyproject.toml`)
}

func (pyproject) ReadVersion(content []byte) (Version, bool) {
	return readVia(content, tomlVersion)
}
