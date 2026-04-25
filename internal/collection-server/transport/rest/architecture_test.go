package rest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectionTransportDoesNotImportForeignOrLegacyInterface(t *testing.T) {
	t.Parallel()

	root := filepath.Clean("..")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
		text := string(data)
		if strings.Contains(text, "internal/apiserver/interface") ||
			strings.Contains(text, "internal/apiserver/transport") {
			t.Fatalf("collection transport must not import apiserver interface/transport: %s", path)
		}
		if strings.Contains(text, "internal/collection-server/interface/restful") {
			t.Fatalf("collection transport must own REST middleware/handlers, not import legacy interface/restful: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
