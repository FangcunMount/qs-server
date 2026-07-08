package domain_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDomainPackagesDoNotIntroduceAssessmentCodeFilenames(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	domainRoot := filepath.Join(root, "internal", "apiserver", "domain")
	allowedPathFragments := []string{
		"/characterization/",
		"/legacy/",
		"/ruleset/",
		"_test.go",
		"legacy_",
		"from_sbti",
		"from_mbti",
	}
	forbiddenFilenameTokens := []string{
		"/mbti_",
		"/sbti_",
		"/bigfive_",
		"/brief2_",
		"/spm_",
		"/phq9_",
		"score_mbti",
		"score_sbti",
	}
	err := filepath.WalkDir(domainRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel := "/" + filepath.ToSlash(mustRel(t, domainRoot, path))
		for _, frag := range allowedPathFragments {
			if strings.Contains(rel, frag) || strings.Contains(filepath.Base(path), frag) {
				return nil
			}
		}
		lower := strings.ToLower(filepath.Base(path))
		for _, token := range forbiddenFilenameTokens {
			if strings.Contains(lower, token) {
				t.Fatalf("%s leaks assessment code in filename; allowed only in legacy/fixture paths", rel)
			}
		}
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

func mustRel(t *testing.T, base, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}
