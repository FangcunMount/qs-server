package interpretation

import (
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"go.mongodb.org/mongo-driver/bson"
)

func TestArtifactReadQueryUsesExistingReportFilterSemantics(t *testing.T) {
	testeeID := uint64(8)
	risk := "medium"
	got := buildArtifactReadModelQuery(evaluationreadmodel.ReportFilter{
		TesteeID:     &testeeID,
		TesteeIDs:    []uint64{8, 9},
		HighRiskOnly: true,
		ModelCode:    "SDS",
		RiskLevel:    &risk,
	})
	want := bson.M{
		"deleted_at": nil,
		"testee_id":  bson.M{"$in": []uint64{8, 9}},
		"risk_level": "medium",
		"scale_code": "SDS",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact query = %#v, want %#v", got, want)
	}
}

func TestMergeNewFirstReportRowsPrefersArtifactsAndPreservesLegacyFallback(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	artifacts := []evaluationreadmodel.ReportRow{
		{AssessmentID: 1, Conclusion: "new", CreatedAt: now},
		{AssessmentID: 3, Conclusion: "newer variant", CreatedAt: now.Add(time.Minute)},
		{AssessmentID: 3, Conclusion: "older variant", CreatedAt: now},
	}
	legacy := []evaluationreadmodel.ReportRow{
		{AssessmentID: 1, Conclusion: "legacy duplicate", CreatedAt: now.Add(2 * time.Minute)},
		{AssessmentID: 2, Conclusion: "legacy only", CreatedAt: now.Add(30 * time.Second)},
	}
	merged := mergeNewFirstReportRows(artifacts, legacy)
	if len(merged) != 3 {
		t.Fatalf("merged rows = %#v", merged)
	}
	if merged[0].AssessmentID != 3 || merged[0].Conclusion != "newer variant" || merged[1].AssessmentID != 2 || merged[2].AssessmentID != 1 || merged[2].Conclusion != "new" {
		t.Fatalf("new-first merged rows = %#v", merged)
	}
}
