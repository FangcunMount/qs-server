package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDuplicateRows(t *testing.T) {
	rows := []duplicateRow{{
		TesteeID: 2, ProfileID: 20, ProfileLinkID: 200, UserID: 100,
		CanonicalTestee: 1, CanonicalProfile: 10, GroupSize: 2, DuplicateOrdinal: 2,
	}}
	if err := validateDuplicateRows(rows); err != nil {
		t.Fatalf("validateDuplicateRows() error = %v", err)
	}

	rows = append(rows, duplicateRow{
		TesteeID: 3, ProfileID: 20, ProfileLinkID: 300, UserID: 100,
		CanonicalTestee: 1, CanonicalProfile: 10, GroupSize: 3, DuplicateOrdinal: 3,
	})
	if err := validateDuplicateRows(rows); err == nil || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("validateDuplicateRows() error = %v, want shared profile guard", err)
	}
}

func TestValidateDuplicateRowsRejectsMoreCompleteDuplicate(t *testing.T) {
	row := duplicateRow{
		TesteeID: 2, ProfileID: 20, ProfileLinkID: 200, UserID: 100,
		CanonicalTestee: 1, CanonicalProfile: 10, GroupSize: 2, DuplicateOrdinal: 2,
		OutcomeCount: 1,
	}
	if err := validateDuplicateRows([]duplicateRow{row}); err == nil || !strings.Contains(err.Error(), "more downstream progress") {
		t.Fatalf("validateDuplicateRows() error = %v", err)
	}
}

func TestDuplicateSelectionOrderPrioritizesTerminalProgress(t *testing.T) {
	want := []string{"outcome_count", "evaluated_count", "completed_tasks", "submitted_count", "assessment_count", "intake_count", "enrollment_count", "created_at", "testee_id"}
	last := -1
	for _, column := range want {
		index := strings.Index(duplicateSelectionOrder, column)
		if index <= last {
			t.Fatalf("selection order %q does not place %s after previous field", duplicateSelectionOrder, column)
		}
		last = index
	}
}

func TestDuplicateRowsQueryUsesCurrentSchemaAndAggregatedProgress(t *testing.T) {
	query := duplicateRowsQuery("iam")
	if strings.Contains(query, "o.deleted_at") {
		t.Fatalf("evaluation_outcome is immutable and has no deleted_at column:\n%s", query)
	}
	for _, want := range []string{
		"FROM iam.profile_links",
		"JOIN iam.profiles",
		"outcome_progress AS",
		"assessment_progress AS",
		"task_progress AS",
		"intake_progress AS",
		"enrollment_progress AS",
		"JOIN base b ON b.testee_id",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("duplicate query missing %q:\n%s", want, query)
		}
	}
	if got := strings.Count(query, "?"); got != 4 {
		t.Fatalf("duplicate query placeholders = %d, want 4", got)
	}
}

func TestScopeAnomaliesQueryLimitsIAMLinksToSourceScope(t *testing.T) {
	query := scopeAnomaliesQuery("iam")
	if !strings.Contains(query, "JOIN (SELECT DISTINCT profile_id FROM source_scope)") {
		t.Fatalf("anomaly query must constrain IAM profile links to source scope:\n%s", query)
	}
	if got := strings.Count(query, "?"); got != 4 {
		t.Fatalf("anomaly query placeholders = %d, want 4", got)
	}
}

func TestEncodeManifestEscapesName(t *testing.T) {
	data, err := encodeManifest([]duplicateRow{{
		TesteeID: 2, ProfileID: 20, ProfileLinkID: 200, UserID: 100,
		CanonicalTestee: 1, CanonicalProfile: 10, OrgID: 9,
		Name: "测试,儿童", Gender: 1, Birthday: "2014-01-02", Source: "daily_simulation",
		CreatedDate: "2026-08-12", CreatedAt: "2026-08-12 10:00:00", GroupSize: 2, DuplicateOrdinal: 2,
	}})
	if err != nil {
		t.Fatalf("encodeManifest() error = %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if got := records[1][7]; got != "测试,儿童" {
		t.Fatalf("name = %q", got)
	}
}

func TestWriteScopeFilesRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	cfg := config{
		outputDir: dir, source: "daily_simulation", clinicianID: 1,
		createdFrom: "2026-08-05 00:00:00", createdUntil: "2026-08-13 00:00:00",
	}
	rows := []duplicateRow{{
		TesteeID: 2, ProfileID: 20, ProfileLinkID: 200, UserID: 100,
		CanonicalTestee: 1, CanonicalProfile: 10, GroupSize: 2, DuplicateOrdinal: 2,
	}}
	if err := writeScopeFiles(cfg, rows); err != nil {
		t.Fatalf("writeScopeFiles() error = %v", err)
	}
	for _, name := range []string{"manifest.csv", "testee_ids.txt", "profile_ids.txt", "summary.txt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", name, info.Mode().Perm())
		}
	}
	if err := writeScopeFiles(cfg, rows); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("second write error = %v", err)
	}
}

func TestBuildSummaryIncludesPerDateCounts(t *testing.T) {
	cfg := config{source: "daily_simulation", clinicianID: 1, createdFrom: "2026-08-05 00:00:00", createdUntil: "2026-08-07 00:00:00"}
	rows := []duplicateRow{
		{TesteeID: 2, CanonicalTestee: 1, CreatedDate: "2026-08-05"},
		{TesteeID: 3, CanonicalTestee: 1, CreatedDate: "2026-08-05"},
		{TesteeID: 5, CanonicalTestee: 4, CreatedDate: "2026-08-06"},
	}
	summary := buildSummary(cfg, rows)
	for _, want := range []string{
		"duplicate_groups=2", "duplicate_testees=3", "total_testees_in_duplicate_groups=5",
		"date=2026-08-05 groups=1 duplicate_testees=2 total_testees=3",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}
