package modelcatalog

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetiredFamilyApplicationPackagesContainNoGoFiles(t *testing.T) {
	t.Parallel()

	for _, directory := range []string{
		"norming",
		"taskperformance",
		"typology",
		"publishedmodel",
		"option",
		"scoring",
	} {
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".go") {
				return &forbiddenReferenceError{path: path, token: "retired application package"}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplicationDoesNotReintroduceRetiredModelCatalogPaths(t *testing.T) {
	t.Parallel()

	forbidden := retiredModelCatalogTokens()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				return &forbiddenReferenceError{path: path, token: token}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRootPackageContainsOnlySharedContracts(t *testing.T) {
	t.Parallel()

	allowed := map[string]struct{}{
		"architecture_test.go":      {},
		"authorization.go":          {},
		"authorization_test.go":     {},
		"capability_matrix_test.go": {},
		"doc.go":                    {},
		"dto.go":                    {},
		"errors.go":                 {},
		"identity.go":               {},
		"kind_mapper.go":            {},
		"kind_mapper_test.go":       {},
		"projection.go":             {},
		"usecases.go":               {},
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if _, ok := allowed[entry.Name()]; !ok {
			t.Fatalf("root application package contains concrete or unclassified file %q; place it in a focused child package", entry.Name())
		}
	}
}

func retiredModelCatalogTokens() []string {
	base := "/application/modelcatalog/"
	return []string{
		base + "norming",
		base + "taskperformance",
		base + "typology",
		base + "published" + "model",
		base + "option",
		base + "scoring",
		"Legacy" + "ScaleBinding",
		"Legacy" + "ScaleBindingSource",
		"Scale" + "QueryService",
		"Scale" + "CategoryService",
		"Prepare" + "ForSave",
		"Save" + "Input",
		"Save" + "Result",
	}
}

type forbiddenReferenceError struct {
	path  string
	token string
}

func (e *forbiddenReferenceError) Error() string {
	return e.path + " reintroduces retired ModelCatalog surface " + e.token
}
