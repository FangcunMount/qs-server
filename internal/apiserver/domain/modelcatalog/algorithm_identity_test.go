package modelcatalog_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestAlgorithmConstantsStayFrozen(t *testing.T) {
	t.Parallel()

	allowed := map[string]string{
		"AlgorithmScaleDefault":            "scale_default",
		"AlgorithmPersonalityTypology":     "personality_typology",
		"AlgorithmBigFive":                 "bigfive",
		"AlgorithmMBTI":                    "mbti",
		"AlgorithmSBTI":                    "sbti",
		"AlgorithmBrief2":                  "brief2",
		"AlgorithmSPM":                     "spm",
		"AlgorithmBehavioralRatingDefault": "behavioral_rating_default",
	}

	text := readRepoFile(t, "internal/apiserver/domain/modelcatalog/types.go")
	matches := regexp.MustCompile(`Algorithm(\w+)\s+Algorithm\s*=\s*"([^"]+)"`).FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		t.Fatal("no Algorithm constants found in types.go")
	}

	got := make(map[string]string, len(matches))
	for _, match := range matches {
		got["Algorithm"+match[1]] = match[2]
	}
	if len(got) != len(allowed) {
		t.Fatalf("algorithm constants = %#v, want frozen set %#v", got, allowed)
	}
	for name, wantValue := range allowed {
		gotValue, ok := got[name]
		if !ok {
			t.Fatalf("missing algorithm constant %s; new questionnaire versions must use model code + runtime, not new Algorithm constants", name)
		}
		if gotValue != wantValue {
			t.Fatalf("%s = %q, want %q", name, gotValue, wantValue)
		}
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func TestApiserverDoesNotIntroduceTypologyAlgorithmVariants(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"AlgorithmMBTI9",
		"AlgorithmMBTI_",
		"AlgorithmSBTI9",
		"AlgorithmSBTI_",
		"AlgorithmBigFive_",
		"EvaluatorKeyMBTI9",
		"EvaluatorKeyMBTI_",
		"EvaluatorKeySBTI9",
		"EvaluatorKeySBTI_",
	}
	root := repoRoot(t)
	apiserverRoot := filepath.Join(root, "internal", "apiserver")
	err := filepath.WalkDir(apiserverRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasSuffix(path, "algorithm_identity_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		rel, _ := filepath.Rel(root, path)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; new questionnaire versions must use Payload.Runtime and model code instead of new algorithm identifiers", rel, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
