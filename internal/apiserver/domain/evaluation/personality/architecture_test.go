package personality_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExecutePackageDoesNotReferenceConcreteTypologyScorers(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	executeRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "execute")
	forbidden := []string{
		"ScoreMBTI",
		"ScoreSBTI",
		"buildMBTIOutcome",
		"buildSBTIOutcome",
		"mbtiAlgorithmRunner",
		"sbtiAlgorithmRunner",
		"domain/evaluation/personality/typology",
	}
	walkGoFiles(t, executeRoot, func(rel, text string) {
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; execute must stay algorithm-agnostic", rel, token)
			}
		}
	})
}

func TestTypologyExecutorStaysAlgorithmAgnostic(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "personality", "typology", "executor.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"ScoreMBTI",
		"ScoreSBTI",
		"AlgorithmMBTI",
		"AlgorithmSBTI",
		"AlgorithmBigFive",
		"buildMBTIOutcome",
		"buildSBTIOutcome",
		"MBTI",
		"SBTI",
		"BigFive",
	} {
		if strings.Contains(text, token) {
			t.Fatalf("executor.go contains %q; concrete model logic belongs in adapters", token)
		}
	}
}

func TestTypologyApplicationLayerKeepsConcreteModelsInAdapters(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	typologyRoot := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "personality", "typology")
	allowed := map[string]struct{}{
		"algorithm_runner.go":    {},
		"module.go":              {},
		"module_registry.go":     {},
		"modules.go":             {},
		"report_builder.go":      {},
		"report_mbti.go":         {},
		"report_sbti.go":         {},
		"report_bigfive.go":      {},
		"converters.go":          {},
		"report_input_mapper.go": {},
		"outcome_assembler.go":   {},
		"model_ref.go":           {},
		"executor.go":            {},
	}
	forbidden := []string{
		"ScoreMBTI",
		"ScoreSBTI",
	}
	err := filepath.WalkDir(typologyRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		base := filepath.Base(path)
		if base == "architecture_test.go" {
			return nil
		}
		if _, ok := allowed[base]; ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		rel := filepath.ToSlash(mustRel(t, root, path))
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; keep concrete scorers in adapter files only", rel, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAlgorithmRunnerStaysModuleRegistryDriven(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "personality", "typology", "algorithm_runner.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"reportBuilders",
		"DefaultRegistry()",
		"personalityadapter.DefaultRegistry",
		"buildMBTIReport",
		"buildSBTIReport",
		"AlgorithmMBTI",
		"AlgorithmSBTI",
	} {
		if strings.Contains(text, token) {
			t.Fatalf("algorithm_runner.go contains %q; resolve modules through ModuleRegistry", token)
		}
	}
}

func TestReportBuilderStaysAlgorithmAgnostic(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	path := filepath.Join(root, "internal", "apiserver", "application", "evaluation", "personality", "typology", "report_builder.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"buildMBTIReport",
		"buildSBTIReport",
		"BuildMBTIReport",
		"BuildSBTIReport",
		"MBTIReportInput",
		"SBTIReportInput",
		"AlgorithmMBTI",
		"AlgorithmSBTI",
	} {
		if strings.Contains(text, token) {
			t.Fatalf("report_builder.go contains %q; keep model-specific report wiring in report_mbti/report_sbti", token)
		}
	}
}

func TestProfileCoreDoesNotDependOnLegacyTypologyPayload(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	profileRoot := filepath.Join(root, "internal", "apiserver", "domain", "evaluation", "personality", "profile")
	forbidden := []string{
		"assessmentmodel/personality/typology",
		"MBTILegacyModel",
		"SBTILegacyModel",
	}
	walkGoFiles(t, profileRoot, func(rel, text string) {
		if strings.HasSuffix(rel, "_test.go") {
			return
		}
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; profile core must stay payload-agnostic", rel, token)
			}
		}
	})
}

func walkGoFiles(t *testing.T, root string, check func(rel, text string)) {
	t.Helper()
	repo := repoRoot(t)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(path) == "architecture_test.go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		check(filepath.ToSlash(mustRel(t, repo, path)), string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}
