package grpcclient

import (
	"testing"

	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
)

func TestConvertAssessmentReportMapsDimensionNormContext(t *testing.T) {
	input := &interpretationpb.AssessmentReport{Dimensions: []*interpretationpb.DimensionInterpret{{
		FactorCode: "gec",
		DerivedScores: []*evaluationpb.ScoreValue{{Kind: "t_score", Value: 65}, {Kind: "percentile", Value: 90}},
		Level: &evaluationpb.ResultLevel{Code: "elevated", Label: "偏高", Severity: "high"},
		NormReference: &interpretationpb.NormReference{ScoreKind: "t_score", Benchmark: 50, TableVersion: "2026", MinAgeMonths: 60, MaxAgeMonths: 95},
	}}}

	got := convertAssessmentReport(input).Dimensions[0]
	if len(got.DerivedScores) != 2 || got.DerivedScores[0].Value != 65 {
		t.Fatalf("derived scores = %#v", got.DerivedScores)
	}
	if got.Level == nil || got.Level.Code != "elevated" || got.NormReference == nil || got.NormReference.Benchmark != 50 || got.NormReference.TableVersion != "2026" {
		t.Fatalf("dimension context = level %#v norm %#v", got.Level, got.NormReference)
	}
}
