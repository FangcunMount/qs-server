package queries

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/dto"
	appErrors "github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/errors"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// GetQuestionnaireQuery 获取问卷查询
type GetQuestionnaireQuery struct {
	ID   *string `form:"id" json:"id"`
	Code *string `form:"code" json:"code"`
}

// Validate 验证查询
func (q *GetQuestionnaireQuery) Validate() error {
	if q.ID == nil && q.Code == nil {
		return appErrors.NewValidationError("id_or_code", "Either ID or Code must be provided")
	}
	return nil
}

// GetQuestionnaireHandler 获取问卷查询处理器
type GetQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewGetQuestionnaireHandler 创建查询处理器
func NewGetQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *GetQuestionnaireHandler {
	return &GetQuestionnaireHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理获取问卷查询
func (h *GetQuestionnaireHandler) Handle(ctx context.Context, query GetQuestionnaireQuery) (*dto.QuestionnaireDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var q *questionnaire.Questionnaire
	var err error

	// 2. 执行查询
	if query.ID != nil {
		q, err = h.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(*query.ID))
	} else if query.Code != nil {
		q, err = h.questionnaireRepo.FindByCode(ctx, *query.Code)
	}

	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			identifier := ""
			if query.ID != nil {
				identifier = *query.ID
			} else {
				identifier = *query.Code
			}
			return nil, appErrors.NewNotFoundError("questionnaire", identifier)
		}
		return nil, appErrors.NewSystemError("Failed to find questionnaire", err)
	}

	// 3. 转换为DTO返回
	result := &dto.QuestionnaireDTO{}
	result.FromDomain(q)
	return result, nil
}

// ListQuestionnairesQuery 问卷列表查询
type ListQuestionnairesQuery struct {
	interfaces.PaginationRequest
	dto.QuestionnaireFilterDTO
}

// Validate 验证查询
func (q *ListQuestionnairesQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	q.QuestionnaireFilterDTO.SetDefaults()
	return nil
}

// ListQuestionnairesHandler 问卷列表查询处理器
type ListQuestionnairesHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewListQuestionnairesHandler 创建查询处理器
func NewListQuestionnairesHandler(questionnaireRepo storage.QuestionnaireRepository) *ListQuestionnairesHandler {
	return &ListQuestionnairesHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理问卷列表查询
func (h *ListQuestionnairesHandler) Handle(ctx context.Context, query ListQuestionnairesQuery) (*dto.QuestionnaireListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建存储查询
	storageQuery := storage.QueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		CreatorID: query.CreatorID,
		Status:    query.Status,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 3. 执行查询
	result, err := h.questionnaireRepo.FindQuestionnaires(ctx, storageQuery)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to find questionnaires", err)
	}

	// 4. 转换为DTO
	questionnairesDTOs := dto.FromDomainList(result.Items)
	pagination := interfaces.NewPaginationResponse(&query.PaginationRequest, result.TotalCount)

	return &dto.QuestionnaireListDTO{
		Items:      questionnairesDTOs,
		Pagination: pagination,
	}, nil
}

// SearchQuestionnairesQuery 问卷搜索查询
type SearchQuestionnairesQuery struct {
	interfaces.PaginationRequest
	interfaces.FilterRequest
	interfaces.SortingRequest

	AdvancedFilters dto.QuestionnaireFilterDTO `json:"filters"`
}

// Validate 验证查询
func (q *SearchQuestionnairesQuery) Validate() error {
	q.PaginationRequest.SetDefaults()
	q.SortingRequest.SetDefaults("updated_at")
	q.AdvancedFilters.SetDefaults()
	return nil
}

// SearchQuestionnairesHandler 问卷搜索查询处理器
type SearchQuestionnairesHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewSearchQuestionnairesHandler 创建查询处理器
func NewSearchQuestionnairesHandler(questionnaireRepo storage.QuestionnaireRepository) *SearchQuestionnairesHandler {
	return &SearchQuestionnairesHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理问卷搜索查询
func (h *SearchQuestionnairesHandler) Handle(ctx context.Context, query SearchQuestionnairesQuery) (*dto.QuestionnaireListDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 构建复杂查询
	storageQuery := storage.QueryOptions{
		Offset:    query.GetOffset(),
		Limit:     query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 应用高级过滤器
	if query.AdvancedFilters.HasCreatorFilter() {
		creatorID := query.AdvancedFilters.GetCreatorID()
		storageQuery.CreatorID = &creatorID
	}
	if query.AdvancedFilters.HasStatusFilter() {
		status := query.AdvancedFilters.GetStatus()
		storageQuery.Status = &status
	}
	if query.AdvancedFilters.HasKeyword() {
		keyword := query.AdvancedFilters.GetKeyword()
		storageQuery.Keyword = &keyword
	}

	// 3. 执行查询
	result, err := h.questionnaireRepo.FindQuestionnaires(ctx, storageQuery)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to search questionnaires", err)
	}

	// 4. 转换为DTO
	questionnairesDTOs := dto.FromDomainList(result.Items)
	pagination := interfaces.NewPaginationResponse(&query.PaginationRequest, result.TotalCount)

	return &dto.QuestionnaireListDTO{
		Items:      questionnairesDTOs,
		Pagination: pagination,
	}, nil
}

// GetQuestionnaireStatisticsQuery 获取问卷统计查询
type GetQuestionnaireStatisticsQuery struct {
	DateFrom *string `form:"date_from" json:"date_from"`
	DateTo   *string `form:"date_to" json:"date_to"`
}

// Validate 验证查询
func (q *GetQuestionnaireStatisticsQuery) Validate() error {
	// 可以添加日期格式验证等
	return nil
}

// GetQuestionnaireStatisticsHandler 获取问卷统计查询处理器
type GetQuestionnaireStatisticsHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewGetQuestionnaireStatisticsHandler 创建查询处理器
func NewGetQuestionnaireStatisticsHandler(questionnaireRepo storage.QuestionnaireRepository) *GetQuestionnaireStatisticsHandler {
	return &GetQuestionnaireStatisticsHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理获取问卷统计查询
func (h *GetQuestionnaireStatisticsHandler) Handle(ctx context.Context, query GetQuestionnaireStatisticsQuery) (*dto.QuestionnaireStatisticsDTO, error) {
	// 1. 验证查询
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 2. 获取基础统计
	// 这里可以调用仓储的统计方法
	// 简化实现，实际应该有专门的统计查询

	// 获取总数
	allQuery := storage.QueryOptions{Limit: 1} // 只要统计，不需要数据
	result, err := h.questionnaireRepo.FindQuestionnaires(ctx, allQuery)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to get questionnaire statistics", err)
	}

	// 3. 构建统计DTO
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
