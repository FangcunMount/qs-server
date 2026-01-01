package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// queryService 问卷查询服务实现
// 行为者：所有用户
type queryService struct {
	repo questionnaire.Repository
}

// NewQueryService 创建问卷查询服务
func NewQueryService(
	repo questionnaire.Repository,
) QuestionnaireQueryService {
	return &queryService{
		repo: repo,
	}
}

// GetByCode 根据编码获取问卷
func (s *queryService) GetByCode(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("获取问卷",
		"action", "get_by_code",
		"resource", "questionnaire",
		"code", code,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, code, "get_by_code"); err != nil {
		return nil, err
	}

	// 2. 从 MongoDB 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, code, "get_by_code")
	if err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "get_by_code", startTime,
		"code", code,
		"status", q.GetStatus().String(),
	)

	return toQuestionnaireResult(q), nil
}

// List 查询问卷摘要列表（轻量级，不包含问题详情）
func (s *queryService) List(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireSummaryListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询问卷摘要列表",
		"action", "list",
		"page", dto.Page,
		"page_size", dto.PageSize,
		"conditions", dto.Conditions,
	)

	// 1. 验证分页参数
	pageSize, err := s.validatePaginationParams(ctx, dto.Page, dto.PageSize, "list")
	if err != nil {
		return nil, err
	}
	dto.PageSize = pageSize

	// 2. 获取问卷摘要列表（轻量级查询，不包含 questions 字段）
	if t, ok := dto.Conditions["type"].(string); ok && t != "" {
		dto.Conditions["type"] = questionnaire.NormalizeQuestionnaireType(t).String()
	}

	questionnaires, err := s.repo.FindBaseList(ctx, dto.Page, dto.PageSize, dto.Conditions)
	if err != nil {
		l.Errorw("查询问卷摘要列表失败",
			"action", "list",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 3. 获取总数
	total, err := s.repo.CountWithConditions(ctx, dto.Conditions)
	if err != nil {
		l.Errorw("获取问卷总数失败",
			"action", "list",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷总数失败")
	}

	// 4. 转换为结果对象
	result := toQuestionnaireSummaryListResult(questionnaires, total)
	s.logSuccess(ctx, "list", startTime,
		"total_count", total,
		"page_count", len(questionnaires),
	)

	return result, nil
}

// GetPublishedByCode 获取已发布的问卷
func (s *queryService) GetPublishedByCode(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("获取已发布问卷",
		"action", "get_published_by_code",
		"resource", "questionnaire",
		"code", code,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, code, "get_published_by_code"); err != nil {
		return nil, err
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, code, "get_published_by_code")
	if err != nil {
		return nil, err
	}

	// 3. 检查问卷状态
	if !q.IsPublished() {
		l.Warnw("问卷未发布",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷未发布")
	}

	s.logSuccess(ctx, "get_published_by_code", startTime,
		"code", code,
	)

	return toQuestionnaireResult(q), nil
}

// ListPublished 查询已发布问卷摘要列表（轻量级）
func (s *queryService) ListPublished(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireSummaryListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询已发布问卷摘要列表",
		"action", "list_published",
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	// 1. 验证分页参数
	pageSize, err := s.validatePaginationParams(ctx, dto.Page, dto.PageSize, "list_published")
	if err != nil {
		return nil, err
	}
	dto.PageSize = pageSize

	// 2. 添加状态过滤条件
	if dto.Conditions == nil {
		dto.Conditions = make(map[string]interface{})
	}
	dto.Conditions["status"] = uint8(questionnaire.STATUS_PUBLISHED)
	if t, ok := dto.Conditions["type"].(string); ok && t != "" {
		dto.Conditions["type"] = questionnaire.NormalizeQuestionnaireType(t).String()
	}

	// 3. 获取问卷摘要列表（轻量级查询）
	questionnaires, err := s.repo.FindBaseList(ctx, dto.Page, dto.PageSize, dto.Conditions)
	if err != nil {
		l.Errorw("查询已发布问卷摘要列表失败",
			"action", "list_published",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 4. 获取总数
	total, err := s.repo.CountWithConditions(ctx, dto.Conditions)
	if err != nil {
		l.Errorw("获取已发布问卷总数失败",
			"action", "list_published",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷总数失败")
	}

	result := toQuestionnaireSummaryListResult(questionnaires, total)
	s.logSuccess(ctx, "list_published", startTime,
		"total_count", total,
		"page_count", len(questionnaires),
	)

	return result, nil
}

// validateCode 验证问卷编码
func (s *queryService) validateCode(ctx context.Context, code string, action string) error {
	if code == "" {
		logger.L(ctx).Warnw("问卷编码为空",
			"action", action,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	return nil
}

// findQuestionnaireByCode 根据编码查找问卷
func (s *queryService) findQuestionnaireByCode(ctx context.Context, code string, action string) (*questionnaire.Questionnaire, error) {
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		logger.L(ctx).Errorw("获取问卷失败",
			"action", action,
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}
	if q == nil {
		logger.L(ctx).Warnw("问卷不存在",
			"action", action,
			"code", code,
			"result", "not_found",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}
	return q, nil
}

// validatePaginationParams 验证分页参数
func (s *queryService) validatePaginationParams(ctx context.Context, page, pageSize int, action string) (int, error) {
	if page <= 0 {
		return 0, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if pageSize <= 0 {
		return 0, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	// 限制最大分页大小为 50
	if pageSize > 50 {
		pageSize = 50
		logger.L(ctx).Debugw("分页大小超限，已调整为最大值",
			"action", action,
			"max_page_size", 50,
		)
	}
	return pageSize, nil
}

// logSuccess 记录成功日志
func (s *queryService) logSuccess(ctx context.Context, action string, startTime time.Time, extraFields ...interface{}) {
	duration := time.Since(startTime)
	fields := []interface{}{
		"action", action,
		"duration_ms", duration.Milliseconds(),
	}
	fields = append(fields, extraFields...)
	logger.L(ctx).Debugw("操作成功", fields...)
}
