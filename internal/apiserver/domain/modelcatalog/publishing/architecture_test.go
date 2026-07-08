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

func TestPublishingDoesNotExportPersonalityPayloadBuilders(t *testing.T) {
	t.Parallel()

	root := "."
	forbiddenPrefixes := []string{"PersonalityPayload", "BehavioralRatingPayload", "CognitivePayload"}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
		text := string(data)
		for _, prefix := range forbiddenPrefixes {
			if strings.Contains(text, "func "+prefix) {
				t.Fatalf("%s exports forbidden model-family builder %s; use typology/norming/taskperformance names", path, prefix)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
