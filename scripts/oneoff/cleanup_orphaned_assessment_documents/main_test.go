package main

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func validConfig() config {
	return config{mongoURI: "mongodb://localhost", mongoDB: "qs", mysqlDSN: "dsn", source: "reports", backupSuffix: "test", batchSize: 1000, workers: 8}
}

func TestValidateConfigRequiresAnswerSheetCutoff(t *testing.T) {
	c := validConfig()
	c.source = "answersheets"
	if err := validateConfig(c); err == nil {
		t.Fatal("expected answersheet cutoff validation error")
	}
	c.answerSheetCreatedBefore = time.Now().Add(-24 * time.Hour)
	if err := validateConfig(c); err != nil {
		t.Fatalf("valid config: %v", err)
	}
}

func TestHardDeleteDryRunIncludesSoftDeletedDocuments(t *testing.T) {
	c := validConfig()
	c.hardDelete = true
	if err := validateConfig(c); err != nil {
		t.Fatalf("hard-delete dry-run should be valid: %v", err)
	}
	if filter := activeRangeFilter(c, 0); len(filter) != 1 {
		t.Fatalf("hard-delete filter = %#v, want domain_id only", filter)
	}
}

func TestPhasesUseAssessmentOwnershipKeys(t *testing.T) {
	if got := reportPhase().lookupSQL(2); got != "SELECT id FROM assessment WHERE id IN (?,?)" {
		t.Fatalf("report lookup = %q", got)
	}
	if got := answerSheetPhase().lookupSQL(2); got != "SELECT answer_sheet_id FROM assessment WHERE answer_sheet_id IN (?,?)" {
		t.Fatalf("answersheet lookup = %q", got)
	}
}

func TestAnswerSheetFilterIncludesSafetyCutoff(t *testing.T) {
	cutoff := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	c := validConfig()
	c.answerSheetCreatedBefore = cutoff
	want := bson.M{"domain_id": bson.M{"$gt": uint64(9)}, "deleted_at": nil, "created_at": bson.M{"$lt": cutoff}}
	if got := answerSheetPhase().filter(c, 9); !reflect.DeepEqual(got, want) {
		t.Fatalf("filter = %#v, want %#v", got, want)
	}
}

func TestReportRelatedCleanupIncludesLegacyAndCatalog(t *testing.T) {
	got := reportPhase().related
	want := []relatedCollection{{"interpret_reports", "domain_id", true}, {"report_query_catalog", "assessment_id", false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("related = %#v, want %#v", got, want)
	}
}
