package operator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"

	"github.com/FangcunMount/component-base/pkg/event"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	txapp "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	assessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	run "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type retryRunsStub struct{ run.Repository }
type eventStagerStub struct{ calls int }

func (s *eventStagerStub) Stage(context.Context, ...event.DomainEvent) error { s.calls++; return nil }
func TestRetryRequiresActionBeforeReadingOrWriting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		permissions []appauthz.Permission
		action      string
		allow       bool
	}{
		{name: "missing snapshot", action: "retry"},
		{name: "result only", permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "read", Mode: appauthz.AuthorizationModeUnconditional}}, action: "retry"},
		{name: "retired conditional", permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "retry", Mode: appauthz.AuthorizationModeObjectCheckRequired}}, action: "retry"},
		{name: "retry", permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "retry", Mode: appauthz.AuthorizationModeUnconditional}}, action: "retry", allow: true},
		{name: "retry is not force", permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "retry", Mode: appauthz.AuthorizationModeUnconditional}}, action: "force_retry"},
		{name: "admin", permissions: []appauthz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: appauthz.AuthorizationModeUnconditional}}, action: "force_retry", allow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.permissions != nil {
				ctx = appauthz.WithSnapshot(ctx, &appauthz.Snapshot{Permissions: tc.permissions})
			}
			writes := 0
			events := &eventStagerStub{}
			svc := NewGovernedRetryService(&assessmentRepoStub{items: map[uint64]*assessment.Assessment{1: newAssessment(t, 1, 1)}}, &retryRunsStub{}, txapp.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { writes++; return fn(ctx) }), events, &accessCheckerStub{})
			_, err := svc.Authorize(ctx, Actor{OrgID: 1, OperatorUserID: 9}, GovernedRetryCommand{AssessmentID: 1, AuthorizationSubject: "user:9", AuthorizationAction: tc.action, Origin: retrygovernance.AttemptOriginManual, RequestID: "test", Reason: "test"})
			if err == nil {
				t.Fatal("nonfailed assessment must be rejected")
			}
			if tc.allow && !errors.Is(err, ErrRetryStateConflict) {
				t.Fatalf("expected typed retry state conflict: %v", err)
			}
			if tc.allow && !strings.Contains(err.Error(), "failed assessment") {
				t.Fatalf("authorized action should reach state validation: %v", err)
			}
			if !tc.allow && !strings.Contains(err.Error(), "Permission denied") {
				t.Fatalf("expected action denial: %v", err)
			}
			if writes != 0 || events.calls != 0 {
				t.Fatal("denied operation mutated state")
			}
		})
	}
}

type authorizedRetryRuns struct {
	run.Repository
	calls int
}

func (s *authorizedRetryRuns) AuthorizeRetry(_ context.Context, r run.RetryAuthorizationRequest) (*evalrun.EvaluationRun, error) {
	s.calls++
	value := evalrun.NewEvaluationRunWithAttempt(r.AssessmentID, r.ExpectedAttempt)
	return &value, nil
}
func TestRetryBothOriginsPreservesBusinessRangeAndSideEffects(t *testing.T) {
	for _, origin := range []assessment.Origin{assessment.NewAdhocOrigin(), assessment.NewPlanOrigin("plan-1")} {
		for _, name := range []string{"qs:assessment_operator", "qs:evaluation_plan_manager"} {
			for _, scope := range []string{"allowed", "cross_org", "unrelated"} {
				t.Run(origin.Type().String()+"/"+name+"/"+scope, func(t *testing.T) {
					record, err := assessment.NewAssessment(1, testee.NewID(101), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"), assessment.NewAnswerSheetRef(meta.FromUint64(201)), origin, assessment.WithID(assessment.NewID(1)), assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("S"), "1", "test")))
					if err != nil {
						t.Fatal(err)
					}
					if err = record.Submit(); err != nil {
						t.Fatal(err)
					}
					if err = record.MarkAsFailed("test failure"); err != nil {
						t.Fatal(err)
					}
					access := &accessCheckerStub{}
					actor := Actor{OrgID: 1, OperatorUserID: 9}
					if scope == "cross_org" {
						actor.OrgID = 2
					}
					if scope == "unrelated" {
						access.denied = map[uint64]error{101: fmt.Errorf("testee access denied")}
					}
					runs := &authorizedRetryRuns{}
					events := &eventStagerStub{}
					transactions := 0
					svc := NewGovernedRetryService(&assessmentRepoStub{items: map[uint64]*assessment.Assessment{1: record}}, runs, txapp.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error { transactions++; return fn(ctx) }), events, access)
					ctx := appauthz.WithSnapshot(context.Background(), &appauthz.Snapshot{DirectRoles: []string{name}, Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: "retry", Mode: appauthz.AuthorizationModeUnconditional}}})
					_, err = svc.Authorize(ctx, actor, GovernedRetryCommand{AssessmentID: 1, ExpectedAttempt: 1, AuthorizationSubject: "user:9", AuthorizationAction: "retry", Origin: retrygovernance.AttemptOriginManual, RequestID: "test-request", Reason: "test"})
					if scope == "allowed" {
						if err != nil || transactions != 1 || runs.calls != 1 || events.calls != 1 {
							t.Fatalf("err=%v transactions=%d runs=%d events=%d", err, transactions, runs.calls, events.calls)
						}
					} else if err == nil || transactions != 0 || runs.calls != 0 || events.calls != 0 {
						t.Fatalf("out-of-range retry: err=%v transactions=%d", err, transactions)
					}
				})
			}
		}
	}
}
