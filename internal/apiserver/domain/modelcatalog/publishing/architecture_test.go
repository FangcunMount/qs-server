package publishing_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishingDoesNotReintroduceModelFamilySnapshotFiles(t *testing.T) {
	t.Parallel()

	root := "."
	forbiddenBasenames := []string{
		"personality_payload",
		"snapshot_personality",
		"snapshot_behavioral_rating",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		base := strings.TrimSuffix(filepath.Base(path), ".go")
		for _, forbidden := range forbiddenBasenames {
			if base == forbidden {
				t.Fatalf("%s reintroduces forbidden model-family publishing file %q; use typology/norming/taskperformance names", path, forbidden+".go")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
