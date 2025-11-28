package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
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
	assessmentID := meta.FromUint64(id)
	a, err := s.repo.FindByID(ctx, assessmentID)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	return toAssessmentResult(a), nil
}

// List 查询测评列表
func (s *managementService) List(ctx context.Context, dto ListAssessmentsDTO) (*AssessmentListResult, error) {
	// TODO: 实现复杂的条件查询
	// 当前先返回空列表，等仓储实现完善后再补充
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
	// 统计各状态数量
	pendingCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusPending)
	submittedCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusSubmitted)
	interpretedCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusInterpreted)
	failedCount, _ := s.repo.CountByOrgIDAndStatus(ctx, int64(dto.OrgID), assessment.StatusFailed)

	totalCount := int(pendingCount + submittedCount + interpretedCount + failedCount)

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
	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 检查是否为失败状态
	if !a.Status().IsFailed() {
		return nil, errors.WithCode(errorCode.ErrAssessmentInvalidStatus, "只能重试失败的测评")
	}

	// 重新提交，触发评估流程
	// 注意：需要在 Assessment 领域模型中添加 RetryFromFailed 方法
	if err := a.RetryFromFailed(); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrAssessmentInvalidStatus, "重置测评状态失败")
	}

	if err := s.repo.Save(ctx, a); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	return toAssessmentResult(a), nil
}
