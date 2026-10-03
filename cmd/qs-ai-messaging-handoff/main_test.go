package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	maintenance "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/aimessaginghandoff"
)

func TestMQHandoffCLIRefusesAmbiguousOrTruncatedReviewDocuments(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown_field":               `{"manifest_revision":1,"approved":true}`,
		"multiple_documents":          `{"manifest_revision":1} {"database":"other"}`,
		"oversized_with_valid_prefix": `{"manifest_revision":1}` + strings.Repeat(" ", 1<<20),
		"truncated":                   `{"manifest_revision":1`,
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			var m maintenance.Manifest
			if decode(file, &m) == nil {
				t.Fatal("ambiguous or oversized review accepted")
			}
		})
	}
}

func TestMQHandoffCLIReadsExplicitIdentityListWithoutMutation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "ids.json")
	raw := []byte(`["00000000-0000-4000-8000-000000000001"]`)
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var ids []string
	if err := decode(file, &ids); err != nil || len(ids) != 1 {
		t.Fatal("explicit list unreadable", err)
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(raw) {
		t.Fatal("review input changed")
	}
}
