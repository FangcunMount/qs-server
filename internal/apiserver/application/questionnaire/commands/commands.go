package commands

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/dto"
	appErrors "github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/errors"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// CreateQuestionnaireCommand 创建问卷命令
type CreateQuestionnaireCommand struct {
	Code        string `json:"code" binding:"required" validate:"required,min=3,max=50"`
	Title       string `json:"title" binding:"required" validate:"required,min=1,max=200"`
	Description string `json:"description" validate:"max=1000"`
	CreatedBy   string `json:"created_by" binding:"required" validate:"required"`
}

// Validate 验证命令
func (cmd *CreateQuestionnaireCommand) Validate() error {
	errors := appErrors.NewValidationErrors()

	if cmd.Code == "" {
		errors.Add("code", "code is required")
	} else if len(cmd.Code) < 3 || len(cmd.Code) > 50 {
		errors.Add("code", "code must be between 3 and 50 characters")
	}

	if cmd.Title == "" {
		errors.Add("title", "title is required")
	} else if len(cmd.Title) > 200 {
		errors.Add("title", "title must not exceed 200 characters")
	}

	if len(cmd.Description) > 1000 {
		errors.Add("description", "description must not exceed 1000 characters")
	}

	if cmd.CreatedBy == "" {
		errors.Add("created_by", "created_by is required")
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// CreateQuestionnaireHandler 创建问卷命令处理器
type CreateQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewCreateQuestionnaireHandler 创建命令处理器
func NewCreateQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *CreateQuestionnaireHandler {
	return &CreateQuestionnaireHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理创建问卷命令
func (h *CreateQuestionnaireHandler) Handle(ctx context.Context, cmd CreateQuestionnaireCommand) (*dto.QuestionnaireDTO, error) {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. 验证业务规则 - 检查代码是否已存在
	exists, err := h.questionnaireRepo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, appErrors.NewSystemError("Failed to check code existence", err)
	}
	if exists {
		return nil, appErrors.NewConflictError("questionnaire", "Code already exists")
	}

	// 3. 创建领域对象
	q := questionnaire.NewQuestionnaire(cmd.Code, cmd.Title, cmd.Description, cmd.CreatedBy)

	// 4. 持久化
	if err := h.questionnaireRepo.Save(ctx, q); err != nil {
		return nil, appErrors.NewSystemError("Failed to save questionnaire", err)
	}

	// 5. 转换为DTO返回
	result := &dto.QuestionnaireDTO{}
	result.FromDomain(q)
	return result, nil
}

// UpdateQuestionnaireCommand 更新问卷命令
type UpdateQuestionnaireCommand struct {
	ID          string `json:"id" binding:"required" validate:"required"`
	Title       string `json:"title" binding:"required" validate:"required,min=1,max=200"`
	Description string `json:"description" validate:"max=1000"`
}

// Validate 验证命令
func (cmd *UpdateQuestionnaireCommand) Validate() error {
	errors := appErrors.NewValidationErrors()

	if cmd.ID == "" {
		errors.Add("id", "id is required")
	}

	if cmd.Title == "" {
		errors.Add("title", "title is required")
	} else if len(cmd.Title) > 200 {
		errors.Add("title", "title must not exceed 200 characters")
	}

	if len(cmd.Description) > 1000 {
		errors.Add("description", "description must not exceed 1000 characters")
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// UpdateQuestionnaireHandler 更新问卷命令处理器
type UpdateQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewUpdateQuestionnaireHandler 创建命令处理器
func NewUpdateQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *UpdateQuestionnaireHandler {
	return &UpdateQuestionnaireHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理更新问卷命令
func (h *UpdateQuestionnaireHandler) Handle(ctx context.Context, cmd UpdateQuestionnaireCommand) (*dto.QuestionnaireDTO, error) {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. 获取领域对象
	q, err := h.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, appErrors.NewNotFoundError("questionnaire", cmd.ID)
		}
		return nil, appErrors.NewSystemError("Failed to find questionnaire", err)
	}

	// 3. 执行业务操作
	if err := q.UpdateBasicInfo(cmd.Title, cmd.Description); err != nil {
		return nil, appErrors.NewBusinessError("UPDATE_FAILED", err.Error())
	}

	// 4. 持久化
	if err := h.questionnaireRepo.Update(ctx, q); err != nil {
		return nil, appErrors.NewSystemError("Failed to update questionnaire", err)
	}

	// 5. 转换为DTO返回
	result := &dto.QuestionnaireDTO{}
	result.FromDomain(q)
	return result, nil
}

// PublishQuestionnaireCommand 发布问卷命令
type PublishQuestionnaireCommand struct {
	ID string `json:"id" binding:"required" validate:"required"`
}

// Validate 验证命令
func (cmd *PublishQuestionnaireCommand) Validate() error {
	if cmd.ID == "" {
		return appErrors.NewValidationError("id", "id is required")
	}
	return nil
}

// PublishQuestionnaireHandler 发布问卷命令处理器
type PublishQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewPublishQuestionnaireHandler 创建命令处理器
func NewPublishQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *PublishQuestionnaireHandler {
	return &PublishQuestionnaireHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理发布问卷命令
func (h *PublishQuestionnaireHandler) Handle(ctx context.Context, cmd PublishQuestionnaireCommand) error {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return err
	}

	// 2. 获取领域对象
	q, err := h.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return appErrors.NewNotFoundError("questionnaire", cmd.ID)
		}
		return appErrors.NewSystemError("Failed to find questionnaire", err)
	}

	// 3. 执行业务操作
	if err := q.Publish(); err != nil {
		return appErrors.NewBusinessError("PUBLISH_FAILED", err.Error())
	}

	// 4. 持久化
	if err := h.questionnaireRepo.Update(ctx, q); err != nil {
		return appErrors.NewSystemError("Failed to update questionnaire", err)
	}

	return nil
}

// DeleteQuestionnaireCommand 删除问卷命令
type DeleteQuestionnaireCommand struct {
	ID string `json:"id" binding:"required" validate:"required"`
}

// Validate 验证命令
func (cmd *DeleteQuestionnaireCommand) Validate() error {
	if cmd.ID == "" {
		return appErrors.NewValidationError("id", "id is required")
	}
	return nil
}

// DeleteQuestionnaireHandler 删除问卷命令处理器
type DeleteQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewDeleteQuestionnaireHandler 创建命令处理器
func NewDeleteQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *DeleteQuestionnaireHandler {
	return &DeleteQuestionnaireHandler{
		questionnaireRepo: questionnaireRepo,
	}
}

// Handle 处理删除问卷命令
func (h *DeleteQuestionnaireHandler) Handle(ctx context.Context, cmd DeleteQuestionnaireCommand) error {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return err
	}

	// 2. 检查是否存在
	exists, err := h.questionnaireRepo.ExistsByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		return appErrors.NewSystemError("Failed to check questionnaire existence", err)
	}
	if !exists {
		return appErrors.NewNotFoundError("questionnaire", cmd.ID)
	}

	// 3. 删除
	if err := h.questionnaireRepo.Remove(ctx, questionnaire.NewQuestionnaireID(cmd.ID)); err != nil {
		return appErrors.NewSystemError("Failed to delete questionnaire", err)
	}

	return nil
}

// CommandHandlers 命令处理器集合
type CommandHandlers struct {
	CreateQuestionnaire  *CreateQuestionnaireHandler
	UpdateQuestionnaire  *UpdateQuestionnaireHandler
	PublishQuestionnaire *PublishQuestionnaireHandler
	DeleteQuestionnaire  *DeleteQuestionnaireHandler
}

// NewCommandHandlers 创建命令处理器集合
func NewCommandHandlers(questionnaireRepo storage.QuestionnaireRepository) *CommandHandlers {
	return &CommandHandlers{
		CreateQuestionnaire:  NewCreateQuestionnaireHandler(questionnaireRepo),
		UpdateQuestionnaire:  NewUpdateQuestionnaireHandler(questionnaireRepo),
		PublishQuestionnaire: NewPublishQuestionnaireHandler(questionnaireRepo),
		DeleteQuestionnaire:  NewDeleteQuestionnaireHandler(questionnaireRepo),
	}
}
