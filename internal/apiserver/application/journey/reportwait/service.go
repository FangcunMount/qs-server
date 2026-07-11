// Package reportwait composes authorized Assessment access with report-completion waiting.
package reportwait

import (
	"context"
	"time"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	reportqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
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

type AssessmentReader interface {
	GetByID(ctx context.Context, assessmentID uint64) (*assessmentApp.AssessmentResult, error)
}

type LegacyProjection interface {
	ProjectAssessment(ctx context.Context, result *assessmentApp.AssessmentResult) (*reportqueryApp.AssessmentProjection, error)
}

// Service is the neutral journey use case for an authorized report wait.
type Service interface {
	Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error)
}

type service struct {
	access     AssessmentAccess
	reader     AssessmentReader
	projection LegacyProjection
}

func NewService(access AssessmentAccess, reader AssessmentReader, projection LegacyProjection) Service {
	return &service{access: access, reader: reader, projection: projection}
}

func (s *service) Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error) {
	if _, err := s.access.LoadAccessibleAssessment(ctx, scope.OrgID, scope.OperatorUserID, scope.AssessmentID); err != nil {
		return evaluationwaiter.StatusSummary{}, err
	}
	if summary, done := s.loadTerminalSummary(ctx, scope.AssessmentID); done {
		return summary, nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return pendingSummary(), nil
		case <-ticker.C:
			if summary, done := s.loadTerminalSummary(ctx, scope.AssessmentID); done {
				return summary, nil
			}
		}
	}
}

func (s *service) loadTerminalSummary(ctx context.Context, assessmentID uint64) (evaluationwaiter.StatusSummary, bool) {
	if s.reader == nil {
		return evaluationwaiter.StatusSummary{}, false
	}
	result, err := s.reader.GetByID(ctx, assessmentID)
	if err != nil || result == nil {
		return evaluationwaiter.StatusSummary{}, false
	}
	if s.projection != nil {
		projected, projectErr := s.projection.ProjectAssessment(ctx, result)
		if projectErr != nil || projected == nil || projected.Assessment == nil {
			return evaluationwaiter.StatusSummary{}, false
		}
		if projected.Status != "interpreted" && projected.Status != "failed" {
			return evaluationwaiter.StatusSummary{}, false
		}
		return statusSummary(projected.Assessment, projected.Status), true
	}
	if result.Status != "interpreted" && result.Status != "failed" {
		return evaluationwaiter.StatusSummary{}, false
	}
	return statusSummary(result, result.Status), true
}

func statusSummary(result *assessmentApp.AssessmentResult, status string) evaluationwaiter.StatusSummary {
	var totalScore *float64
	if result.TotalScore != nil {
		value := *result.TotalScore
		totalScore = &value
	}
	var riskLevel *string
	if result.RiskLevel != nil {
		value := *result.RiskLevel
		riskLevel = &value
	}
	return evaluationwaiter.StatusSummary{Status: status, TotalScore: totalScore, RiskLevel: riskLevel, UpdatedAt: time.Now().Unix()}
}

func pendingSummary() evaluationwaiter.StatusSummary {
	return evaluationwaiter.StatusSummary{Status: "pending", UpdatedAt: time.Now().Unix()}
}

var _ Service = (*service)(nil)
