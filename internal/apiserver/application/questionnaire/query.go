package questionnaire

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// QuestionnaireQuery 问卷查询器 - 负责所有问卷相关的读操作
// 面向业务场景，提供各种问卷查询和搜索功能
type QuestionnaireQuery struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewQuestionnaireQuery 创建问卷查询器
func NewQuestionnaireQuery(questionnaireRepo storage.QuestionnaireRepository) *QuestionnaireQuery {
	return &QuestionnaireQuery{
		questionnaireRepo: questionnaireRepo,
	}
}

// 单个问卷查询相关业务

// GetQuestionnaireByID 根据ID获取问卷
// 业务场景：查看问卷详情、问卷编辑页面
func (q *QuestionnaireQuery) GetQuestionnaireByID(ctx context.Context, questionnaireID string) (*QuestionnaireDTO, error) {
	// 验证参数
	if questionnaireID == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidID, "问卷ID不能为空")
	}

	// 查询问卷
	existingQuestionnaire, err := q.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(questionnaireID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}

	// 转换为DTO
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// GetQuestionnaireByCode 根据代码获取问卷
// 业务场景：通过问卷代码访问问卷、问卷分享
func (q *QuestionnaireQuery) GetQuestionnaireByCode(ctx context.Context, code string) (*QuestionnaireDTO, error) {
	// 验证参数
	if code == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCode, "问卷代码不能为空")
	}

	// 查询问卷
	existingQuestionnaire, err := q.questionnaireRepo.FindByCode(ctx, code)
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}

	// 转换为DTO
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// 问卷列表查询相关业务

// QuestionnaireListQuery 问卷列表查询参数
type QuestionnaireListQuery struct {
	Page      int    // 页码，从1开始
	PageSize  int    // 每页大小
	Status    string // 问卷状态筛选：draft, published, archived, all
	CreatorID string // 创建者ID筛选
	Keyword   string // 搜索关键词（标题或描述）
	SortBy    string // 排序字段：created_at, updated_at, title
	SortDir   string // 排序方向：asc, desc
}

// QuestionnaireListResult 问卷列表查询结果
type QuestionnaireListResult struct {
	Questionnaires []*QuestionnaireDTO `json:"questionnaires"`
	Total          int64               `json:"total"`
	Page           int                 `json:"page"`
	PageSize       int                 `json:"page_size"`
	TotalPages     int                 `json:"total_pages"`
}

// GetQuestionnaireList 获取问卷列表
// 业务场景：问卷管理页面、问卷列表展示、问卷搜索
func (q *QuestionnaireQuery) GetQuestionnaireList(ctx context.Context, query QuestionnaireListQuery) (*QuestionnaireListResult, error) {
	// 验证和设置默认值
	if err := q.validateListQuery(&query); err != nil {
		return nil, err
	}

	// 构建存储层查询参数
	var status *questionnaire.Status
	if query.Status != "all" {
		switch query.Status {
		case "draft":
			draftStatus := questionnaire.StatusDraft
			status = &draftStatus
		case "published":
			publishedStatus := questionnaire.StatusPublished
			status = &publishedStatus
		case "archived":
			archivedStatus := questionnaire.StatusArchived
			status = &archivedStatus
		}
	}

	var keyword *string
	if query.Keyword != "" {
		keyword = &query.Keyword
	}

	var creatorID *string
	if query.CreatorID != "" {
		creatorID = &query.CreatorID
	}

	storageQuery := storage.QueryOptions{
		Offset:    (query.Page - 1) * query.PageSize,
		Limit:     query.PageSize,
		Status:    status,
		CreatorID: creatorID,
		Keyword:   keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortDir,
	}

	// 查询问卷列表
	result, err := q.questionnaireRepo.FindQuestionnaires(ctx, storageQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷列表失败")
	}

	// 转换为DTO
	questionnaireDTOs := make([]*QuestionnaireDTO, 0, len(result.Items))
	for _, qu := range result.Items {
		dto := &QuestionnaireDTO{}
		dto.FromDomain(qu)
		questionnaireDTOs = append(questionnaireDTOs, dto)
	}

	// 计算总页数
	totalPages := int(result.TotalCount) / query.PageSize
	if int(result.TotalCount)%query.PageSize > 0 {
		totalPages++
	}

	return &QuestionnaireListResult{
		Questionnaires: questionnaireDTOs,
		Total:          result.TotalCount,
		Page:           query.Page,
		PageSize:       query.PageSize,
		TotalPages:     totalPages,
	}, nil
}

// 用户问卷查询相关业务

// GetUserQuestionnaires 获取用户的问卷列表
// 业务场景：我的问卷页面、用户创建的问卷列表
func (q *QuestionnaireQuery) GetUserQuestionnaires(ctx context.Context, creatorID string, page, pageSize int) (*QuestionnaireListResult, error) {
	// 验证参数
	if creatorID == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCreator, "创建者ID不能为空")
	}

	// 构建查询参数
	query := QuestionnaireListQuery{
		Page:      page,
		PageSize:  pageSize,
		Status:    "all",
		CreatorID: creatorID,
		SortBy:    "updated_at",
		SortDir:   "desc",
	}

	return q.GetQuestionnaireList(ctx, query)
}

