package version

import (
	"strings"
	"testing"
)

const rootPom = `<?xml version="1.0" encoding="UTF-8"?>
<project>
    <modelVersion>4.0.0</modelVersion>
    <groupId>io.example</groupId>
    <artifactId>app</artifactId>
    <version>0.3.1-SNAPSHOT</version>
    <packaging>pom</packaging>
    <modules>
        <module>core</module>
    </modules>
    <dependencies>
        <dependency>
            <groupId>com.google.code.gson</groupId>
            <artifactId>gson</artifactId>
            <version>2.10.1</version>
        </dependency>
    </dependencies>
</project>
`

const childPom = `<?xml version="1.0" encoding="UTF-8"?>
<project>
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>io.example</groupId>
        <artifactId>app</artifactId>
        <version>0.3.1-SNAPSHOT</version>
    </parent>
    <artifactId>core</artifactId>
    <dependencies>
        <dependency>
            <groupId>com.google.code.gson</groupId>
            <artifactId>gson</artifactId>
            <version>2.10.1</version>
        </dependency>
    </dependencies>
</project>
`

// externalParentPom inherits from a third-party parent (e.g. spring-boot-starter-parent),
// whose version must never be rewritten as the project's own.
const externalParentPom = `<?xml version="1.0" encoding="UTF-8"?>
<project>
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>3.2.0</version>
    </parent>
    <groupId>io.example</groupId>
    <artifactId>app</artifactId>
    <version>1.0.0</version>
    <dependencies>
        <dependency>
            <artifactId>x</artifactId>
            <version>9.9.9</version>
        </dependency>
    </dependencies>
</project>
`

// TestMavenSetVersionRootPom sets the aggregator pom's own version, preserves a -SNAPSHOT
// qualifier, and leaves dependency pins (and the modelVersion) untouched.
func TestMavenSetVersionRootPom(t *testing.T) {
	m, ok := ManagerFor("pom.xml")
	if !ok {
		t.Fatal("pom.xml manager not registered")
	}
	out, changed, err := m.SetVersion([]byte(rootPom), Version{1, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected the root version to change")
	}
	s := string(out)
	if !strings.Contains(s, "<version>1.0.0-SNAPSHOT</version>") {
		t.Errorf("own version not set / qualifier not preserved:\n%s", s)
	}
	if !strings.Contains(s, "<version>2.10.1</version>") {
		t.Errorf("dependency pin was rewritten:\n%s", s)
	}
	if !strings.Contains(s, "<modelVersion>4.0.0</modelVersion>") {
		t.Errorf("modelVersion was rewritten:\n%s", s)
	}
}

// TestMavenSetVersionAlreadyCorrect is a no-op (qualifier ignored for the equality check).
func TestMavenSetVersionAlreadyCorrect(t *testing.T) {
	m, _ := ManagerFor("pom.xml")
	_, changed, err := m.SetVersion([]byte(rootPom), Version{0, 3, 1})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("0.3.1 core already matches; should be a no-op")
	}
}

// TestMavenSetVersionChildPomSkips: a submodule that inherits its version (no own <version>,
// only a <parent> reference) states nothing this manager can set — it must error so the
// honesty engine skips it rather than rewriting the parent reference.
func TestMavenSetVersionChildPomSkips(t *testing.T) {
	m, _ := ManagerFor("pom.xml")
	if _, _, err := m.SetVersion([]byte(childPom), Version{1, 0, 0}); err == nil {
		t.Error("an inheriting child pom should error (no own version), not rewrite <parent>")
	}
}

// TestMavenSetVersionExternalParent: the project's own version is set, the third-party
// parent's version is never touched.
func TestMavenSetVersionExternalParent(t *testing.T) {
	m, _ := ManagerFor("pom.xml")
	out, changed, err := m.SetVersion([]byte(externalParentPom), Version{2, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected the own version to change")
	}
	s := string(out)
	if !strings.Contains(s, "<version>2.0.0</version>") {
		t.Errorf("own version not set:\n%s", s)
	}
	if !strings.Contains(s, "<version>3.2.0</version>") {
		t.Errorf("external parent version was rewritten:\n%s", s)
	}
	if !strings.Contains(s, "<version>9.9.9</version>") {
		t.Errorf("dependency pin was rewritten:\n%s", s)
	}
}
