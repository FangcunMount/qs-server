// Package reportwait composes authorized Assessment access with report-completion waiting.
package reportwait

import (
	"context"
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	reportqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

// Scope identifies the protected caller and Assessment whose report is awaited.
type Scope struct {
	OrgID          int64
	OperatorUserID int64
	AssessmentID   uint64
}

type AssessmentReportProjection interface {
	ProjectAssessment(ctx context.Context, result *evaluationoperator.Assessment) (*reportqueryApp.AssessmentProjection, error)
}

type AssessmentQuery interface {
	GetAssessment(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Assessment, error)
}

// Service is the neutral journey use case for an authorized report wait.
type Service interface {
	Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error)
}

type service struct {
	operator   AssessmentQuery
	projection AssessmentReportProjection
}

func NewService(operator AssessmentQuery, projection AssessmentReportProjection) Service {
	return &service{operator: operator, projection: projection}
}

func (s *service) Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error) {
	if _, err := s.operator.GetAssessment(ctx, evaluationoperator.Actor{OrgID: scope.OrgID, OperatorUserID: scope.OperatorUserID}, scope.AssessmentID); err != nil {
		return evaluationwaiter.StatusSummary{}, err
	}
	if summary, done := s.loadTerminalSummary(ctx, scope); done {
		return summary, nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return pendingSummary(), nil
		case <-ticker.C:
			if summary, done := s.loadTerminalSummary(ctx, scope); done {
				return summary, nil
			}
		}
	}
}

func (s *service) loadTerminalSummary(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, bool) {
	if s.operator == nil {
		return evaluationwaiter.StatusSummary{}, false
	}
	result, err := s.operator.GetAssessment(ctx, evaluationoperator.Actor{OrgID: scope.OrgID, OperatorUserID: scope.OperatorUserID}, scope.AssessmentID)
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
	return evaluationwaiter.StatusSummary{}, false
}

func statusSummary(result *evaluationoperator.Assessment, status string) evaluationwaiter.StatusSummary {
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
