package queries

import (
	"context"
	"strings"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// GetQuestionnaireQuery 获取问卷查询
type GetQuestionnaireQuery struct {
	ID   *string `form:"id" json:"id"`
	Code *string `form:"code" json:"code"`
}

// Validate 验证查询
func (q GetQuestionnaireQuery) Validate() error {
	if q.ID == nil && q.Code == nil {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireValidationFailed, "必须提供问卷ID或问卷代码中的一个")
	}
	return nil
}

// ListQuestionnairesQuery 获取问卷列表查询
type ListQuestionnairesQuery struct {
	interfaces.PaginationRequest
	dto.QuestionnaireFilterDTO
}

// Validate 验证查询
func (q ListQuestionnairesQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	return nil
}

// SearchQuestionnairesQuery 搜索问卷查询
type SearchQuestionnairesQuery struct {
	interfaces.PaginationRequest
	interfaces.FilterRequest
	interfaces.SortingRequest
}

// Validate 验证查询
func (q SearchQuestionnairesQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	return nil
}

// GetQuestionnaireStatisticsQuery 获取问卷统计查询
type GetQuestionnaireStatisticsQuery struct {
	ID string `form:"id" json:"id" binding:"required"`
}

// Validate 验证查询
func (q GetQuestionnaireStatisticsQuery) Validate() error {
	if strings.TrimSpace(q.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidID, "问卷ID不能为空")
	}
	return nil
}

// GetQuestionnaireHandler 获取问卷查询处理器
type GetQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewGetQuestionnaireHandler 创建查询处理器
func NewGetQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *GetQuestionnaireHandler {
	return &GetQuestionnaireHandler{questionnaireRepo: questionnaireRepo}
}

// Handle 处理获取问卷查询
func (h *GetQuestionnaireHandler) Handle(ctx context.Context, query GetQuestionnaireQuery) (*dto.QuestionnaireDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var foundQuestionnaire *questionnaire.Questionnaire
	var err error

	// 2. 根据不同条件查找问卷
	if query.ID != nil {
		foundQuestionnaire, err = h.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(*query.ID))
	} else if query.Code != nil {
		foundQuestionnaire, err = h.questionnaireRepo.FindByCode(ctx, *query.Code)
	}

	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}

	// 3. 转换为DTO
	result := &dto.QuestionnaireDTO{}
	result.FromDomain(foundQuestionnaire)
	return result, nil
}

// ListQuestionnairesHandler 获取问卷列表查询处理器
type ListQuestionnairesHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewListQuestionnairesHandler 创建查询处理器
func NewListQuestionnairesHandler(questionnaireRepo storage.QuestionnaireRepository) *ListQuestionnairesHandler {
	return &ListQuestionnairesHandler{questionnaireRepo: questionnaireRepo}
}

// Handle 处理获取问卷列表查询
func (h *ListQuestionnairesHandler) Handle(ctx context.Context, query ListQuestionnairesQuery) (*dto.QuestionnaireListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建查询选项
	queryOptions := storage.QueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		Status:    query.Status,
		CreatorID: query.CreatorID,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 3. 查询问卷列表
	result, err := h.questionnaireRepo.FindQuestionnaires(ctx, queryOptions)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷列表失败")
	}

	// 4. 转换为DTO
	items := dto.FromDomainList(result.Items)
	pagination := &interfaces.PaginationResponse{
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}

	return &dto.QuestionnaireListDTO{
		Items:      items,
		Pagination: pagination,
	}, nil
}

// SearchQuestionnairesHandler 搜索问卷查询处理器
type SearchQuestionnairesHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewSearchQuestionnairesHandler 创建查询处理器
func NewSearchQuestionnairesHandler(questionnaireRepo storage.QuestionnaireRepository) *SearchQuestionnairesHandler {
	return &SearchQuestionnairesHandler{questionnaireRepo: questionnaireRepo}
}

// Handle 处理搜索问卷查询
func (h *SearchQuestionnairesHandler) Handle(ctx context.Context, query SearchQuestionnairesQuery) (*dto.QuestionnaireListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建搜索查询选项
	queryOptions := storage.QueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 3. 执行搜索
	result, err := h.questionnaireRepo.FindQuestionnaires(ctx, queryOptions)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "搜索问卷失败")
	}

	// 4. 转换为DTO
	items := dto.FromDomainList(result.Items)
	pagination := &interfaces.PaginationResponse{
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}

	return &dto.QuestionnaireListDTO{
		Items:      items,
		Pagination: pagination,
	}, nil
}

// GetQuestionnaireStatisticsHandler 获取问卷统计查询处理器
type GetQuestionnaireStatisticsHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewGetQuestionnaireStatisticsHandler 创建查询处理器
func NewGetQuestionnaireStatisticsHandler(questionnaireRepo storage.QuestionnaireRepository) *GetQuestionnaireStatisticsHandler {
	return &GetQuestionnaireStatisticsHandler{questionnaireRepo: questionnaireRepo}
}

// Handle 处理获取问卷统计查询
func (h *GetQuestionnaireStatisticsHandler) Handle(ctx context.Context, query GetQuestionnaireStatisticsQuery) (*dto.QuestionnaireStatisticsDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 验证问卷是否存在
	_, err := h.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(query.ID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}

	// 3. 获取统计信息（简化实现）
	// 实际实现中，这里应该调用专门的统计方法
	queryOptions := storage.QueryOptions{Limit: 1} // 只获取总数
	result, err := h.questionnaireRepo.FindQuestionnaires(ctx, queryOptions)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "获取问卷统计失败")
	}

	// 4. 构建统计结果
	statistics := &dto.QuestionnaireStatisticsDTO{
		TotalCount:       result.TotalCount,
		StatusCounts:     make(map[questionnaire.Status]int64),
		CreatedToday:     0,                  // 需要实现具体逻辑
		CreatedThisWeek:  0,                  // 需要实现具体逻辑
		CreatedThisMonth: 0,                  // 需要实现具体逻辑
		PopularTags:      []dto.TagStatDTO{}, // 需要实现具体逻辑
	}

	return statistics, nil
}

// QueryHandlers 查询处理器集合
type QueryHandlers struct {
	GetQuestionnaire           *GetQuestionnaireHandler
	ListQuestionnaires         *ListQuestionnairesHandler
	SearchQuestionnaires       *SearchQuestionnairesHandler
	GetQuestionnaireStatistics *GetQuestionnaireStatisticsHandler
}

// NewQueryHandlers 创建查询处理器集合
func NewQueryHandlers(questionnaireRepo storage.QuestionnaireRepository) *QueryHandlers {
	return &QueryHandlers{
		GetQuestionnaire:           NewGetQuestionnaireHandler(questionnaireRepo),
		ListQuestionnaires:         NewListQuestionnairesHandler(questionnaireRepo),
		SearchQuestionnaires:       NewSearchQuestionnairesHandler(questionnaireRepo),
		GetQuestionnaireStatistics: NewGetQuestionnaireStatisticsHandler(questionnaireRepo),
	}
}
