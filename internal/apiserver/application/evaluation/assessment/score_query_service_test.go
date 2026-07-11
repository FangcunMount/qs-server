package assessment

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type scoreOutcomeRepoStub struct {
	record *domainoutcome.Record
}

func (*scoreOutcomeRepoStub) Save(context.Context, *domainoutcome.Record) error { return nil }
func (r *scoreOutcomeRepoStub) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return r.record, nil
}
func (r *scoreOutcomeRepoStub) FindByAssessmentID(context.Context, meta.ID) (*domainoutcome.Record, error) {
	return r.record, nil
}

func TestScoreDetailReadsCanonicalOutcomeInsteadOfScoreProjection(t *testing.T) {
	t.Parallel()

	record := scoreOutcomeRecord(t)
	service := NewScoreQueryService(&scoreOutcomeRepoStub{record: record}, nil, nil, nil)

	result, err := service.GetByAssessmentID(context.Background(), record.AssessmentID().Uint64())
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalScore != 18 || result.RiskLevel != "severe" || len(result.FactorScores) != 2 {
		t.Fatalf("score result = %#v", result)
	}
}

func TestHighRiskFactorsReadCanonicalOutcomeWithoutScoreProjection(t *testing.T) {
	t.Parallel()

	record := scoreOutcomeRecord(t)
	service := NewScoreQueryService(&scoreOutcomeRepoStub{record: record}, nil, nil, nil)

	result, err := service.GetHighRiskFactors(context.Background(), record.AssessmentID().Uint64())
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasHighRisk || len(result.HighRiskFactors) != 2 || !result.NeedsUrgentCare {
		t.Fatalf("high risk result = %#v", result)
	}
}

func TestHighRiskFactorsRemainEmptyWhenOutcomeDoesNotExist(t *testing.T) {
	t.Parallel()

	service := NewScoreQueryService(&scoreOutcomeRepoStub{}, nil, nil, nil)
	result, err := service.GetHighRiskFactors(context.Background(), 7001)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssessmentID != 7001 || result.HasHighRisk || len(result.HighRiskFactors) != 0 {
		t.Fatalf("high risk result = %#v", result)
	}
}

func scoreOutcomeRecord(t *testing.T) *domainoutcome.Record {
	t.Helper()
	model := domainassessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("S-1"), "1.0.0", "scale")
	legacy := domainassessment.NewAssessmentOutcome(
		model,
		domainassessment.ResultSummary{PrimaryLabel: "high"},
		domainassessment.EvaluationDetail{Kind: domainassessment.EvaluationModelKindScale},
	)
	legacy.Primary = &domainassessment.OutcomeScoreValue{Kind: domainassessment.OutcomeScoreKindRawTotal, Value: 18}
	legacy.Level = &domainassessment.OutcomeResultLevel{Code: "severe"}
	legacy.Dimensions = []domainassessment.DimensionResult{
		{Code: "total", Name: "总分", Kind: domainassessment.DimensionKindFactor, Score: &domainassessment.OutcomeScoreValue{Kind: domainassessment.OutcomeScoreKindRawTotal, Value: 18}, Level: &domainassessment.OutcomeResultLevel{Code: "severe"}},
		{Code: "sleep", Name: "睡眠", Kind: domainassessment.DimensionKindFactor, Score: &domainassessment.OutcomeScoreValue{Kind: domainassessment.OutcomeScoreKindRawTotal, Value: 9}, Level: &domainassessment.OutcomeResultLevel{Code: "high"}},
	}
	execution := evaloutcome.ExecutionFromAssessmentOutcome(legacy)
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           meta.FromUint64(9001),
		OrgID:        1,
		AssessmentID: meta.FromUint64(7001),
		TesteeID:     testee.NewID(8001).Uint64(),
		RunID:        "7001:1",
		Model:        domainoutcome.ModelIdentity{Kind: "scale", Code: "S-1", Version: "1.0.0"},
		Payload:      payload,
		EvaluatedAt:  time.Unix(100, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}
