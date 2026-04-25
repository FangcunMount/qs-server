package rest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectionTransportDoesNotImportApiserverInterface(t *testing.T) {
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
		if strings.Contains(string(data), "internal/apiserver/interface") ||
			strings.Contains(string(data), "internal/apiserver/transport") {
			t.Fatalf("collection transport must not import apiserver interface/transport: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
