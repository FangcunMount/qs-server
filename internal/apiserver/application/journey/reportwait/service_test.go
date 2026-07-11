package reportwait

import (
	"context"
	"errors"
	"testing"
	"time"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	reportqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
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

type assessmentReaderStub struct {
	called bool
	result *assessmentApp.AssessmentResult
}

func (s *assessmentReaderStub) GetByID(_ context.Context, assessmentID uint64) (*assessmentApp.AssessmentResult, error) {
	s.called = true
	if s.result != nil && s.result.ID != assessmentID {
		return nil, errors.New("unexpected assessment")
	}
	return s.result, nil
}

type projectionStub struct {
	called bool
	at     time.Time
}

func (s *projectionStub) ProjectAssessment(_ context.Context, result *assessmentApp.AssessmentResult) (*reportqueryApp.AssessmentProjection, error) {
	s.called = true
	return &reportqueryApp.AssessmentProjection{Assessment: result, Status: "interpreted", InterpretedAt: &s.at}, nil
}

func TestWaitValidatesAccessBeforeDelegatingToCompletionWaiter(t *testing.T) {
	access := &accessStub{}
	reader := &assessmentReaderStub{result: &assessmentApp.AssessmentResult{ID: 56, Status: "evaluated"}}
	projection := &projectionStub{at: time.Unix(123, 0)}

	summary, err := NewService(access, reader, projection).Wait(context.Background(), Scope{OrgID: 12, OperatorUserID: 34, AssessmentID: 56})
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if access.lastScope != (Scope{OrgID: 12, OperatorUserID: 34, AssessmentID: 56}) {
		t.Fatalf("access scope = %#v", access.lastScope)
	}
	if !reader.called || !projection.called || summary.Status != "interpreted" {
		t.Fatalf("reader/projection/summary = %#v / %#v / %#v", reader, projection, summary)
	}
}

func TestWaitDoesNotRegisterWhenAssessmentIsInaccessible(t *testing.T) {
	access := &accessStub{err: errors.New("forbidden")}
	reader := &assessmentReaderStub{}

	if _, err := NewService(access, reader, &projectionStub{}).Wait(context.Background(), Scope{AssessmentID: 56}); err == nil {
		t.Fatal("Wait() error = nil, want access error")
	}
	if reader.called {
		t.Fatal("assessment reader must not be called before access succeeds")
	}
}

var _ AssessmentAccess = (*accessStub)(nil)
var _ AssessmentReader = (*assessmentReaderStub)(nil)
var _ LegacyProjection = (*projectionStub)(nil)

func TestWaitReturnsPendingWhenContextEndsBeforeReportExists(t *testing.T) {
	access := &accessStub{}
	reader := &assessmentReaderStub{result: &assessmentApp.AssessmentResult{ID: 56, Status: "evaluated"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	summary, err := NewService(access, reader, nil).Wait(ctx, Scope{AssessmentID: 56})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Status != "pending" || summary.UpdatedAt <= 0 {
		t.Fatalf("summary = %#v, want pending", summary)
	}
}
