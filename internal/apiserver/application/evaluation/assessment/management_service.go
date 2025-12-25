package assessment

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
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
		"org_id", dto.OrgID,
		"page", dto.Page,
		"page_size", dto.PageSize,
		"conditions", dto.Conditions,
	)

	var assessments []*assessment.Assessment
	var total int64
	var err error

	// 构建分页参数
	pagination := assessment.NewPagination(dto.Page, dto.PageSize)

	// 如果有 testee_id 条件，使用 FindByTesteeID 查询
	if testeeIDStr, ok := dto.Conditions["testee_id"]; ok && testeeIDStr != "" {
		testeeIDUint, parseErr := strconv.ParseUint(testeeIDStr, 10, 64)
		if parseErr != nil {
			l.Errorw("解析受试者ID失败",
				"testee_id", testeeIDStr,
				"error", parseErr.Error(),
			)
			return nil, errors.WrapC(parseErr, errorCode.ErrAssessmentNotFound, "无效的受试者ID")
		}

		testeeID := testee.NewID(testeeIDUint)
		assessments, total, err = s.repo.FindByTesteeID(ctx, testeeID, pagination)
		if err != nil {
			l.Errorw("查询受试者测评列表失败",
				"testee_id", testeeIDStr,
				"error", err.Error(),
			)
			return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询测评列表失败")
		}
	} else {
		// 如果没有 testee_id，使用 orgID 查询
		if dto.OrgID == 0 {
			l.Warnw("未提供 testee_id 和 org_id，无法查询",
				"org_id", dto.OrgID,
			)
			assessments = make([]*assessment.Assessment, 0)
			total = 0
		} else {
			// 解析 status 条件
			var statusFilter *assessment.Status
			if statusStr, ok := dto.Conditions["status"]; ok && statusStr != "" {
				status := assessment.Status(statusStr)
				if status.IsValid() {
					statusFilter = &status
				} else {
					l.Warnw("无效的状态值",
						"status", statusStr,
					)
				}
			}

			// 使用 orgID 查询
			pagination := assessment.NewPagination(dto.Page, dto.PageSize)
			assessments, total, err = s.repo.FindByOrgID(ctx, int64(dto.OrgID), statusFilter, pagination)
			if err != nil {
				l.Errorw("查询组织测评列表失败",
					"org_id", dto.OrgID,
					"error", err.Error(),
				)
				return nil, errors.WrapC(err, errorCode.ErrDatabase, "查询测评列表失败")
			}
		}
	}

	// 如果通过 testee_id 查询，需要过滤 orgID 和 status
	filteredAssessments := assessments
	if testeeIDStr, ok := dto.Conditions["testee_id"]; ok && testeeIDStr != "" {
		filteredAssessments = make([]*assessment.Assessment, 0)
		for _, a := range assessments {
			// 过滤 orgID（如果提供了）
			if dto.OrgID > 0 && uint64(a.OrgID()) != dto.OrgID {
				continue
			}

			// 过滤 status（如果提供了）
			if statusStr, ok := dto.Conditions["status"]; ok && statusStr != "" {
				expectedStatus := assessment.Status(statusStr)
				if !expectedStatus.IsValid() {
					l.Warnw("无效的状态值",
						"status", statusStr,
					)
					continue
				}
				if a.Status() != expectedStatus {
					continue
				}
			}

			filteredAssessments = append(filteredAssessments, a)
		}
	}

	// 转换结果
	results := make([]*AssessmentResult, 0, len(filteredAssessments))
	for _, a := range filteredAssessments {
		results = append(results, toAssessmentResult(a))
	}

	// 计算总页数
	totalPages := int((total + int64(dto.PageSize) - 1) / int64(dto.PageSize))
	if totalPages == 0 && total > 0 {
		totalPages = 1
	}

	duration := time.Since(startTime)
	l.Debugw("查询测评列表成功",
		"action", "list_assessments",
		"result", "success",
		"org_id", dto.OrgID,
		"page", dto.Page,
		"page_size", dto.PageSize,
		"total_count", len(results),
		"total", total,
		"duration_ms", duration.Milliseconds(),
	)

	return &AssessmentListResult{
		Items:      results,
		Total:      int(total),
		Page:       dto.Page,
		PageSize:   dto.PageSize,
		TotalPages: totalPages,
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
