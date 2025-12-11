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
	if code == "" {
		l.Warnw("问卷编码为空",
			"action", "get_by_code",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 从 MongoDB 获取问卷
	l.Debugw("查询问卷数据库",
		"code", code,
		"action", "read",
	)

	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		l.Errorw("获取问卷失败",
			"code", code,
			"action", "get_by_code",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}
	if q == nil {
		l.Warnw("问卷不存在",
			"code", code,
			"action", "get_by_code",
			"result", "not_found",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}

	duration := time.Since(startTime)
	l.Debugw("获取问卷成功",
		"code", code,
		"questionnaire_id", q.GetID(),
		"status", q.GetStatus().String(),
		"duration_ms", duration.Milliseconds(),
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
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	// 限制最大分页大小为 50
	if dto.PageSize > 50 {
		dto.PageSize = 50
		l.Debugw("分页大小超限，已调整为最大值",
			"action", "list",
			"max_page_size", 50,
		)
	}

	// 2. 获取问卷摘要列表（轻量级查询，不包含 questions 字段）
	if t, ok := dto.Conditions["type"].(string); ok && t != "" {
		dto.Conditions["type"] = questionnaire.NormalizeQuestionnaireType(t).String()
	}

	summaries, err := s.repo.FindSummaryList(ctx, dto.Page, dto.PageSize, dto.Conditions)
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

	duration := time.Since(startTime)
	l.Debugw("查询问卷摘要列表成功",
		"action", "list",
		"result", "success",
		"total_count", total,
		"page_count", len(summaries),
		"duration_ms", duration.Milliseconds(),
	)

	return toQuestionnaireSummaryListResult(summaries, total), nil
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
	if code == "" {
		l.Warnw("问卷编码为空",
			"action", "get_published_by_code",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 获取问卷
	l.Debugw("从数据库查询问卷",
		"code", code,
		"action", "read",
	)

	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		l.Errorw("获取问卷失败",
			"code", code,
			"action", "get_published_by_code",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}
	if q == nil {
		l.Warnw("问卷不存在",
			"code", code,
			"action", "get_published_by_code",
			"result", "not_found",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}

	// 3. 检查问卷状态
	l.Debugw("检查问卷发布状态",
		"code", code,
		"status", q.GetStatus().String(),
	)

	if !q.IsPublished() {
		l.Warnw("问卷未发布",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷未发布")
	}

	duration := time.Since(startTime)
	l.Debugw("获取已发布问卷成功",
		"code", code,
		"questionnaire_id", q.GetID(),
		"duration_ms", duration.Milliseconds(),
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
	if dto.Page <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	// 限制最大分页大小为 50
	if dto.PageSize > 50 {
		dto.PageSize = 50
		l.Debugw("分页大小超限，已调整为最大值",
			"action", "list_published",
			"max_page_size", 50,
		)
	}

	// 2. 添加状态过滤条件
	if dto.Conditions == nil {
		dto.Conditions = make(map[string]interface{})
	}
	dto.Conditions["status"] = uint8(questionnaire.STATUS_PUBLISHED)
	if t, ok := dto.Conditions["type"].(string); ok && t != "" {
		dto.Conditions["type"] = questionnaire.NormalizeQuestionnaireType(t).String()
	}

	// 3. 获取问卷摘要列表（轻量级查询）
	summaries, err := s.repo.FindSummaryList(ctx, dto.Page, dto.PageSize, dto.Conditions)
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

	duration := time.Since(startTime)
	l.Debugw("查询已发布问卷摘要列表成功",
		"action", "list_published",
		"result", "success",
		"total_count", total,
		"page_count", len(summaries),
		"duration_ms", duration.Milliseconds(),
	)

	return toQuestionnaireSummaryListResult(summaries, total), nil
}
