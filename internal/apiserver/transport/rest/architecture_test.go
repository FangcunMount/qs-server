package rest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRESTTransportDoesNotDependOnLegacyRESTImplementation(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "internal/apiserver/interface/restful") {
			t.Fatalf("transport/rest must own REST implementation and not import legacy interface/restful: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
