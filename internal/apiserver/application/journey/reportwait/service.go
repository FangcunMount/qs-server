// Package reportwait composes authorized Assessment access with report-completion waiting.
package reportwait

import (
	"context"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

// Scope identifies the protected caller and Assessment whose report is awaited.
type Scope struct {
	OrgID          int64
	OperatorUserID int64
	AssessmentID   uint64
}

// AssessmentAccess verifies that a caller can view an Assessment before a wait
// is registered. It is deliberately narrower than the protected query facade.
type AssessmentAccess interface {
	LoadAccessibleAssessment(ctx context.Context, orgID int64, operatorUserID int64, assessmentID uint64) (*assessmentApp.AccessibleAssessmentContext, error)
}

// CompletionWaiter resolves a terminal report-completion summary or waits until
// the supplied context ends.
type CompletionWaiter interface {
	WaitReport(ctx context.Context, assessmentID uint64) evaluationwaiter.StatusSummary
}

// Service is the neutral journey use case for an authorized report wait.
type Service interface {
	Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error)
}

type service struct {
	access AssessmentAccess
	waiter CompletionWaiter
}

func NewService(access AssessmentAccess, waiter CompletionWaiter) Service {
	return &service{access: access, waiter: waiter}
}

func (s *service) Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error) {
	if _, err := s.access.LoadAccessibleAssessment(ctx, scope.OrgID, scope.OperatorUserID, scope.AssessmentID); err != nil {
		return evaluationwaiter.StatusSummary{}, err
	}
	return s.waiter.WaitReport(ctx, scope.AssessmentID), nil
}

var _ Service = (*service)(nil)
