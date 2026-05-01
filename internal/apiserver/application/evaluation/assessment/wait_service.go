package assessment

import (
	"context"
	"time"

	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

type waitService struct {
	managementService AssessmentManagementService
	registry          evaluationwaiter.Registry
}

func NewWaitService(managementService AssessmentManagementService, registry evaluationwaiter.Registry) AssessmentWaitService {
	return &waitService{
		managementService: managementService,
		registry:          registry,
	}
}

func (s *waitService) WaitReport(ctx context.Context, assessmentID uint64) evaluationwaiter.StatusSummary {
	if summary, done := s.loadTerminalAssessmentSummary(ctx, assessmentID); done {
		return summary
	}
	if s.registry == nil {
		return s.waitForReportByPolling(ctx, assessmentID)
	}
	return s.waitForReportWithRegistry(ctx, assessmentID)
}

func (s *waitService) waitForReportByPolling(ctx context.Context, assessmentID uint64) evaluationwaiter.StatusSummary {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return pendingAssessmentStatusSummary()
		case <-ticker.C:
			if summary, done := s.loadTerminalAssessmentSummary(ctx, assessmentID); done {
				return summary
			}
		}
	}
}

func (s *waitService) waitForReportWithRegistry(ctx context.Context, assessmentID uint64) evaluationwaiter.StatusSummary {
	ch := make(chan evaluationwaiter.StatusSummary, 1)
	s.registry.Add(assessmentID, ch)
	defer s.registry.Remove(assessmentID, ch)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return pendingAssessmentStatusSummary()
		case summary := <-ch:
			return summary
		case <-ticker.C:
			if summary, done := s.loadTerminalAssessmentSummary(ctx, assessmentID); done {
				return summary
			}
		}
	}
}

func (s *waitService) loadTerminalAssessmentSummary(ctx context.Context, assessmentID uint64) (evaluationwaiter.StatusSummary, bool) {
	if s.managementService == nil {
		return evaluationwaiter.StatusSummary{}, false
	}
	result, err := s.managementService.GetByID(ctx, assessmentID)
	if err != nil || result == nil {
		return evaluationwaiter.StatusSummary{}, false
	}
	return assessmentStatusSummary(result)
}

func assessmentStatusSummary(result *AssessmentResult) (evaluationwaiter.StatusSummary, bool) {
	if result == nil || !isTerminalAssessmentStatus(result.Status) {
		return evaluationwaiter.StatusSummary{}, false
	}
	return buildAssessmentStatusSummary(result), true
}

func isTerminalAssessmentStatus(status string) bool {
	return status == "interpreted" || status == "failed"
}

func buildAssessmentStatusSummary(result *AssessmentResult) evaluationwaiter.StatusSummary {
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

	return evaluationwaiter.StatusSummary{
		Status:     result.Status,
		TotalScore: totalScore,
		RiskLevel:  riskLevel,
		UpdatedAt:  time.Now().Unix(),
	}
}

func pendingAssessmentStatusSummary() evaluationwaiter.StatusSummary {
	return evaluationwaiter.StatusSummary{
		Status:    "pending",
		UpdatedAt: time.Now().Unix(),
	}
}
