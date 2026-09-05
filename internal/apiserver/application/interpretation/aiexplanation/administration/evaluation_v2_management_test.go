package administration

import (
	"context"
	"testing"
	"time"

	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/stretchr/testify/require"
)

type managementV2Stub struct {
	orgID   int64
	calls   int
	actor   string
	reason  string
	version int64
}

func (s *managementV2Stub) ListEvidenceV2(_ context.Context, orgID int64, _ *domainevaluation.EvidenceStatus, _ string, _ int) ([]appevaluation.EvidenceV2Summary, string, error) {
	s.orgID = orgID
	return []appevaluation.EvidenceV2Summary{}, "", nil
}
func (s *managementV2Stub) Cancel(_ context.Context, _ meta.ID, version int64, actor, reason string, _ bool, _ time.Time) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.calls++
	s.actor = actor
	s.reason = reason
	s.version = version
	return reviewableEvaluationV2(), nil
}
func TestEvaluationV2ManagementUsesTrustedScopeAndRejectsInvalidWrites(t *testing.T) {
	management := &managementV2Stub{}
	access := &accessStub{}
	svc := NewService(&reviewWorkflowStub{}, &governanceStub{}, access, WithEvaluationV2(nil, &evaluationV2EvidenceStub{value: reviewableEvaluationV2()}, nil, nil), WithEvaluationV2Management(management, management))
	actor := Actor{OrgID: 7, OperatorUserID: 42}
	_, err := svc.ListEvaluationsV2(context.Background(), actor, EvaluationV2ListQuery{})
	require.NoError(t, err)
	require.Equal(t, int64(7), management.orgID)
	require.Equal(t, 1, access.readCalls)
	invalid := domainevaluation.EvidenceStatus("garbage")
	_, err = svc.ListEvaluationsV2(context.Background(), actor, EvaluationV2ListQuery{Status: &invalid})
	require.Error(t, err)
	for _, command := range []CancelEvaluationV2Command{{ExpectedVersion: 1, Reason: " "}, {ExpectedVersion: 0, Reason: "stop"}} {
		_, err = svc.CancelEvaluationV2(context.Background(), actor, meta.ID(701), command)
		require.Error(t, err)
	}
	_, err = svc.CancelEvaluationV2(context.Background(), Actor{OrgID: 99, OperatorUserID: 42}, meta.ID(701), CancelEvaluationV2Command{ExpectedVersion: 1, Reason: "stop"})
	require.Error(t, err)
	require.Zero(t, management.calls)
	_, err = svc.CancelEvaluationV2(context.Background(), actor, meta.ID(701), CancelEvaluationV2Command{ExpectedVersion: 9, Reason: " stop ", Discard: true})
	require.NoError(t, err)
	require.Equal(t, 1, management.calls)
	require.Equal(t, "user:42", management.actor)
	require.Equal(t, "stop", management.reason)
	require.Equal(t, int64(9), management.version)
}
