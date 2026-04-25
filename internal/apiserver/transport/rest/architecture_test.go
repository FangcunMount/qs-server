package rest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRESTTransportUsesTransportOwnedHandlerFacade(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasPrefix(filepath.ToSlash(path), "handler/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "internal/apiserver/interface/restful/handler") {
			t.Fatalf("transport/rest should import transport/rest/handler facade, not legacy interface handler: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