// GetPublishedQuestionnaires 获取已发布的问卷列表
// 业务场景：公开问卷列表页面、问卷广场
func (q *QuestionnaireQuery) GetPublishedQuestionnaires(ctx context.Context, page, pageSize int, keyword string) (*QuestionnaireListResult, error) {
	// 构建查询参数
	query := QuestionnaireListQuery{
		Page:     page,
		PageSize: pageSize,
		Status:   "published",
		Keyword:  keyword,
		SortBy:   "updated_at",
		SortDir:  "desc",
	}

	return q.GetQuestionnaireList(ctx, query)
}

// 问卷统计查询相关业务

// QuestionnaireStats 问卷统计信息
type QuestionnaireStats struct {
	TotalQuestionnaires     int64 `json:"total_questionnaires"`
	DraftQuestionnaires     int64 `json:"draft_questionnaires"`
	PublishedQuestionnaires int64 `json:"published_questionnaires"`
	ArchivedQuestionnaires  int64 `json:"archived_questionnaires"`
}

// GetQuestionnaireStats 获取问卷统计信息
// 业务场景：管理后台数据展示、问卷概览统计
func (q *QuestionnaireQuery) GetQuestionnaireStats(ctx context.Context) (*QuestionnaireStats, error) {
	// 简化实现：分别查询各状态的问卷数量
	allQuery := storage.QueryOptions{Offset: 0, Limit: 1}
	allResult, err := q.questionnaireRepo.FindQuestionnaires(ctx, allQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷统计信息失败")
	}

	draftStatus := questionnaire.StatusDraft
	draftQuery := storage.QueryOptions{Status: &draftStatus, Offset: 0, Limit: 1}
	draftResult, err := q.questionnaireRepo.FindQuestionnaires(ctx, draftQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷统计信息失败")
	}

	publishedStatus := questionnaire.StatusPublished
	publishedQuery := storage.QueryOptions{Status: &publishedStatus, Offset: 0, Limit: 1}
	publishedResult, err := q.questionnaireRepo.FindQuestionnaires(ctx, publishedQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷统计信息失败")
	}

	archivedStatus := questionnaire.StatusArchived
	archivedQuery := storage.QueryOptions{Status: &archivedStatus, Offset: 0, Limit: 1}
	archivedResult, err := q.questionnaireRepo.FindQuestionnaires(ctx, archivedQuery)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷统计信息失败")
	}

	return &QuestionnaireStats{
		TotalQuestionnaires:     allResult.TotalCount,
		DraftQuestionnaires:     draftResult.TotalCount,
		PublishedQuestionnaires: publishedResult.TotalCount,
		ArchivedQuestionnaires:  archivedResult.TotalCount,
	}, nil
}

// 问卷验证查询相关业务

// CheckQuestionnaireCodeExists 检查问卷代码是否存在
// 业务场景：问卷创建时的代码可用性检查
func (q *QuestionnaireQuery) CheckQuestionnaireCodeExists(ctx context.Context, code string) (bool, error) {
	// 验证参数
	if code == "" {
		return false, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCode, "问卷代码不能为空")
	}

	// 检查代码是否存在
	exists, err := q.questionnaireRepo.ExistsByCode(ctx, code)
	if err != nil {
		return false, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "检查问卷代码是否存在失败")
	}

	return exists, nil
}

// ValidateQuestionnaireAccess 验证问卷访问权限
// 业务场景：问卷访问控制、权限验证
func (q *QuestionnaireQuery) ValidateQuestionnaireAccess(ctx context.Context, questionnaireID, userID string) (*QuestionnaireDTO, error) {
	// 验证参数
	if questionnaireID == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidID, "问卷ID不能为空")
	}
	if userID == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCreator, "用户ID不能为空")
	}

	// 查询问卷
	existingQuestionnaire, err := q.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(questionnaireID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}

	// 检查访问权限
	if existingQuestionnaire.CreatedBy() != userID && !existingQuestionnaire.IsPublished() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireAccessDenied, "没有权限访问此问卷")
	}

	// 转换为DTO
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// 辅助方法

func (q *QuestionnaireQuery) validateListQuery(query *QuestionnaireListQuery) error {
	// 设置默认值
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100 // 限制最大页面大小
	}
	if query.Status == "" {
		query.Status = "all"
	}
	if query.SortBy == "" {
		query.SortBy = "updated_at"
	}
	if query.SortDir == "" {
		query.SortDir = "desc"
	}

	// 验证参数
	validStatuses := map[string]bool{
		"all":       true,
		"draft":     true,
		"published": true,
		"archived":  true,
	}
	if !validStatuses[query.Status] {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidStatus, "无效的问卷状态")
	}

	validSortFields := map[string]bool{
		"created_at": true,
		"updated_at": true,
		"title":      true,
	}
	if !validSortFields[query.SortBy] {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCode, "无效的排序字段")
	}

	validSortDirs := map[string]bool{
		"asc":  true,
		"desc": true,
	}
	if !validSortDirs[query.SortDir] {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCode, "无效的排序方向")
	}

	return nil
}
