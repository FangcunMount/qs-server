package services

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// QuestionnaireService 问卷应用服务
// 协调领域对象和端口，实现业务用例
type QuestionnaireService struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewQuestionnaireService 创建问卷应用服务
func NewQuestionnaireService(questionnaireRepo storage.QuestionnaireRepository) *QuestionnaireService {
	return &QuestionnaireService{
		questionnaireRepo: questionnaireRepo,
	}
}

// CreateQuestionnaireCommand 创建问卷命令
type CreateQuestionnaireCommand struct {
	Code        string `json:"code" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by" binding:"required"`
}

// CreateQuestionnaire 创建问卷用例
func (s *QuestionnaireService) CreateQuestionnaire(ctx context.Context, cmd CreateQuestionnaireCommand) (*questionnaire.Questionnaire, error) {
	// 1. 验证业务规则
	exists, err := s.questionnaireRepo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to check code existence: %w", err)
	}
	if exists {
		return nil, questionnaire.ErrDuplicateCode
	}

	// 2. 创建领域对象
	q := questionnaire.NewQuestionnaire(cmd.Code, cmd.Title, cmd.Description, cmd.CreatedBy)

	// 3. 持久化
	if err := s.questionnaireRepo.Save(ctx, q); err != nil {
		return nil, fmt.Errorf("failed to save questionnaire: %w", err)
	}

	return q, nil
}

// UpdateQuestionnaireCommand 更新问卷命令
type UpdateQuestionnaireCommand struct {
	ID          string `json:"id" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
}

// UpdateQuestionnaire 更新问卷用例
func (s *QuestionnaireService) UpdateQuestionnaire(ctx context.Context, cmd UpdateQuestionnaireCommand) (*questionnaire.Questionnaire, error) {
	// 1. 获取领域对象
	q, err := s.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to find questionnaire: %w", err)
	}

	// 2. 执行业务操作
	if err := q.UpdateBasicInfo(cmd.Title, cmd.Description); err != nil {
		return nil, err
	}

	// 3. 持久化
	if err := s.questionnaireRepo.Update(ctx, q); err != nil {
		return nil, fmt.Errorf("failed to update questionnaire: %w", err)
	}

	return q, nil
}

// PublishQuestionnaireCommand 发布问卷命令
type PublishQuestionnaireCommand struct {
	ID string `json:"id" binding:"required"`
}

// PublishQuestionnaire 发布问卷用例
func (s *QuestionnaireService) PublishQuestionnaire(ctx context.Context, cmd PublishQuestionnaireCommand) error {
	// 1. 获取领域对象
	q, err := s.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		return fmt.Errorf("failed to find questionnaire: %w", err)
	}

	// 2. 执行业务操作
	if err := q.Publish(); err != nil {
		return err
	}

	// 3. 持久化
	if err := s.questionnaireRepo.Update(ctx, q); err != nil {
		return fmt.Errorf("failed to update questionnaire: %w", err)
	}

	return nil
}

// GetQuestionnaireQuery 获取问卷查询
type GetQuestionnaireQuery struct {
	ID   *string `form:"id"`
	Code *string `form:"code"`
}

// GetQuestionnaire 获取问卷用例
func (s *QuestionnaireService) GetQuestionnaire(ctx context.Context, query GetQuestionnaireQuery) (*questionnaire.Questionnaire, error) {
	if query.ID != nil {
		return s.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(*query.ID))
	}

	if query.Code != nil {
		return s.questionnaireRepo.FindByCode(ctx, *query.Code)
	}

	return nil, fmt.Errorf("either ID or Code must be provided")
}

// ListQuestionnairesQuery 问卷列表查询
type ListQuestionnairesQuery struct {
	Page      int                   `form:"page"`
	PageSize  int                   `form:"page_size"`
	CreatorID *string               `form:"creator_id"`
	Status    *questionnaire.Status `form:"status"`
	Keyword   *string               `form:"keyword"`
	SortBy    string                `form:"sort_by"`
	SortOrder string                `form:"sort_order"`
}

// ListQuestionnairesResult 问卷列表结果
type ListQuestionnairesResult struct {
	Items      []*questionnaire.Questionnaire `json:"items"`
	TotalCount int64                          `json:"total_count"`
	HasMore    bool                           `json:"has_more"`
	Page       int                            `json:"page"`
	PageSize   int                            `json:"page_size"`
}

// ListQuestionnaires 获取问卷列表用例
func (s *QuestionnaireService) ListQuestionnaires(ctx context.Context, query ListQuestionnairesQuery) (*ListQuestionnairesResult, error) {
	// 设置默认值
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}

	// 构建存储查询
	storageQuery := storage.QueryOptions{
		Offset:    (query.Page - 1) * query.PageSize,
		Limit:     query.PageSize,
		CreatorID: query.CreatorID,
		Status:    query.Status,
		Keyword:   query.Keyword,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}

	// 执行查询
	result, err := s.questionnaireRepo.FindQuestionnaires(ctx, storageQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to find questionnaires: %w", err)
	}

	return &ListQuestionnairesResult{
		Items:      result.Items,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
		Page:       query.Page,
		PageSize:   query.PageSize,
	}, nil
}

// DeleteQuestionnaireCommand 删除问卷命令
type DeleteQuestionnaireCommand struct {
	ID string `json:"id" binding:"required"`
}

// DeleteQuestionnaire 删除问卷用例
func (s *QuestionnaireService) DeleteQuestionnaire(ctx context.Context, cmd DeleteQuestionnaireCommand) error {
	// 1. 检查是否存在
	exists, err := s.questionnaireRepo.ExistsByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		return fmt.Errorf("failed to check questionnaire existence: %w", err)
	}
	if !exists {
		return questionnaire.ErrQuestionnaireNotFound
	}

	// 2. 删除
	if err := s.questionnaireRepo.Remove(ctx, questionnaire.NewQuestionnaireID(cmd.ID)); err != nil {
		return fmt.Errorf("failed to delete questionnaire: %w", err)
	}

	return nil
}
