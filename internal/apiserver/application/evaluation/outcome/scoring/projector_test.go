package scoring_test

import (
	"context"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type evaluationScoreRepoStub struct {
	assessment *assessment.Assessment
	outcomeID  meta.ID
	score      *assessment.ScaleScoreProjection
}

func (r *evaluationScoreRepoStub) SaveProjectionFromOutcome(_ context.Context, outcomeID meta.ID, a *assessment.Assessment, score *assessment.ScaleScoreProjection) error {
	r.outcomeID = outcomeID
	r.assessment = a
	r.score = score
	return nil
}
func (*evaluationScoreRepoStub) DeleteByAssessmentID(context.Context, assessment.ID) error {
	return nil
}

func TestAssessmentScoreProjectorPersistsOutcomeDerivedProjectionInEvaluation(t *testing.T) {
	t.Parallel()

	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(2)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(3)),
		assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("S-1"), "1.0.0", "scale")),
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome := assessment.NewAssessmentOutcome(*a.EvaluationModelRef(), assessment.ResultSummary{}, assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale})
	outcome.Dimensions = []assessment.DimensionResult{{
		Code:  "total",
		Name:  "总分",
		Kind:  assessment.DimensionKindFactor,
		Score: &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 18},
		Level: &assessment.OutcomeResultLevel{Code: "high"},
	}}
	repo := &evaluationScoreRepoStub{}
	projector := outcomescoring.NewAssessmentScoreProjector(repo)

	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           meta.FromUint64(4),
		OrgID:        1,
		AssessmentID: a.ID(),
		TesteeID:     a.TesteeID().Uint64(),
		RunID:        "3:1",
		Model:        domainoutcome.ModelIdentity{Kind: "scale", Code: "S-1"},
		Payload:      []byte(`{}`),
		EvaluatedAt:  time.Unix(100, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projector.Project(context.Background(), record, evaloutcome.Outcome{Assessment: a, Execution: evaloutcome.ExecutionFromAssessmentOutcome(outcome)}); err != nil {
		t.Fatal(err)
	}
	if repo.assessment != a || repo.outcomeID != record.ID() || repo.score == nil || len(repo.score.FactorScores()) != 1 {
		t.Fatalf("score projection = %#v assessment=%p outcome=%s", repo.score, repo.assessment, repo.outcomeID)
	}
}
