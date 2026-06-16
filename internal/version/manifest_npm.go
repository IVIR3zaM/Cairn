package version

import "regexp"

// npmVersion captures the value of the top-level "version" key in a package.json.
// JSON has no comments and a single quoting style, so a regex on the key is safe glue.
var npmVersion = regexp.MustCompile(`"version"\s*:\s*"(` + versionToken + `)"`)

type npm struct{}

func init() { register(npm{}) }

func (npm) Filename() string { return "package.json" }

func (npm) SetVersion(content []byte, v Version) ([]byte, bool, error) {
	return setVia(content, npmVersion, v, `"version" in package.json`)
}
