package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestValidateConfig(t *testing.T) {
	valid := config{
		mongoURI: "mongodb://localhost", mongoDB: "qs", source: "artifact",
		batchSize: 1000, workers: 8, progressInterval: time.Second,
	}
	if err := validateConfig(valid); err != nil {
		t.Fatalf("artifact backfill should not require MySQL: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*config)
	}{
		{"archive requires MySQL", func(c *config) { c.source = "archive" }},
		{"batch size has an upper bound", func(c *config) { c.batchSize = 10001 }},
		{"workers has an upper bound", func(c *config) { c.workers = 65 }},
		{"range must advance", func(c *config) { c.afterID, c.toID = 10, 10 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid
			tt.mutate(&candidate)
			if err := validateConfig(candidate); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRangeFilter(t *testing.T) {
	filter := rangeFilter(100, 200)
	rangeQuery, ok := filter["domain_id"].(bson.M)
	if !ok {
		t.Fatalf("domain_id filter type = %T", filter["domain_id"])
	}
	if got := asUint64(rangeQuery["$gt"]); got != 100 {
		t.Fatalf("$gt = %d, want 100", got)
	}
	if got := asUint64(rangeQuery["$lte"]); got != 200 {
		t.Fatalf("$lte = %d, want 200", got)
	}
}

func TestSourceRangeFilterExcludesSoftDeletedDocuments(t *testing.T) {
	filter := sourceRangeFilter(archivePhase(), 100, 200)
	if value, ok := filter["deleted_at"]; !ok || value != nil {
		t.Fatalf("deleted_at filter = %#v, want explicit nil", value)
	}
}

func TestLatestArtifactsByAssessment(t *testing.T) {
	t0 := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	docs := []bson.M{
		{"domain_id": int64(10), "assessment_id": int64(1), "generated_at": t0},
		{"domain_id": int64(20), "assessment_id": int64(2), "generated_at": t0},
		{"domain_id": int64(11), "assessment_id": int64(1), "generated_at": t0.Add(time.Minute)},
		{"domain_id": int64(21), "assessment_id": int64(2), "generated_at": t0},
	}

	latest := latestArtifactsByAssessment(docs)
	if len(latest) != 2 {
		t.Fatalf("len = %d, want 2", len(latest))
	}
	if got := asUint64(latest[0]["domain_id"]); got != 11 {
		t.Fatalf("assessment 1 report = %d, want 11", got)
	}
	if got := asUint64(latest[1]["domain_id"]); got != 21 {
		t.Fatalf("assessment 2 report = %d, want 21 tie-break winner", got)
	}
}

func TestArchiveCatalogEntryUsesAssessmentAssociation(t *testing.T) {
	createdAt := time.Date(2021, 12, 30, 16, 5, 30, 0, time.UTC)
	doc := bson.M{
		"domain_id":  int64(614332407667044910),
		"testee_id":  int64(999), // Legacy value must not be trusted.
		"scale_code": "SNAP-IV",
		"risk_level": "medium",
		"created_at": createdAt,
	}
	association := assessmentAssociation{TesteeID: 613512251248226862, OrgID: 42}

	entry := archiveCatalogEntry(doc, association)
	if got := asUint64(entry["assessment_id"]); got != 614332407667044910 {
		t.Fatalf("assessment_id = %d", got)
	}
	if got := asUint64(entry["testee_id"]); got != association.TesteeID {
		t.Fatalf("testee_id = %d, want assessment association %d", got, association.TesteeID)
	}
	if got := asInt64(entry["org_id"]); got != association.OrgID {
		t.Fatalf("org_id = %d, want %d", got, association.OrgID)
	}
}

func TestAssessmentAssociationQueryUsesAssessmentIdentity(t *testing.T) {
	want := "SELECT id, testee_id, org_id FROM assessment WHERE id IN (?,?,?)"
	if got := assessmentAssociationQuery(3); got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestArchiveCatalogFilterCannotMatchArtifact(t *testing.T) {
	want := bson.M{
		"assessment_id": uint64(7),
		"$or": bson.A{
			bson.M{"source_kind": "archive"},
			bson.M{"source_kind": bson.M{"$exists": false}},
		},
	}
	if got := archiveCatalogFilter(7); !reflect.DeepEqual(got, want) {
		t.Fatalf("filter = %#v, want %#v", got, want)
	}
}

func TestFormatProgressLine(t *testing.T) {
	line := formatProgressLine("archive", 100, summary{scanned: 25, inserted: 20, missingAssessment: 2}, 123, 5*time.Second)
	for _, want := range []string{"archive", "25.00%", "25/100", "rate=5/s", "checkpoint=123", "ins=20", "miss=2"} {
		if !strings.Contains(line, want) {
			t.Fatalf("progress line %q does not contain %q", line, want)
		}
	}
}

func TestApplyBulkResult(t *testing.T) {
	delta := summary{}
	applyBulkResult(&delta, 10, nil, 2)
	if delta.conflict != 2 || delta.unchanged != 8 {
		t.Fatalf("conflict=%d unchanged=%d, want 2 and 8", delta.conflict, delta.unchanged)
	}
}
