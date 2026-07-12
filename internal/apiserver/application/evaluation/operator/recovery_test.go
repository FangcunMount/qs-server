package operator

import (
	"context"
	"errors"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type recoveryTx struct{}

func (recoveryTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type recoveryStager struct{}

func (recoveryStager) Stage(context.Context, ...event.DomainEvent) error { return nil }

func TestRecoveryAuthorizesTesteeBeforeChangingFailedAssessment(t *testing.T) {
	a := newAssessment(t, 1, 1)
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	if err := a.MarkAsFailedAt("failed", time.Now()); err != nil {
		t.Fatal(err)
	}
	repo := &assessmentRepoStub{items: map[uint64]*domainassessment.Assessment{1: a}}
	access := &accessCheckerStub{denied: map[uint64]error{101: errors.New("forbidden")}}
	service := NewRecoveryService(repo, recoveryTx{}, recoveryStager{}, access)

	if _, err := service.Retry(context.Background(), Actor{OrgID: 1, OperatorUserID: 9}, 1); err == nil {
		t.Fatal("Retry() error = nil, want access denial")
	}
	if !a.Status().IsFailed() {
		t.Fatalf("status = %s, want failed", a.Status())
	}
}

var _ apptransaction.Runner = recoveryTx{}
