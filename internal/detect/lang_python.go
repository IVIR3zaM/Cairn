package detect

func init() {
	register(langSpec{
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
	})
}
