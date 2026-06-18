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
		// The reactor's version lives in the root pom; the version manager sets/checks that
		// own <version> (submodules inherit via <parent>). Gradle's location varies, so only
		// the Maven manifest is declared until a Gradle writer lands.
		versionManifests: []string{"pom.xml"},
		// Maven reactors and Gradle multi-projects are one build rooted at the top pom /
		// settings file; submodule manifests must not become separate units (building a
		// submodule alone can't resolve its siblings and hangs).
		singleRoot: true,
	})
}
