package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// managementService 测评管理服务实现
// 行为者：管理员 (Staff/Admin)
type managementService struct {
	repo assessment.Repository
}

// NewManagementService 创建测评管理服务
func NewManagementService(repo assessment.Repository) AssessmentManagementService {
	return &managementService{
		repo: repo,
	}
}

// GetByID 根据ID获取测评详情
func (s *managementService) GetByID(ctx context.Context, id uint64) (*AssessmentResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("获取测评详情",
		"action", "get_assessment",
		"resource", "assessment",
		"assessment_id", id,
	)

	assessmentID := meta.FromUint64(id)
	a, err := s.repo.FindByID(ctx, assessmentID)
	if err != nil {
		l.Errorw("获取测评失败",
			"assessment_id", id,
			"action", "get_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	duration := time.Since(startTime)
	l.Debugw("获取测评成功",
		"assessment_id", id,
		"status", a.Status().String(),
		"duration_ms", duration.Milliseconds(),
	)

	return toAssessmentResult(a), nil
}

// List 查询测评列表
func (s *managementService) List(ctx context.Context, dto ListAssessmentsDTO) (*AssessmentListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询测评列表",
		"action", "list_assessments",
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	// TODO: 实现复杂的条件查询
	// 当前先返回空列表，等仓储实现完善后再补充
	duration := time.Since(startTime)
	l.Debugw("查询测评列表成功",
		"action", "list_assessments",
		"result", "success",
		"page", dto.Page,
		"page_size", dto.PageSize,
		"total_count", 0,
		"duration_ms", duration.Milliseconds(),
	)

	return &AssessmentListResult{
		Items:      make([]*AssessmentResult, 0),
		Total:      0,
		Page:       dto.Page,
		PageSize:   dto.PageSize,
		TotalPages: 0,
	}, nil
}

// GetStatistics 获取测评统计
func (s *managementService) GetStatistics(ctx context.Context, dto GetStatisticsDTO) (*AssessmentStatistics, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询测评统计",
		"action", "get_statistics",
		"org_id", dto.OrgID,
	)

	// 统计各状态数量
	l.Debugw("开始统计各状态数量",
		"org_id", dto.OrgID,
	)

	pendingCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusPending)
	submittedCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusSubmitted)
	interpretedCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusInterpreted)
	failedCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusFailed)

	totalCount := int(pendingCount + submittedCount + interpretedCount + failedCount)

	duration := time.Since(startTime)
	l.Debugw("测评统计完成",
		"action", "get_statistics",
		"result", "success",
		"org_id", dto.OrgID,
		"total_count", totalCount,
		"pending_count", pendingCount,
		"submitted_count", submittedCount,
		"interpreted_count", interpretedCount,
		"failed_count", failedCount,
		"duration_ms", duration.Milliseconds(),
	)

	return &AssessmentStatistics{
		TotalCount:       totalCount,
		PendingCount:     int(pendingCount),
		SubmittedCount:   int(submittedCount),
		InterpretedCount: int(interpretedCount),
		FailedCount:      int(failedCount),
		AverageScore:     nil, // TODO: 计算平均分
		RiskDistribution: make(map[string]int),
		ScaleStats:       make([]ScaleStatistics, 0),
	}, nil
}

// Retry 重试失败的测评
func (s *managementService) Retry(ctx context.Context, assessmentID uint64) (*AssessmentResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始重试失败的测评",
		"action", "retry_assessment",
		"resource", "assessment",
		"assessment_id", assessmentID,
	)

	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		l.Errorw("加载测评失败",
			"assessment_id", assessmentID,
			"action", "retry_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 检查是否为失败状态
	l.Debugw("检查测评状态",
		"assessment_id", assessmentID,
		"status", a.Status().String(),
	)

	if !a.Status().IsFailed() {
		l.Warnw("测评状态不是失败状态，无法重试",
			"assessment_id", assessmentID,
			"status", a.Status().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrAssessmentInvalidStatus, "只能重试失败的测评")
	}

	// 重新提交，触发评估流程
	// 注意：需要在 Assessment 领域模型中添加 RetryFromFailed 方法
	l.Debugw("重置测评状态",
		"assessment_id", assessmentID,
		"old_status", a.Status().String(),
	)

	if err := a.RetryFromFailed(); err != nil {
		l.Errorw("重置测评状态失败",
			"assessment_id", assessmentID,
			"action", "retry_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentInvalidStatus, "重置测评状态失败")
	}

	l.Debugw("保存重试后的测评",
		"assessment_id", assessmentID,
		"new_status", a.Status().String(),
	)

	if err := s.repo.Save(ctx, a); err != nil {
		l.Errorw("保存测评失败",
			"assessment_id", assessmentID,
			"action", "retry_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	duration := time.Since(startTime)
	l.Infow("重试测评完成",
		"action", "retry_assessment",
		"result", "success",
		"assessment_id", assessmentID,
		"duration_ms", duration.Milliseconds(),
	)

	return toAssessmentResult(a), nil
}
