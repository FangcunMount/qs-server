package evaluation

import (
	"reflect"
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
)

func TestReportPOToReadRowMapsReportDocumentShape(t *testing.T) {
	maxScore := 100.0
	factorCode := "sleep"
	createdAt := time.Date(2026, 5, 2, 11, 0, 0, 0, time.UTC)

	row := reportPOToReadRow(&InterpretReportPO{
		BaseDocument: base.BaseDocument{
			DomainID:  meta.FromUint64(7001),
			CreatedAt: createdAt,
		},
		ScaleName:  "SDS",
		ScaleCode:  "sds",
		TesteeID:   8001,
		TotalScore: 88,
		RiskLevel:  "high",
		Conclusion: "高风险",
		Dimensions: []DimensionInterpretPO{
			{
				FactorCode:  "total",
				FactorName:  "总分",
				RawScore:    88,
				MaxScore:    &maxScore,
				RiskLevel:   "high",
				Description: "高风险描述",
				Suggestion:  "建议干预",
			},
		},
		Suggestions: []SuggestionPO{
			{Category: "general", Content: "总体建议"},
			{Category: "dimension", Content: "睡眠建议", FactorCode: &factorCode},
		},
	})

	if row.AssessmentID != 7001 || row.ScaleName != "SDS" || row.ScaleCode != "sds" {
		t.Fatalf("unexpected report identity: %#v", row)
	}
	if row.TotalScore != 88 || row.RiskLevel != "high" || row.Conclusion != "高风险" || !row.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected report summary: %#v", row)
	}
	if len(row.Dimensions) != 1 || row.Dimensions[0].FactorCode != "total" || row.Dimensions[0].MaxScore == nil || *row.Dimensions[0].MaxScore != maxScore {
		t.Fatalf("unexpected dimensions: %#v", row.Dimensions)
	}
	if len(row.Suggestions) != 2 || row.Suggestions[1].FactorCode == nil || *row.Suggestions[1].FactorCode != factorCode {
		t.Fatalf("unexpected suggestions: %#v", row.Suggestions)
	}
}

func TestReportPOToReadRowToleratesNilLegacySlices(t *testing.T) {
	row := reportPOToReadRow(&InterpretReportPO{
		BaseDocument: base.BaseDocument{DomainID: meta.FromUint64(7001)},
	})

	if row.AssessmentID != 7001 {
		t.Fatalf("assessment id = %d, want 7001", row.AssessmentID)
	}
	if row.Dimensions == nil {
		t.Fatalf("dimensions should be an empty slice for stable response mapping")
	}
	if row.Suggestions == nil {
		t.Fatalf("suggestions should be an empty slice for stable response mapping")
	}
}

func TestBuildReportReadModelQueryDocumentsFilterContract(t *testing.T) {
	testeeID := uint64(8001)
	riskLevel := "high"

	query := buildReportReadModelQuery(evaluationreadmodel.ReportFilter{
		TesteeID:     &testeeID,
		TesteeIDs:    []uint64{8001, 8002},
		HighRiskOnly: true,
		ScaleCode:    "SDS",
		RiskLevel:    &riskLevel,
	})

	want := bson.M{
		"deleted_at": nil,
		"testee_id":  bson.M{"$in": []uint64{8001, 8002}},
		"risk_level": "high",
		"scale_code": "SDS",
	}
	if !reflect.DeepEqual(query, want) {
		t.Fatalf("query = %#v, want %#v", query, want)
	}
}
