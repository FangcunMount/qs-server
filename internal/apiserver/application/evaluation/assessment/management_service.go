package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// managementService 测评管理服务实现
// 行为者：管理员 (Staff/Admin)
type managementService struct {
	repo        assessment.Repository
	reader      evaluationreadmodel.AssessmentReader
	txRunner    apptransaction.Runner
	eventStager EventStager
}

// NewManagementService 创建测评管理服务
func NewManagementService(repo assessment.Repository, _ event.EventPublisher) AssessmentManagementService {
	return &managementService{
		repo: repo,
	}
}

func NewManagementServiceWithReadModel(
	repo assessment.Repository,
	reader evaluationreadmodel.AssessmentReader,
	_ event.EventPublisher,
) AssessmentManagementService {
	return &managementService{
		repo:   repo,
		reader: reader,
	}
}

func NewManagementServiceWithTransactionalOutbox(
	repo assessment.Repository,
	_ event.EventPublisher,
	txRunner apptransaction.Runner,
	eventStager EventStager,
) AssessmentManagementService {
	return &managementService{
		repo:        repo,
		txRunner:    txRunner,
		eventStager: eventStager,
	}
}

func NewManagementServiceWithTransactionalOutboxAndReadModel(
	repo assessment.Repository,
	reader evaluationreadmodel.AssessmentReader,
	_ event.EventPublisher,
	txRunner apptransaction.Runner,
	eventStager EventStager,
) AssessmentManagementService {
	return &managementService{
		repo:        repo,
		reader:      reader,
		txRunner:    txRunner,
		eventStager: eventStager,
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

	result, convErr := toAssessmentResult(a)
	if convErr != nil {
		return nil, convErr
	}

	return result, nil
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

	orgID, err := safeconv.Uint64ToInt64(dto.OrgID)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "机构ID超出 int64 范围")
	}

	// 构建分页参数
	pagination := assessment.NewPagination(dto.Page, dto.PageSize)
	page := pagination.Page()
	pageSize := pagination.PageSize()

	conditions, err := parseAssessmentListConditions(dto.Conditions)
	if err != nil {
		l.Errorw("解析受试者ID失败",
			"testee_id", dto.Conditions["testee_id"],
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "无效的受试者ID")
	}
	if conditions.invalidStatus {
		l.Warnw("无效的状态值",
			"status", conditions.rawStatus,
		)
	}

	results, total, err := assessmentAdminQuery{reader: s.reader}.List(ctx, dto, orgID, pagination, conditions)
	if err != nil {
		return nil, err
	}

	// 计算总页数
	totalPages, err := safeconv.Int64ToInt((total + int64(pageSize) - 1) / int64(pageSize))
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrDatabase, "测评总页数超出安全范围")
	}
	if totalPages == 0 && total > 0 {
		totalPages = 1
	}
	totalInt, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, errors.WithCode(errorCode.ErrDatabase, "测评总数超出安全范围")
	}

	duration := time.Since(startTime)
	l.Debugw("查询测评列表成功",
		"action", "list_assessments",
		"result", "success",
		"org_id", dto.OrgID,
		"page", page,
		"page_size", pageSize,
		"total_count", len(results),
		"total", total,
		"duration_ms", duration.Milliseconds(),
	)

	return &AssessmentListResult{
		Items:      results,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *managementService) Retry(ctx context.Context, orgID int64, assessmentID uint64) (*AssessmentResult, error) {
	return assessmentRetryWorkflow{service: s}.Retry(ctx, orgID, assessmentID)
}

type assessmentRetryWorkflow struct {
	service *managementService
}

func (w assessmentRetryWorkflow) Retry(ctx context.Context, orgID int64, assessmentID uint64) (*AssessmentResult, error) {
	s := w.service
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始重试失败的测评",
		"action", "retry_assessment",
		"resource", "assessment",
		"org_id", orgID,
		"assessment_id", assessmentID,
	)

	a, err := s.loadAssessmentInOrg(ctx, orgID, assessmentID, "retry_assessment")
	if err != nil {
		return nil, err
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

	if err := saveAssessmentAndStageEvents(ctx, s.repo, s.txRunner, s.eventStager, a, nil); err != nil {
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

	result, convErr := toAssessmentResult(a)
	if convErr != nil {
		return nil, convErr
	}

	return result, nil
}

func (s *managementService) loadAssessmentInOrg(ctx context.Context, orgID int64, assessmentID uint64, action string) (*assessment.Assessment, error) {
	l := logger.L(ctx)

	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		l.Errorw("加载测评失败",
			"assessment_id", assessmentID,
			"action", action,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	if a.OrgID() != orgID {
		l.Warnw("测评写操作的机构范围校验失败",
			"assessment_id", assessmentID,
			"action", action,
			"request_org_id", orgID,
			"resource_org_id", a.OrgID(),
		)
		return nil, errors.WithCode(errorCode.ErrPermissionDenied, "测评不属于当前机构")
	}

	return a, nil
}
