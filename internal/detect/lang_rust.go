package detect

func init() {
	register(langSpec{
		name:    "rust",
		markers: []marker{{"Cargo.toml", "cargo"}},
		tools: []Tool{
			{"cargo", "https://rustup.rs"},
			{"rustfmt", "rustup component add rustfmt"},
			{"clippy-driver", "rustup component add clippy"},
		},
		skipDirs: []string{"target"},
	})
}
