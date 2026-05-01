package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// queryService 问卷查询服务实现
// 行为者：所有用户
type queryService struct {
	repo        questionnaire.Repository
	reader      surveyreadmodel.QuestionnaireReader
	identitySvc iambridge.IdentityResolver
	hotset      cachetarget.HotsetRecorder
}

// NewQueryService 创建问卷查询服务
func NewQueryService(
	repo questionnaire.Repository,
	identitySvc iambridge.IdentityResolver,
	hotset cachetarget.HotsetRecorder,
	reader surveyreadmodel.QuestionnaireReader,
) QuestionnaireQueryService {
	return &queryService{
		repo:        repo,
		reader:      reader,
		identitySvc: identitySvc,
		hotset:      hotset,
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
	s.recordHotset(ctx, cachetarget.NewStaticQuestionnaireWarmupTarget(code))

	return toQuestionnaireResult(q), nil
}

// List 查询问卷摘要列表（轻量级，不包含问题详情）
func (s *queryService) List(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireSummaryListResult, error) {
	return s.listQuestionnaireSummaries(ctx, "list", dto, false)
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

	// 2. 获取已发布问卷
	q, err := s.repo.FindPublishedByCode(ctx, code)
	if err != nil {
		l.Errorw("获取已发布问卷失败",
			"action", "get_published_by_code",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取已发布问卷失败")
	}
	if q == nil {
		l.Warnw("问卷未发布",
			"code", code,
			"result", "not_found",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷未发布")
	}

	s.logSuccess(ctx, "get_published_by_code", startTime,
		"code", code,
	)
	s.recordHotset(ctx, cachetarget.NewStaticQuestionnaireWarmupTarget(code))

	return toQuestionnaireResult(q), nil
}

// GetQuestionCount 获取问卷题目数量（轻量，不加载 questions）
func (s *queryService) GetQuestionCount(ctx context.Context, code string) (int32, error) {
	if err := s.validateCode(ctx, code, "get_question_count"); err != nil {
		return 0, err
	}

	q, err := s.repo.FindBaseByCode(ctx, code)
	if err != nil {
		return 0, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷题目数量失败")
	}
	if q == nil {
		return 0, errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在")
	}

	count, err := safeconv.IntToInt32(q.GetQuestionCnt())
	if err != nil {
		return 0, errors.WrapC(err, errorCode.ErrDatabase, "问卷题目数量溢出")
	}
	return count, nil
}

// ListPublished 查询已发布问卷摘要列表（轻量级）
func (s *queryService) ListPublished(ctx context.Context, dto ListQuestionnairesDTO) (*QuestionnaireSummaryListResult, error) {
	return s.listQuestionnaireSummaries(ctx, "list_published", dto, true)
}

func (s *queryService) listQuestionnaireSummaries(ctx context.Context, action string, dto ListQuestionnairesDTO, publishedOnly bool) (*QuestionnaireSummaryListResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询问卷摘要列表",
		"action", action,
		"page", dto.Page,
		"page_size", dto.PageSize,
		"filter", dto.Filter,
		"published_only", publishedOnly,
	)

	pageSize, err := s.validatePaginationParams(ctx, dto.Page, dto.PageSize, action)
	if err != nil {
		return nil, err
	}
	dto.PageSize = pageSize

	filter, err := s.normalizeQuestionnaireFilter(dto.Filter)
	if err != nil {
		return nil, err
	}
	if publishedOnly {
		filter.Status = questionnaire.STATUS_PUBLISHED.String()
	}

	questionnaires, err := s.listQuestionnaireRows(ctx, filter, surveyreadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize}, publishedOnly)
	if err != nil {
		l.Errorw("查询问卷摘要列表失败",
			"action", action,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷列表失败")
	}

	total, err := s.countQuestionnaireRows(ctx, filter, publishedOnly)
	if err != nil {
		l.Errorw("获取问卷总数失败",
			"action", action,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取问卷总数失败")
	}

	result := toQuestionnaireSummaryRowsResult(ctx, questionnaires, total, s.identitySvc)
	s.logSuccess(ctx, action, startTime,
		"total_count", total,
		"page_count", len(questionnaires),
	)

	return result, nil
}

func (s *queryService) listQuestionnaireRows(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest, publishedOnly bool) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	if publishedOnly {
		return s.reader.ListPublishedQuestionnaires(ctx, filter, page)
	}
	return s.reader.ListQuestionnaires(ctx, filter, page)
}

func (s *queryService) countQuestionnaireRows(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, publishedOnly bool) (int64, error) {
	if publishedOnly {
		return s.reader.CountPublishedQuestionnaires(ctx, filter)
	}
	return s.reader.CountQuestionnaires(ctx, filter)
}

func (s *queryService) recordHotset(ctx context.Context, target cachetarget.WarmupTarget) {
	if s == nil || s.hotset == nil {
		return
	}
	_ = s.hotset.Record(ctx, target)
}

func (s *queryService) normalizeQuestionnaireFilter(filter QuestionnaireListFilter) (surveyreadmodel.QuestionnaireFilter, error) {
	normalized := surveyreadmodel.QuestionnaireFilter{
		Status: filter.Status,
		Title:  filter.Title,
		Type:   filter.Type,
	}
	if normalized.Status != "" {
		parsed, ok := questionnaire.ParseStatus(normalized.Status)
		if !ok {
			return surveyreadmodel.QuestionnaireFilter{}, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "状态无效")
		}
		normalized.Status = parsed.String()
	}
	if normalized.Type != "" {
		normalized.Type = questionnaire.NormalizeQuestionnaireType(normalized.Type).String()
	}
	return normalized, nil
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
