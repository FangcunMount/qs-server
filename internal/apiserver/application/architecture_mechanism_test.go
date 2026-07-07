package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// transitionalAssessmentCodePackages lists algorithm-specific extension packages allowed
// during migration to mechanism-oriented organization. Do not add new entries.
var transitionalAssessmentCodePackages = map[string]string{
	"internal/apiserver/domain/modelcatalog/behavioral_rating/brief2":  "transitional: converging to factor_norm / calculation/norm",
	"internal/apiserver/domain/modelcatalog/cognitive/spm":             "transitional: converging to task_performance",
	"internal/apiserver/domain/evaluation/personality/adapter/mbti":    "transitional: characterization-only legacy adapter",
	"internal/apiserver/domain/evaluation/personality/adapter/sbti":    "transitional: characterization-only legacy adapter",
	"internal/apiserver/domain/evaluation/personality/adapter/bigfive": "transitional: characterization-only legacy adapter",
}

var forbiddenAssessmentCodeDirNames = []string{
	"brief2", "spm", "mbti", "sbti", "bigfive", "conners", "snap_iv", "snapiv", "phq9", "gad7",
}

func TestDomainAndApplicationDoNotAddAssessmentCodeNamedPackages(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	for _, relRoot := range []string{
		"internal/apiserver/domain",
		"internal/apiserver/application",
	} {
		scanRoot := filepath.Join(root, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if !isForbiddenAssessmentCodeDirName(name) {
				return nil
			}
			rel := filepath.ToSlash(mustRel(t, root, path))
			if reason, ok := transitionalAssessmentCodePackages[rel]; ok {
				t.Logf("allowed transitional package %s (%s)", rel, reason)
				return filepath.SkipDir
			}
			t.Fatalf("%s uses forbidden assessment code directory name %q; organize by mechanism, not assessment code", rel, name)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func isForbiddenAssessmentCodeDirName(name string) bool {
	lower := strings.ToLower(name)
	for _, forbidden := range forbiddenAssessmentCodeDirNames {
		if lower == forbidden {
			return true
		}
	}
	return false
}
