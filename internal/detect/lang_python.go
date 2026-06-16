package detect

func init() {
	register(langSpec{
		name: "python",
		markers: []marker{
			{"pyproject.toml", "pip"},
			{"setup.py", "pip"},
			{"requirements.txt", "pip"},
		},
		versionManifests: []string{"pyproject.toml"},
		tools: []Tool{
			{"ruff", "pip install ruff"},
			{"black", "pip install black"},
			{"flake8", "pip install flake8"},
			{"python3", "https://www.python.org/downloads/"},
		},
	})
}
