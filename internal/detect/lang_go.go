package detect

func init() {
	register(langSpec{
		name:    "go",
		markers: []marker{{"go.mod", "go modules"}},
		tools: []Tool{
			{"gofumpt", "go install mvdan.cc/gofumpt@latest"},
			{"golangci-lint", "https://golangci-lint.run/usage/install/"},
			{"go", "https://go.dev/dl/"},
		},
		skipDirs: []string{"vendor"},
	})
}
