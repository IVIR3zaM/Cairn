package version

type cargo struct{}

func init() { register(cargo{}) }

func (cargo) Filename() string { return "Cargo.toml" }

func (cargo) SetVersion(content []byte, v Version) ([]byte, bool, error) {
	return setVia(content, tomlVersion, v, `version in Cargo.toml`)
}

func (cargo) ReadVersion(content []byte) (Version, bool) {
	return readVia(content, tomlVersion)
}
