package operator

import (
	"context"
	"errors"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	outcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	assessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssessmentAuthorizerPreservesRequestedAction(t *testing.T) {
	for _, action := range []string{"read", "read_progress", "retry", "force_retry", "batch_evaluate"} {
		t.Run(action, func(t *testing.T) {
			checker := &accessCheckerStub{}
			auth := authorizer{assessments: &assessmentRepoStub{items: map[uint64]*assessment.Assessment{1: newAssessment(t, 1, 1)}}, access: checker}
			_, err := auth.loadAssessment(context.Background(), Actor{OrgID: 1, OperatorUserID: 9}, 1, action)
			require.NoError(t, err)
			require.Equal(t, []string{action}, checker.actions)
			require.Len(t, checker.calls, 1)
		})
	}
}

type scopedTrendReader struct {
	outcome.ScoreFactReader
	calls int
}

func (r *scopedTrendReader) Trend(_ context.Context, id uint64, factor string, _ int) (*outcome.FactorTrendFact, error) {
	r.calls++
	return &outcome.FactorTrendFact{TesteeID: id, FactorCode: factor}, nil
}
func TestTrendChecksStoreScopeBeforeScoreReader(t *testing.T) {
	checker := &accessCheckerStub{denied: map[uint64]error{101: errors.New("outside read scope")}}
	scores := &scopedTrendReader{}
	s := &queryService{access: checker, scores: scores}
	ctx := authztest.WithPermission(context.Background(), "qs:evaluation:collection:assessments", "read")
	_, err := s.GetFactorTrend(ctx, Actor{OrgID: 1, OperatorUserID: 9}, TrendQuery{TesteeID: 101, FactorCode: "sleep"})
	require.Error(t, err)
	require.Zero(t, scores.calls)
	require.Equal(t, []string{"read"}, checker.actions)
	delete(checker.denied, 101)
	_, err = s.GetFactorTrend(ctx, Actor{OrgID: 1, OperatorUserID: 9}, TrendQuery{TesteeID: 101, FactorCode: "sleep"})
	require.NoError(t, err)
	require.Equal(t, 1, scores.calls)
}
