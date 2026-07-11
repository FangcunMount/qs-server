package assessment

import (
	"context"
	"time"

	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

// waitService 等待测评报告服务
type waitService struct {
	managementService AssessmentManagementService
	registry          evaluationwaiter.Registry
	reportQuery       ReportQueryService
}

// NewWaitService 创建等待测评报告服务实例
func NewWaitService(managementService AssessmentManagementService, registry evaluationwaiter.Registry, reportQuery ...ReportQueryService) AssessmentWaitService {
	service := &waitService{
		managementService: managementService,
		registry:          registry,
	}
	if len(reportQuery) > 0 {
		service.reportQuery = reportQuery[0]
	}
	return service
}

// WaitReport 等待测评报告
func (s *waitService) WaitReport(ctx context.Context, assessmentID uint64) evaluationwaiter.StatusSummary {
	if summary, done := s.loadTerminalAssessmentSummary(ctx, assessmentID); done {
		return summary
	}
	if s.registry == nil {
		return s.waitForReportByPolling(ctx, assessmentID)
	}
	return s.waitForReportWithRegistry(ctx, assessmentID)
}

// waitForReportByPolling 等待测评报告通过轮询
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

// waitForReportWithRegistry 等待测评报告通过注册表
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

// loadTerminalAssessmentSummary 加载终端测评总结
func (s *waitService) loadTerminalAssessmentSummary(ctx context.Context, assessmentID uint64) (evaluationwaiter.StatusSummary, bool) {
	if s.managementService == nil {
		return evaluationwaiter.StatusSummary{}, false
	}
	result, err := s.managementService.GetByID(ctx, assessmentID)
	if err != nil || result == nil {
		return evaluationwaiter.StatusSummary{}, false
	}
	if result.Status == "evaluated" && s.reportQuery != nil {
		if report, reportErr := s.reportQuery.GetByAssessmentID(ctx, assessmentID); reportErr == nil && report != nil {
			result = cloneAsLegacyInterpreted(result, report.CreatedAt)
		}
	}
	return assessmentStatusSummary(result)
}

func cloneAsLegacyInterpreted(result *AssessmentResult, interpretedAt time.Time) *AssessmentResult {
	if result == nil {
		return nil
	}
	projected := *result
	projected.Status = "interpreted"
	projected.InterpretedAt = &interpretedAt
	return &projected
}

// assessmentStatusSummary 测评总结
func assessmentStatusSummary(result *AssessmentResult) (evaluationwaiter.StatusSummary, bool) {
	if result == nil || !isTerminalAssessmentStatus(result.Status) {
		return evaluationwaiter.StatusSummary{}, false
	}
	return buildAssessmentStatusSummary(result), true
}

// isTerminalAssessmentStatus 是否终端测评状态
func isTerminalAssessmentStatus(status string) bool {
	return status == "interpreted" || status == "failed"
}

// buildAssessmentStatusSummary 构建测评总结
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

// pendingAssessmentStatusSummary 待测评总结
func pendingAssessmentStatusSummary() evaluationwaiter.StatusSummary {
	return evaluationwaiter.StatusSummary{
		Status:    "pending",
		UpdatedAt: time.Now().Unix(),
	}
}
