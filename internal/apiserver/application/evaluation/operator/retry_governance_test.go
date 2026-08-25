package operator

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/component-base/pkg/event"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationrun "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type objectCheckerStub struct {
	decision appauthz.ObjectDecision
	err      error
	request  appauthz.ObjectCheckRequest
	calls    int
}

func (s *objectCheckerStub) CheckObject(_ context.Context, request appauthz.ObjectCheckRequest) (appauthz.ObjectDecision, error) {
	s.calls++
	s.request = request
	return s.decision, s.err
}

type retryRunsStub struct{ evaluationrun.Repository }

type eventStagerStub struct{ calls int }

func (s *eventStagerStub) Stage(context.Context, ...event.DomainEvent) error {
	s.calls++
	return nil
}

func TestGovernedRetryChecksObjectBeforeStatusAndTransaction(t *testing.T) {
	assessment := newAssessment(t, 1, 1)
	checker := &objectCheckerStub{decision: appauthz.ObjectDecision{Allowed: false}}
	txCalls := 0
	tx := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		txCalls++
		return fn(ctx)
	})
	events := &eventStagerStub{}
	service := NewGovernedRetryService(
		&assessmentRepoStub{items: map[uint64]*domainassessment.Assessment{1: assessment}},
		&retryRunsStub{}, tx, events, &accessCheckerStub{}, checker,
	)

	_, err := service.Authorize(context.Background(), Actor{OrgID: 1, OperatorUserID: 9}, GovernedRetryCommand{
		AssessmentID: 1, ExpectedAttempt: 0, Origin: retrygovernance.AttemptOriginManual,
		RequestID: "request-1", Reason: "manual retry",
		AuthorizationSubject: "user:9", AuthorizationDomain: "fangcun", AuthorizationAction: "retry",
	})
	if err == nil {
		t.Fatal("Authorize() error = nil, want IAM denial")
	}
	if checker.calls != 1 || txCalls != 0 || events.calls != 0 {
		t.Fatalf("checker=%d tx=%d events=%d", checker.calls, txCalls, events.calls)
	}
	attribute := checker.request.Attributes[appauthz.ObjectOriginTypeAttribute]
	if attribute.String == nil || *attribute.String != "adhoc" || checker.request.ObjectID != "1" || checker.request.Resource != appauthz.AssessmentResource {
		t.Fatalf("object check request = %#v", checker.request)
	}
}

func TestGovernedRetryMapsIAMUnavailableBeforeSideEffects(t *testing.T) {
	checker := &objectCheckerStub{err: appauthz.ErrAuthorizationUnavailable}
	txCalls := 0
	service := NewGovernedRetryService(
		&assessmentRepoStub{items: map[uint64]*domainassessment.Assessment{1: newAssessment(t, 1, 1)}},
		&retryRunsStub{}, apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
			txCalls++
			return fn(ctx)
		}), &eventStagerStub{}, &accessCheckerStub{}, checker,
	)
	_, err := service.Authorize(context.Background(), Actor{OrgID: 1, OperatorUserID: 9}, GovernedRetryCommand{
		AssessmentID: 1, Origin: retrygovernance.AttemptOriginManual, RequestID: "request-1", Reason: "retry",
		AuthorizationSubject: "user:9", AuthorizationDomain: "fangcun", AuthorizationAction: "retry",
	})
	if err == nil || !errors.Is(err, appauthz.ErrAuthorizationUnavailable) || txCalls != 0 {
		t.Fatalf("err=%v tx=%d", err, txCalls)
	}
}
