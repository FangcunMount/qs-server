package outcome

import (
	"context"
	"testing"
	"time"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScoreFactUsesFrozenOutcomeMaxScoreAndName(t *testing.T) {
	max := 20.0
	execution := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "F1", Name: "Frozen factor", Score: &domainoutcome.ScoreValue{Value: 8, Max: &max},
	}}}
	names, maxScores, _ := scoreMetadataFromRecord(nil, execution)
	projection := domainassessment.NewScaleScoreProjection(
		meta.FromUint64(1), 8, domainassessment.RiskLevelNone,
		[]domainassessment.ScaleFactorScore{domainassessment.NewScaleFactorScore(domainassessment.NewFactorCode("F1"), "projection name", 8, domainassessment.RiskLevelNone, false)},
	)
	fact := scoreFactFromProjection(projection, names, maxScores)
	if len(fact.FactorScores) != 1 || fact.FactorScores[0].FactorName != "Frozen factor" || fact.FactorScores[0].MaxScore == nil || *fact.FactorScores[0].MaxScore != 20 {
		t.Fatalf("score fact = %#v", fact)
	}
}

type scoreOutcomeRepoStub struct {
	record *domainoutcome.Record
}

func (s *scoreOutcomeRepoStub) Save(context.Context, *domainoutcome.Record) error { return nil }
func (s *scoreOutcomeRepoStub) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return s.record, nil
}
func (s *scoreOutcomeRepoStub) FindByAssessmentID(context.Context, meta.ID) (*domainoutcome.Record, error) {
	return s.record, nil
}

func TestScoreFactReaderRejectsMissingFrozenReportInput(t *testing.T) {
	t.Parallel()

	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID: meta.FromUint64(10), AssessmentID: meta.FromUint64(8), TesteeID: 9, RunID: "8:1",
		Model: domainoutcome.ModelIdentity{
			Kind: modelcatalog.KindScale, Code: "SDS", Version: "1.0.0", Title: "SDS",
		},
		Runtime:       domainoutcome.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindScoreRange},
		SchemaVersion: domainoutcome.CurrentSchemaVersion,
		EvaluatedAt:   time.Unix(100, 0),
		Payload:       []byte(`{"Dimensions":[{"Code":"total","Name":"总分","Role":"total","Score":{"Value":42}}]}`),
		// Empty ReportInput is invalid after the DefinitionV2-only cutover.
	})
	if err != nil {
		t.Fatal(err)
	}

	reader := NewScoreFactReader(&scoreOutcomeRepoStub{record: record}, nil)
	if _, err := reader.Get(context.Background(), 8); err == nil {
		t.Fatal("outcome without schema 3 report input was accepted")
	}
}
