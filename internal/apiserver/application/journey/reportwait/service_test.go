package reportwait

import (
	"context"
	"errors"
	"testing"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

type accessStub struct {
	err       error
	lastScope Scope
}

func (s *accessStub) LoadAccessibleAssessment(_ context.Context, orgID int64, operatorUserID int64, assessmentID uint64) (*assessmentApp.AccessibleAssessmentContext, error) {
	s.lastScope = Scope{OrgID: orgID, OperatorUserID: operatorUserID, AssessmentID: assessmentID}
	if s.err != nil {
		return nil, s.err
	}
	return &assessmentApp.AccessibleAssessmentContext{AssessmentID: assessmentID}, nil
}

type waiterStub struct {
	called       bool
	assessmentID uint64
	summary      evaluationwaiter.StatusSummary
}

func (s *waiterStub) WaitReport(_ context.Context, assessmentID uint64) evaluationwaiter.StatusSummary {
	s.called = true
	s.assessmentID = assessmentID
	return s.summary
}

func TestWaitValidatesAccessBeforeDelegatingToCompletionWaiter(t *testing.T) {
	access := &accessStub{}
	waiter := &waiterStub{summary: evaluationwaiter.StatusSummary{Status: "interpreted", UpdatedAt: 123}}

	summary, err := NewService(access, waiter).Wait(context.Background(), Scope{OrgID: 12, OperatorUserID: 34, AssessmentID: 56})
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if access.lastScope != (Scope{OrgID: 12, OperatorUserID: 34, AssessmentID: 56}) {
		t.Fatalf("access scope = %#v", access.lastScope)
	}
	if !waiter.called || waiter.assessmentID != 56 || summary.Status != "interpreted" {
		t.Fatalf("waiter/summary = %#v / %#v", waiter, summary)
	}
}

func TestWaitDoesNotRegisterWhenAssessmentIsInaccessible(t *testing.T) {
	access := &accessStub{err: errors.New("forbidden")}
	waiter := &waiterStub{}

	if _, err := NewService(access, waiter).Wait(context.Background(), Scope{AssessmentID: 56}); err == nil {
		t.Fatal("Wait() error = nil, want access error")
	}
	if waiter.called {
		t.Fatal("completion waiter must not be called before access succeeds")
	}
}

var _ AssessmentAccess = (*accessStub)(nil)
var _ CompletionWaiter = (*waiterStub)(nil)
