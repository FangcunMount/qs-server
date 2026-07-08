package interpretation_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInterpretationSubpackagesOwnMechanismConcerns(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	interpRoot := filepath.Join(root, "internal", "apiserver", "domain", "interpretation")
	required := []string{"report", "builder", "rule", "template", "policy"}
	for _, pkg := range required {
		if _, err := os.Stat(filepath.Join(interpRoot, pkg)); err != nil {
			t.Fatalf("missing interpretation subpackage %q", pkg)
		}
	}
}

func TestInterpretationMechanismEntryPointsExist(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	interpRoot := filepath.Join(root, "internal", "apiserver", "domain", "interpretation")
	requiredFiles := []string{
		filepath.Join(interpRoot, "scoring", "assembler.go"),
		filepath.Join(interpRoot, "typology", "patterns", "mechanism_assembler.go"),
	}
	for _, path := range requiredFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing mechanism entry %s", filepath.Base(path))
		}
	}
	data, err := os.ReadFile(filepath.Join(interpRoot, "scoring", "assembler.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "BuildFactorScoringReport") {
		t.Fatal("scoring/assembler.go must export BuildFactorScoringReport")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
