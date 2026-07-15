package service

import (
	"context"
	"testing"

	participant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

type reportReaderStub struct{ row interpretationreadmodel.ReportRow }

func (s reportReaderStub) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	return &s.row, nil
}

func (s reportReaderStub) ListReports(context.Context, interpretationreadmodel.ReportFilter, interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	return nil, 0, nil
}

type participantAccessStub struct{}

func (participantAccessStub) AuthorizeParticipant(context.Context, participant.Actor) error { return nil }
func (participantAccessStub) AuthorizeOwnAssessment(context.Context, uint64, uint64) error  { return nil }

func TestToProtoParticipantReportMapsDimensionNormContext(t *testing.T) {
	reader := reportReaderStub{row: interpretationreadmodel.ReportRow{AssessmentID: 7, Dimensions: []interpretationreadmodel.ReportDimensionRow{{
		FactorCode: "gec", FactorName: "GEC", RawScore: 12,
		DerivedScores: []interpretationreadmodel.ScoreValueRow{{Kind: "t_score", Value: 65}, {Kind: "percentile", Value: 90}},
		Level: &interpretationreadmodel.ResultLevelRow{Code: "elevated", Label: "偏高", Severity: "high"},
		NormReference: &interpretationreadmodel.NormReferenceRow{ScoreKind: "t_score", Benchmark: 50, TableVersion: "2026", FormVariant: "teacher", MinAgeMonths: 60, MaxAgeMonths: 95},
	}}}}
	result, err := participant.NewService(reader, participantAccessStub{}).GetMyReport(context.Background(), participant.Actor{TesteeID: 8}, participant.GetQuery{AssessmentID: 7})
	if err != nil {
		t.Fatal(err)
	}

	got := toProtoParticipantReport(result).GetDimensions()[0]
	if len(got.GetDerivedScores()) != 2 || got.GetDerivedScores()[0].GetValue() != 65 {
		t.Fatalf("derived scores = %#v", got.GetDerivedScores())
	}
	if got.GetLevel().GetCode() != "elevated" || got.GetNormReference().GetBenchmark() != 50 || got.GetNormReference().GetTableVersion() != "2026" {
		t.Fatalf("dimension context = level %#v norm %#v", got.GetLevel(), got.GetNormReference())
	}
}
