package detect

func init() {
	register(langSpec{
		name:             "javascript",
		markers:          []marker{{"package.json", "npm"}},
		versionManifests: []string{"package.json"},
		tools: []Tool{
			{"node", "https://nodejs.org"},
			{"npx", "https://nodejs.org"},
		},
		skipDirs: []string{"node_modules"},
	})
}
