package assessmentmodel

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
