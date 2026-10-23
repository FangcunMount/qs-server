package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/migration"
)

func TestInitialBindingCLIRequiresSeparateBoundedReviewedAction(t *testing.T) {
	base := []string{"--mysql-database", "owned", "--mongo-database", "owned", "--timeout", "30s"}
	valid := append(append([]string{}, base...), "--binding-action", "preview", "--binding-plan", "private.json", "--binding-operation-id", "op", "--binding-owner", "qs.mysql", "--binding-store-id", "original", "--binding-writer-epoch", "m3-1")
	if _, err := parseArguments(valid); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{append(append([]string{}, valid...), "--apply"), append(append([]string{}, valid...), "--binding-reviewed-sha256", strings.Repeat("a", 64)), append(append([]string{}, base...), "--binding-plan", "private.json"), append(append([]string{}, base...), "--binding-action", "apply", "--binding-plan", "private.json", "--binding-reviewed-sha256", strings.Repeat("a", 64)), append(append([]string{}, valid...), "--timeout", "15m")} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("ambiguous binding/migration flags accepted")
		}
	}
}
func TestInitialBindingPrivateRawPlanRejectsOverwriteAndDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	approved, err := writeInitialBindingPlan(path, migration.InitialBindingPlan{Schema: migration.InitialBindingSchema})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readInitialBindingPlan(path, approved); err != nil {
		t.Fatal(err)
	}
	if _, err = writeInitialBindingPlan(path, migration.InitialBindingPlan{}); err == nil {
		t.Fatal("private plan overwritten")
	}
	if _, err = readInitialBindingPlan(path, strings.Repeat("f", 64)); err == nil {
		t.Fatal("unreviewed bytes accepted")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = readInitialBindingPlan(path, approved); err == nil {
		t.Fatal("public plan accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(path), "link.json")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err = readInitialBindingPlan(link, approved); err == nil {
		t.Fatal("symlink plan accepted")
	}
	for _, raw := range []string{`{"schema":"one","schema":"two"}`, `{"binding":{"Owner":"one","Owner":"two"}}`, `{"schema":"one","force":true}`, strings.Repeat(" ", 16385)} {
		if err = os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(raw))
		if _, err = readInitialBindingPlan(path, hex.EncodeToString(sum[:])); err == nil {
			t.Fatal("duplicate, unknown or oversized JSON accepted")
		}
	}
}
