package interpretation

import (
	"reflect"
	"testing"
	"time"

	evaluationreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"go.mongodb.org/mongo-driver/bson"
)

func TestCurrentReportReadQueryUsesExistingFilterSemantics(t *testing.T) {
	testeeID := uint64(8)
	risk := "medium"
	got := buildInterpretReportReadModelQuery(evaluationreadmodel.ReportFilter{
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
		t.Fatalf("current report query = %#v, want %#v", got, want)
	}
}

func TestMergeCurrentAndArchivedReportRowsPrefersCurrentReport(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	reports := []evaluationreadmodel.ReportRow{
		{AssessmentID: 1, Conclusion: "new", CreatedAt: now},
		{AssessmentID: 3, Conclusion: "newer variant", CreatedAt: now.Add(time.Minute)},
		{AssessmentID: 3, Conclusion: "older variant", CreatedAt: now},
	}
	archives := []evaluationreadmodel.ReportRow{
		{AssessmentID: 1, Conclusion: "archived duplicate", CreatedAt: now.Add(2 * time.Minute)},
		{AssessmentID: 2, Conclusion: "archived only", CreatedAt: now.Add(30 * time.Second)},
	}
	merged := mergeCurrentAndArchivedReportRows(reports, archives)
	if len(merged) != 3 {
		t.Fatalf("merged rows = %#v", merged)
	}
	if merged[0].AssessmentID != 3 || merged[0].Conclusion != "newer variant" || merged[1].AssessmentID != 2 || merged[2].AssessmentID != 1 || merged[2].Conclusion != "new" {
		t.Fatalf("new-first merged rows = %#v", merged)
	}
}
