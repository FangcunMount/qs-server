package operator

import (
	"context"
	"encoding/json"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"testing"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
)

func TestIndependentRolesDenyResultsBeforeReadingAnyRepository(t *testing.T) {
	ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{DirectRoles: []string{"qs:assessment_operator"}, Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "read_progress", Mode: appauthz.AuthorizationModeUnconditional}, {Resource: appauthz.AssessmentResource, Action: "list_progress", Mode: appauthz.AuthorizationModeUnconditional}}})
	s := &queryService{}
	_, err := s.GetAssessment(ctx, Actor{}, 1)
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
	_, err = s.ListAssessments(ctx, Actor{}, ListQuery{})
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
	_, err = s.GetScores(ctx, Actor{}, 1)
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
	_, err = s.GetHighRiskFactors(ctx, Actor{}, 1)
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
	_, err = s.ListAssessmentRuns(ctx, Actor{}, 1, 10)
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
}
func TestProgressRequiresSpecificPermission(t *testing.T) {
	s := &queryService{}
	_, err := s.GetProgress(context.Background(), Actor{}, 1)
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
	_, err = s.ListProgress(context.Background(), Actor{}, ListQuery{})
	require.True(t, cberrors.IsCode(err, code.ErrPermissionDenied), "%v", err)
}
func TestProgressWireIsAnAllowlist(t *testing.T) {
	raw, err := json.Marshal(Progress{})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.ElementsMatch(t, []string{"id", "testee_id", "questionnaire_code", "questionnaire_version", "origin_type", "status"}, keys(fields))
	for _, forbidden := range []string{"total_score", "risk_level", "failure_reason", "answer_sheet_id", "report", "error_message", "input_snapshot_ref"} {
		require.NotContains(t, fields, forbidden)
	}
}
func keys(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
