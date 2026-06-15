package detect

func init() {
	register(langSpec{
		name: "java",
		markers: []marker{
			{"pom.xml", "maven"},
			{"build.gradle", "gradle"},
			{"build.gradle.kts", "gradle"},
		},
		tools:    []Tool{{"java", "https://adoptium.net"}},
		skipDirs: []string{"build"},
	})
}
