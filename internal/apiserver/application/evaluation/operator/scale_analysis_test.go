package operator

import (
	"context"
	"errors"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"testing"
)

type scaleAnalysisQueryStub struct {
	QueryService
	scoreErr error
}

func (s scaleAnalysisQueryStub) ListAssessments(context.Context, Actor, ListQuery) (*AssessmentList, error) {
	kind, model, risk := "scale", "M1", "high"
	total := 99.0
	return &AssessmentList{Items: []*Assessment{{ID: 1, Status: "evaluated", ModelKind: &kind, ModelCode: &model, TotalScore: &total, RiskLevel: &risk}}}, nil
}
func (s scaleAnalysisQueryStub) GetScores(context.Context, Actor, uint64) (*Score, error) {
	return nil, s.scoreErr
}

func TestScaleAnalysisDoesNotReturnSummaryWhenScoreAuthorizationFails(t *testing.T) {
	for _, failure := range []error{evalerrors.PermissionDenied("outside current store"), errors.New("scope service unavailable")} {
		service := NewScaleAnalysisService(scaleAnalysisQueryStub{scoreErr: failure})
		result, err := service.GetScaleAnalysis(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, 3)
		if !errors.Is(err, failure) || result != nil {
			t.Fatalf("denied enrichment exposed summary: result=%+v err=%v", result, err)
		}
	}
}
func TestScaleAnalysisPreservesOptionalMissingScoreRecord(t *testing.T) {
	service := NewScaleAnalysisService(scaleAnalysisQueryStub{scoreErr: evalerrors.AssessmentScoreNotFound(errors.New("missing"), "no score yet")})
	result, err := service.GetScaleAnalysis(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, 3)
	if err != nil || len(result.Scales) != 1 || result.Scales[0].Tests[0].TotalScore != 99 {
		t.Fatalf("optional missing score: %+v %v", result, err)
	}
}
