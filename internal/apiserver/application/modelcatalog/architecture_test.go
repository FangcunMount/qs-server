package modelcatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssessmentModelServiceDoesNotDependOnLegacyScalePorts(t *testing.T) {
	root := "."
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") ||
			path == "architecture_test.go" ||
			strings.HasPrefix(path, "behavior"+string(filepath.Separator)) ||
			strings.Contains(path, string(filepath.Separator)+"behavior"+string(filepath.Separator)) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		for _, forbidden := range []string{
			"ScaleLifecycleService",
			"ScaleFactorService",
			"ScaleQueryService",
			"ScaleCategoryService",
			"ScaleQRCodeQueryService",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s must not depend on legacy scale port %s; put scale adaptation behind assessmentmodel/behavior", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPersonalityCommandDoesNotWriteLegacyRuleSet(t *testing.T) {
	root := filepath.Join("personality")
	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/ruleset",
	}
	forbiddenTokens := []string{
		"mongoruleset",
		"evaluation_rule_sets",
		"UpsertPublished",
		"DualStore",
		"NewLayeredCatalog",
		"KindMBTIMigration",
		"KindSBTIMigration",
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		for _, importPath := range forbiddenImports {
			if strings.Contains(text, importPath) {
				t.Fatalf("%s must not import %s; personality command writes only v2 published snapshots", path, importPath)
			}
		}
		for _, token := range forbiddenTokens {
			if strings.Contains(text, token) {
				t.Fatalf("%s must not reference legacy ruleset token %s", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRuntimePersonalityReadsDoNotUseDraftModelRepository(t *testing.T) {
	scanRoots := []string{
		"../personalitymodel",
		"../evaluation",
		"../../collection-server/application",
		"../../infra/evaluationinput",
	}
	forbiddenTokens := []string{
		"ModelRepository",
		"DraftRepository",
		"NewDraftRepository",
	}
	for _, root := range scanRoots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if strings.Contains(path, string(filepath.Separator)+"modelcatalog"+string(filepath.Separator)+"personality"+string(filepath.Separator)) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(content)
			for _, token := range forbiddenTokens {
				if strings.Contains(text, token) {
					t.Fatalf("%s must not reference draft model repository token %s; runtime reads use published snapshots only", path, token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
