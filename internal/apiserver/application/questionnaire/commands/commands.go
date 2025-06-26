package commands

import (
	"context"
	"strings"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// CreateQuestionnaireCommand 创建问卷命令
type CreateQuestionnaireCommand struct {
	Code        string `json:"code" binding:"required,min=3,max=50"`
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description,omitempty"`
	CreatorID   string `json:"creator_id" binding:"required"`
}

// Validate 验证命令
func (cmd CreateQuestionnaireCommand) Validate() error {
	if strings.TrimSpace(cmd.Code) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCode, "问卷代码不能为空")
	}
	if len(cmd.Code) < 3 || len(cmd.Code) > 50 {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCode, "问卷代码长度必须在3-50个字符之间")
	}
	if strings.TrimSpace(cmd.Title) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidTitle, "问卷标题不能为空")
	}
	if len(cmd.Title) > 200 {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidTitle, "问卷标题长度不能超过200个字符")
	}
	if strings.TrimSpace(cmd.CreatorID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCreator, "创建者ID不能为空")
	}
	return nil
}

// UpdateQuestionnaireCommand 更新问卷命令
type UpdateQuestionnaireCommand struct {
	ID          string  `json:"id" binding:"required"`
	Title       *string `json:"title,omitempty" binding:"omitempty,min=1,max=200"`
	Description *string `json:"description,omitempty"`
}

// Validate 验证命令
func (cmd UpdateQuestionnaireCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidID, "问卷ID不能为空")
	}
	if cmd.Title != nil && strings.TrimSpace(*cmd.Title) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidTitle, "问卷标题不能为空")
	}
	if cmd.Title != nil && len(*cmd.Title) > 200 {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidTitle, "问卷标题长度不能超过200个字符")
	}
	return nil
}

// PublishQuestionnaireCommand 发布问卷命令
type PublishQuestionnaireCommand struct {
	ID string `json:"id" binding:"required"`
}

// Validate 验证命令
func (cmd PublishQuestionnaireCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidID, "问卷ID不能为空")
	}
	return nil
}

// DeleteQuestionnaireCommand 删除问卷命令
type DeleteQuestionnaireCommand struct {
	ID string `json:"id" binding:"required"`
}

// Validate 验证命令
func (cmd DeleteQuestionnaireCommand) Validate() error {
	if strings.TrimSpace(cmd.ID) == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidID, "问卷ID不能为空")
	}
	return nil
}

// CreateQuestionnaireHandler 创建问卷命令处理器
type CreateQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewCreateQuestionnaireHandler 创建命令处理器
func NewCreateQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *CreateQuestionnaireHandler {
	return &CreateQuestionnaireHandler{questionnaireRepo: questionnaireRepo}
}

// Handle 处理创建问卷命令
func (h *CreateQuestionnaireHandler) Handle(ctx context.Context, cmd CreateQuestionnaireCommand) (*dto.QuestionnaireDTO, error) {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. 验证业务规则
	exists, err := h.questionnaireRepo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "检查问卷代码是否存在失败")
	}
	if exists {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireCodeAlreadyExists, "问卷代码已存在")
	}

	// 3. 创建领域对象
	q := questionnaire.NewQuestionnaire(cmd.Code, cmd.Title, cmd.Description, cmd.CreatorID)

	// 4. 持久化
	if err := h.questionnaireRepo.Save(ctx, q); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireCreateFailed, "保存问卷失败")
	}

	// 5. 转换为DTO返回
	result := &dto.QuestionnaireDTO{}
	result.FromDomain(q)
	return result, nil
}

// UpdateQuestionnaireHandler 更新问卷命令处理器
type UpdateQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewUpdateQuestionnaireHandler 创建命令处理器
func NewUpdateQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *UpdateQuestionnaireHandler {
	return &UpdateQuestionnaireHandler{questionnaireRepo: questionnaireRepo}
}

// Handle 处理更新问卷命令
func (h *UpdateQuestionnaireHandler) Handle(ctx context.Context, cmd UpdateQuestionnaireCommand) (*dto.QuestionnaireDTO, error) {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return nil, err
	}

	// 2. 获取现有问卷
	existingQuestionnaire, err := h.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}

	// 3. 检查问卷状态
	if existingQuestionnaire.IsPublished() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireAlreadyPublished, "已发布的问卷不能修改")
	}

	// 4. 更新问卷信息
	if cmd.Title != nil {
		existingQuestionnaire.ChangeTitle(*cmd.Title)
	}
	if cmd.Description != nil {
		existingQuestionnaire.ChangeDescription(*cmd.Description)
	}

	// 5. 持久化
	if err := h.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireUpdateFailed, "更新问卷失败")
	}

	// 6. 转换为DTO返回
	result := &dto.QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// PublishQuestionnaireHandler 发布问卷命令处理器
type PublishQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewPublishQuestionnaireHandler 创建命令处理器
func NewPublishQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *PublishQuestionnaireHandler {
	return &PublishQuestionnaireHandler{questionnaireRepo: questionnaireRepo}
}

// Handle 处理发布问卷命令
func (h *PublishQuestionnaireHandler) Handle(ctx context.Context, cmd PublishQuestionnaireCommand) error {
	// 1. 验证命令
	if err := cmd.Validate(); err != nil {
		return err
	}

	// 2. 获取现有问卷
	existingQuestionnaire, err := h.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(cmd.ID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}

	// 3. 检查问卷状态
	if existingQuestionnaire.IsPublished() {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireAlreadyPublished, "问卷已发布")
	}

	// 4. 执行发布操作
	if err := existingQuestionnaire.Publish(); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnairePublishFailed, "发布问卷失败: %v", err)
	}

	// 5. 持久化
	if err := h.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnairePublishFailed, "发布问卷失败")
	}

	return nil
}

// DeleteQuestionnaireHandler 删除问卷命令处理器
type DeleteQuestionnaireHandler struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewDeleteQuestionnaireHandler 创建命令处理器
func NewDeleteQuestionnaireHandler(questionnaireRepo storage.QuestionnaireRepository) *DeleteQuestionnaireHandler {
	return &DeleteQuestionnaireHandler{questionnaireRepo: questionnaireRepo}
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
		return internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "检查问卷是否存在失败")
	}
	if !exists {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
	}

	// 3. 删除
	if err := h.questionnaireRepo.Remove(ctx, questionnaire.NewQuestionnaireID(cmd.ID)); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireDeleteFailed, "删除问卷失败")
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
