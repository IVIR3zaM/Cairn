package detect

func init() {
	register(langSpec{
		name: "java",
		markers: []marker{
			{"pom.xml", "maven"},
			{"build.gradle", "gradle"},
			{"build.gradle.kts", "gradle"},
		},
		// The build tool ships as a committed wrapper (mvnw/gradlew) in most projects, so
		// the JDK is the only hard requirement; the quality adapter prefers the wrapper.
		tools:    []Tool{{"java", "https://adoptium.net"}},
		skipDirs: []string{"build"},
		// Maven reactors and Gradle multi-projects are one build rooted at the top pom /
		// settings file; submodule manifests must not become separate units (building a
		// submodule alone can't resolve its siblings and hangs).
		singleRoot: true,
	})
}
