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
			strings.HasPrefix(path, "scoring"+string(filepath.Separator)) ||
			strings.Contains(path, string(filepath.Separator)+"scoring"+string(filepath.Separator)) {
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
				t.Fatalf("%s must not depend on legacy scale port %s; put scale adaptation behind modelcatalog/scoring", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTypologyCommandDoesNotWriteLegacyRuleSet(t *testing.T) {
	root := filepath.Join("typology")
	forbiddenImports := []string{
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/ruleset",
	}
	forbiddenTokens := []string{
		"mongoruleset",
		"evaluation_rule_sets",
		"UpsertPublished",
		"NewPublishedStore",
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
				t.Fatalf("%s must not import %s; typology command writes only v2 published snapshots", path, importPath)
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

func TestRuntimeTypologyReadsDoNotUseDraftModelRepository(t *testing.T) {
	scanRoots := []string{
		"typology/consumer",
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
			if strings.Contains(path, string(filepath.Separator)+"modelcatalog"+string(filepath.Separator)+"typology"+string(filepath.Separator)) {
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

func TestFamilyServicesDoNotBypassPublicationSnapshotBuilder(t *testing.T) {
	scanRoots := []string{"typology", "norming", "taskperformance"}
	for _, root := range scanRoots {
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
			if strings.Contains(string(content), "/application/modelcatalog/publishedmodel") {
				t.Fatalf("%s must use publication.Publisher and definition.Handler instead of publishedmodel directly", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
