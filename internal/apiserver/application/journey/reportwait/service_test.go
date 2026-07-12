package reportwait

import (
	"context"
	"errors"
	"testing"
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	reportquery "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
)

type assessmentQueryStub struct {
	err       error
	result    *evaluationoperator.Assessment
	lastActor evaluationoperator.Actor
	called    int
}

func (s *assessmentQueryStub) GetAssessment(_ context.Context, actor evaluationoperator.Actor, id uint64) (*evaluationoperator.Assessment, error) {
	s.called++
	s.lastActor = actor
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil && s.result.ID != id {
		return nil, errors.New("unexpected assessment")
	}
	return s.result, nil
}

type projectionStub struct {
	called bool
	at     time.Time
}

func (s *projectionStub) ProjectAssessment(_ context.Context, result *evaluationoperator.Assessment) (*reportquery.AssessmentProjection, error) {
	s.called = true
	return &reportquery.AssessmentProjection{Assessment: result, Status: "interpreted", InterpretedAt: &s.at}, nil
}

func TestWaitAuthorizesAndProjectsWithoutExecutingEvaluation(t *testing.T) {
	query := &assessmentQueryStub{result: &evaluationoperator.Assessment{ID: 56, Status: "evaluated"}}
	projection := &projectionStub{at: time.Unix(123, 0)}
	summary, err := NewService(query, projection).Wait(context.Background(), Scope{OrgID: 12, OperatorUserID: 34, AssessmentID: 56})
	if err != nil {
		t.Fatal(err)
	}
	if query.lastActor != (evaluationoperator.Actor{OrgID: 12, OperatorUserID: 34}) || !projection.called || summary.Status != "interpreted" {
		t.Fatalf("query/projection/summary = %#v / %#v / %#v", query, projection, summary)
	}
}
func TestWaitStopsBeforePollingWhenAssessmentIsInaccessible(t *testing.T) {
	query := &assessmentQueryStub{err: errors.New("forbidden")}
	if _, err := NewService(query, nil).Wait(context.Background(), Scope{AssessmentID: 56}); err == nil {
		t.Fatal("want access error")
	}
	if query.called != 1 {
		t.Fatalf("calls=%d", query.called)
	}
}
func TestWaitReturnsPendingWhenContextEndsBeforeReportExists(t *testing.T) {
	query := &assessmentQueryStub{result: &evaluationoperator.Assessment{ID: 56, Status: "evaluated"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	summary, err := NewService(query, nil).Wait(ctx, Scope{AssessmentID: 56})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Status != "pending" || summary.UpdatedAt <= 0 {
		t.Fatalf("summary=%#v", summary)
	}
}

var _ AssessmentQuery = (*assessmentQueryStub)(nil)
var _ AssessmentReportProjection = (*projectionStub)(nil)
