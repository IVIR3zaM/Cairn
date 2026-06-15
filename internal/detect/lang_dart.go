package detect

func init() {
	register(langSpec{
		name:     "dart",
		markers:  []marker{{"pubspec.yaml", "pub"}},
		tools:    []Tool{{"dart", "https://dart.dev/get-dart"}},
		skipDirs: []string{".dart_tool"},
	})
}
