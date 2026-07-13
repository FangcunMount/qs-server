// Package reportwait composes authorized Assessment access with report-completion waiting.
package reportwait

import (
	"context"
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	reportqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

// Scope 范围
type Scope struct {
	OrgID          int64  // 组织ID
	OperatorUserID int64  // 操作员用户ID
	AssessmentID   uint64 // 评估ID
}

// AssessmentReportProjection 评估报告投影
type AssessmentReportProjection interface {
	// ProjectAssessment 投影评估
	ProjectAssessment(ctx context.Context, result *evaluationoperator.Assessment) (*reportqueryApp.AssessmentProjection, error)
}

// AssessmentQuery 评估查询
type AssessmentQuery interface {
	// GetAssessment 获取评估
	GetAssessment(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Assessment, error)
}

// Service 服务
type Service interface {
	// Wait 等待报告
	Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error)
}

// service 服务实现
type service struct {
	operator   AssessmentQuery            // 评估查询
	projection AssessmentReportProjection // 评估报告投影
}

// NewService 创建服务
func NewService(operator AssessmentQuery, projection AssessmentReportProjection) Service {
	return &service{operator: operator, projection: projection}
}

// Wait 等待报告
func (s *service) Wait(ctx context.Context, scope Scope) (evaluationwaiter.StatusSummary, error) {
	// 获取评估
	if _, err := s.operator.GetAssessment(ctx, evaluationoperator.Actor{OrgID: scope.OrgID, OperatorUserID: scope.OperatorUserID}, scope.AssessmentID); err != nil {
		return evaluationwaiter.StatusSummary{}, err
	}
	// 加载终端摘要
	if summary, done := s.loadTerminalSummary(ctx, scope); done {
		return summary, nil
	}
	// 创建定时器
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

// loadTerminalSummary 加载终端摘要
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

// statusSummary 状态摘要
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

// pendingSummary 待处理摘要
func pendingSummary() evaluationwaiter.StatusSummary {
	return evaluationwaiter.StatusSummary{Status: "pending", UpdatedAt: time.Now().Unix()}
}

// _ Service = (*service)(nil)
var _ Service = (*service)(nil)
