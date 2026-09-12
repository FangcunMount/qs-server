package operator

import (
	"context"
	"encoding/json"
	"fmt"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	assessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	readmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	runport "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"testing"
	"time"

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
	require.ElementsMatch(t, []string{"id", "testee_id", "questionnaire_code", "questionnaire_version", "origin_type", "status", "manual_retry_available"}, keys(fields))
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

type progressRuns struct {
	runport.Repository
	latest *evalrun.EvaluationRun
	err    error
	calls  int
}

func (r *progressRuns) FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error) {
	r.calls++
	return r.latest, r.err
}
func TestProgressManualRetryEligibility(t *testing.T) {
	for _, tc := range []struct {
		name            string
		attempt         int
		retryable, want bool
	}{
		{"automatic", 1, true, false}, {"manual", 3, true, true}, {"terminal", 3, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record, createErr := assessment.NewAssessment(1, testee.NewID(101), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), assessment.NewAnswerSheetRef(meta.FromUint64(201)), assessment.NewAdhocOrigin(), assessment.WithID(assessment.NewID(1)), assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("S"), "1", "test")))
			require.NoError(t, createErr)
			require.NoError(t, record.Submit())
			require.NoError(t, record.MarkAsFailed("private failure"))
			run := evalrun.NewEvaluationRunWithAttempt(1, tc.attempt)
			require.NoError(t, run.Start(time.Now()))
			require.NoError(t, run.Fail(time.Now(), evalrun.Failure{Kind: evalrun.FailureKindTimeout, Message: "private diagnostic", Retryable: tc.retryable}))
			runs := &progressRuns{latest: &run}
			s := &queryService{assessments: &assessmentRepoStub{items: map[uint64]*assessment.Assessment{1: record}}, access: &accessCheckerStub{}, runs: runs}
			ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "read_progress", Mode: appauthz.AuthorizationModeUnconditional}}})
			result, err := s.GetProgress(ctx, Actor{OrgID: 1, OperatorUserID: 9}, 1)
			require.NoError(t, err)
			raw, err := json.Marshal(result)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(raw, &fields))
			require.Equal(t, tc.want, fields["manual_retry_available"])
			require.NotContains(t, string(raw), "private")
			runs.latest = nil
			result, err = s.GetProgress(ctx, Actor{OrgID: 1, OperatorUserID: 9}, 1)
			require.NoError(t, err)
			raw, _ = json.Marshal(result)
			require.Contains(t, string(raw), `"manual_retry_available":false`)
			runs.err = fmt.Errorf("storage failed")
			_, err = s.GetProgress(ctx, Actor{OrgID: 1, OperatorUserID: 9}, 1)
			require.Error(t, err)
			calls := runs.calls
			_, err = s.GetProgress(ctx, Actor{OrgID: 2, OperatorUserID: 9}, 1)
			require.Error(t, err)
			require.Equal(t, calls, runs.calls, "range rejection must precede run reads")
		})
	}
}

type progressReader struct {
	readmodel.AssessmentReader
	filter readmodel.AssessmentFilter
	calls  int
}

func (r *progressReader) ListAssessments(_ context.Context, f readmodel.AssessmentFilter, _ readmodel.PageRequest) ([]readmodel.AssessmentRow, int64, error) {
	r.calls++
	r.filter = f
	return []readmodel.AssessmentRow{{ID: 1, TesteeID: 101, Status: "failed"}, {ID: 2, TesteeID: 101, Status: "evaluated"}}, 2, nil
}
func TestProgressListAddsEligibilityAfterScopeFiltering(t *testing.T) {
	run := evalrun.NewEvaluationRunWithAttempt(1, 3)
	require.NoError(t, run.Start(time.Now()))
	require.NoError(t, run.Fail(time.Now(), evalrun.Failure{Kind: evalrun.FailureKindTimeout, Retryable: true}))
	runs := &progressRuns{latest: &run}
	reader := &progressReader{}
	service := &queryService{reader: reader, access: &accessCheckerStub{}, runs: runs}
	ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{AuthzVersion: 1, ScopeContractVersion: 1, Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "list_progress", Mode: appauthz.AuthorizationModeUnconditional, Scopes: []appauthz.DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}}}}})
	id := uint64(101)
	page, err := service.ListProgress(ctx, Actor{OrgID: 1, OperatorUserID: 9}, ListQuery{TesteeID: &id})
	require.NoError(t, err)
	require.EqualValues(t, 1, reader.filter.OrgID)
	require.Equal(t, &id, reader.filter.TesteeID)
	require.True(t, reader.filter.RestrictToStoreScope)
	require.Equal(t, []uint64{7}, reader.filter.AllowedStoreIDs)
	require.Equal(t, 2, page.Total)
	require.True(t, page.Items[0].ManualRetryAvailable)
	require.False(t, page.Items[1].ManualRetryAvailable)
	require.Equal(t, 1, runs.calls, "completed rows must not load run diagnostics")
	service.access = &accessCheckerStub{denied: map[uint64]error{101: fmt.Errorf("unrelated")}}
	_, err = service.ListProgress(ctx, Actor{OrgID: 1, OperatorUserID: 9}, ListQuery{TesteeID: &id})
	require.Error(t, err)
	require.Equal(t, 1, runs.calls)
}

func TestAssessmentListsKeepActionRangesSeparate(t *testing.T) {
	reader := &progressReader{}
	service := &queryService{reader: reader, access: &accessCheckerStub{}}
	snapshot := &appauthz.Snapshot{AuthzVersion: 1, ScopeContractVersion: 1, Permissions: []appauthz.Permission{
		{Resource: appauthz.AssessmentResource, Action: "list_progress", Mode: appauthz.AuthorizationModeUnconditional, Scopes: []appauthz.DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{7}}}},
		{Resource: appauthz.AssessmentResource, Action: "list", Mode: appauthz.AuthorizationModeUnconditional, Scopes: []appauthz.DataScope{{OrgID: 1, Kind: "stores", StoreIDs: []uint64{8}}}},
	}}
	ctx := appauthz.WithSnapshot(context.Background(), snapshot)
	q := ListQuery{AccessibleTesteeIDs: []uint64{101}, RestrictToAccessScope: true}
	for _, tc := range []struct {
		action string
		store  uint64
	}{{"list_progress", 7}, {"list", 8}} {
		_, _, _, _, err := service.listRows(ctx, Actor{OrgID: 1, OperatorUserID: 9}, q, tc.action)
		require.NoError(t, err)
		require.Equal(t, []uint64{tc.store}, reader.filter.AllowedStoreIDs)
		require.True(t, reader.filter.RestrictToAccessScope)
		require.Equal(t, []uint64{101}, reader.filter.AccessibleTesteeIDs)
	}
	snapshot.Permissions[1].Scopes = nil
	_, _, _, _, err := service.listRows(ctx, Actor{OrgID: 1, OperatorUserID: 9}, q, "list")
	require.Error(t, err)
	require.Equal(t, 2, reader.calls, "missing list range must not borrow progress range")
}
