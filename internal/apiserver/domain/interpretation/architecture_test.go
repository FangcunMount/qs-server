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
	required := []string{"generation", "run", "report", "builder", "rule", "template", "policy"}
	for _, pkg := range required {
		if _, err := os.Stat(filepath.Join(interpRoot, pkg)); err != nil {
			t.Fatalf("missing interpretation subpackage %q", pkg)
		}
	}
}

func TestInterpretationLifecyclePackagesDoNotDependOnEvaluation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	paths := []string{
		filepath.Join(root, "internal", "apiserver", "domain", "interpretation", "generation"),
		filepath.Join(root, "internal", "apiserver", "domain", "interpretation", "run"),
		filepath.Join(root, "internal", "apiserver", "domain", "interpretation", "report", "artifact.go"),
	}
	forbidden := []string{
		"application/evaluation",
		"domain/evaluation",
		"port/evaluation",
		"infra/mongo/evaluation",
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		files := []string{path}
		if info.IsDir() {
			entries, readErr := os.ReadDir(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			files = files[:0]
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
		for _, file := range files {
			data, readErr := os.ReadFile(file)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, token := range forbidden {
				if strings.Contains(string(data), token) {
					t.Fatalf("Interpretation lifecycle package depends on Evaluation in %s: %s", file, token)
				}
			}
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
	if !strings.Contains(string(data), "BuildFactorScoringDraft") {
		t.Fatal("scoring/assembler.go must export BuildFactorScoringDraft")
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

func TestInterpretationRootPackageOnlyFacadeFiles(t *testing.T) {
	t.Parallel()

	interpRoot := filepath.Join(repoRoot(t), "internal", "apiserver", "domain", "interpretation")
	allowed := map[string]struct{}{
		"doc.go":            {},
		"types.go":          {},
		"export.go":         {},
		"errors.go":         {},
		"events.go":         {},
		"events_outcome.go": {},
		"event_wire.go":     {},
		"strategy.go":       {},
	}
	entries, err := os.ReadDir(interpRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := allowed[name]; !ok {
			t.Fatalf("unexpected root file %s; interpretation root must only contain facade files", name)
		}
	}
}
