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

	duration := time.Since(startTime)
	l.Debugw("获取问卷成功",
		"code", code,
		"questionnaire_id", q.GetID(),
		"status", q.GetStatus().String(),
		"duration_ms", duration.Milliseconds(),
	)

	return toQuestionnaireResult(q), nil
}

// List 查询问卷列表
func (s *queryService) List(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询问卷列表",
		"action", "list_questionnaires",
		"page", dto.Page,
		"page_size", dto.PageSize,
		"conditions", dto.Conditions,
	)

	// 1. 验证分页参数
	if dto.Page <= 0 {
		l.Warnw("页码有效性检查失败",
			"action", "list_questionnaires",
			"page", dto.Page,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		l.Warnw("每页数量有效性检查失败",
			"action", "list_questionnaires",
			"page_size", dto.PageSize,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		l.Warnw("每页数量超限",
			"action", "list_questionnaires",
			"page_size", dto.PageSize,
			"max_size", 100,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量不能超过100")
	}

	// 2. 获取问卷列表
	l.Debugw("开始查询问卷列表",
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	questionnaires, err := s.repo.FindList(ctx, dto.Page, dto.PageSize, dto.Conditions)
	if err != nil {
		l.Errorw("查询问卷列表失败",
			"action", "list_questionnaires",
			"page", dto.Page,
			"page_size", dto.PageSize,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 3. 获取总数
	l.Debugw("查询问卷总数",
		"action", "count",
	)

	total, err := s.repo.CountWithConditions(ctx, dto.Conditions)
	if err != nil {
		l.Errorw("获取问卷总数失败",
			"action", "list_questionnaires",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷总数失败")
	}

	duration := time.Since(startTime)
	l.Debugw("查询问卷列表成功",
		"action", "list_questionnaires",
		"result", "success",
		"total_count", total,
		"page_count", len(questionnaires),
		"duration_ms", duration.Milliseconds(),
	)

	return toQuestionnaireListResult(questionnaires, total), nil
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

// ListPublished 查询已发布问卷列表
func (s *queryService) ListPublished(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询已发布问卷列表",
		"action", "list_published",
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	// 1. 验证分页参数
	if dto.Page <= 0 {
		l.Warnw("页码有效性检查失败",
			"action", "list_published",
			"page", dto.Page,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		l.Warnw("每页数量有效性检查失败",
			"action", "list_published",
			"page_size", dto.PageSize,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		l.Warnw("每页数量超限",
			"action", "list_published",
			"page_size", dto.PageSize,
			"max_size", 100,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "每页数量不能超过100")
	}

	// 2. 添加状态过滤条件
	l.Debugw("添加状态过滤条件",
		"status_filter", "published",
	)

	if dto.Conditions == nil {
		dto.Conditions = make(map[string]interface{})
	}
	dto.Conditions["status"] = uint8(questionnaire.STATUS_PUBLISHED)

	// 3. 获取问卷列表
	l.Debugw("开始查询已发布问卷列表",
		"page", dto.Page,
		"page_size", dto.PageSize,
	)

	questionnaires, err := s.repo.FindList(ctx, dto.Page, dto.PageSize, dto.Conditions)
	if err != nil {
		l.Errorw("查询已发布问卷列表失败",
			"action", "list_published",
			"page", dto.Page,
			"page_size", dto.PageSize,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	// 4. 获取总数
	l.Debugw("查询已发布问卷总数",
		"action", "count",
	)

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
	l.Debugw("查询已发布问卷列表成功",
		"action", "list_published",
		"result", "success",
		"total_count", total,
		"page_count", len(questionnaires),
		"duration_ms", duration.Milliseconds(),
	)

	return toQuestionnaireListResult(questionnaires, total), nil
}
