package questionnaire

import (
	"context"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
	internalErrors "github.com/yshujie/questionnaire-scale/internal/pkg/errors"
)

// QuestionnaireEditor 问卷编辑器 - 负责所有问卷相关的写操作
// 面向业务场景，隐藏 CQRS 的技术细节
type QuestionnaireEditor struct {
	questionnaireRepo storage.QuestionnaireRepository
}

// NewQuestionnaireEditor 创建问卷编辑器
func NewQuestionnaireEditor(questionnaireRepo storage.QuestionnaireRepository) *QuestionnaireEditor {
	return &QuestionnaireEditor{
		questionnaireRepo: questionnaireRepo,
	}
}

// 问卷创建相关业务

// CreateQuestionnaire 创建问卷
// 业务场景：用户创建新问卷
func (e *QuestionnaireEditor) CreateQuestionnaire(ctx context.Context, title, description, creatorID string) (*QuestionnaireDTO, error) {
	// 验证参数
	if err := e.validateCreateParams(title, description, creatorID); err != nil {
		return nil, err
	}

	// 创建问卷
	newQuestionnaire := questionnaire.NewQuestionnaire(generateQuestionnaireCode(), title, description, creatorID)

	// 保存问卷
	if err := e.questionnaireRepo.Save(ctx, newQuestionnaire); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireCreateFailed, "创建问卷失败")
	}

	// 返回结果
	result := &QuestionnaireDTO{}
	result.FromDomain(newQuestionnaire)
	return result, nil
}

// 问卷内容编辑相关业务

// UpdateQuestionnaireInfo 更新问卷基本信息
// 业务场景：用户修改问卷标题、描述等基本信息
func (e *QuestionnaireEditor) UpdateQuestionnaireInfo(ctx context.Context, questionnaireID, title, description string) (*QuestionnaireDTO, error) {
	// 验证参数
	if err := e.validateQuestionnaireID(questionnaireID); err != nil {
		return nil, err
	}
	if title == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidTitle, "问卷标题不能为空")
	}

	// 获取现有问卷
	existingQuestionnaire, err := e.getQuestionnaireByID(ctx, questionnaireID)
	if err != nil {
		return nil, err
	}

	// 检查问卷状态
	if existingQuestionnaire.IsPublished() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnairePublished, "已发布的问卷不能修改")
	}

	// 更新问卷信息
	existingQuestionnaire.ChangeTitle(title)
	existingQuestionnaire.ChangeDescription(description)

	// 保存更新
	if err := e.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireUpdateFailed, "更新问卷失败")
	}

	// 返回结果
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// AddQuestion 添加问题到问卷
// 业务场景：用户向问卷添加新问题
func (e *QuestionnaireEditor) AddQuestion(ctx context.Context, questionnaireID string, questionText string, questionType string, options []string) (*QuestionnaireDTO, error) {
	// 验证参数
	if err := e.validateQuestionnaireID(questionnaireID); err != nil {
		return nil, err
	}
	if questionText == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidQuestion, "问题内容不能为空")
	}

	// 获取现有问卷
	existingQuestionnaire, err := e.getQuestionnaireByID(ctx, questionnaireID)
	if err != nil {
		return nil, err
	}

	// 检查问卷状态
	if existingQuestionnaire.IsPublished() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnairePublished, "已发布的问卷不能修改")
	}

	// 添加问题
	question := questionnaire.NewQuestion(questionText, questionType, options)
	existingQuestionnaire.AddQuestion(question)

	// 保存更新
	if err := e.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireUpdateFailed, "添加问题失败")
	}

	// 返回结果
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// RemoveQuestion 从问卷移除问题
// 业务场景：用户从问卷中删除问题
func (e *QuestionnaireEditor) RemoveQuestion(ctx context.Context, questionnaireID, questionID string) (*QuestionnaireDTO, error) {
	// 验证参数
	if err := e.validateQuestionnaireID(questionnaireID); err != nil {
		return nil, err
	}
	if questionID == "" {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidQuestion, "问题ID不能为空")
	}

	// 获取现有问卷
	existingQuestionnaire, err := e.getQuestionnaireByID(ctx, questionnaireID)
	if err != nil {
		return nil, err
	}

	// 检查问卷状态
	if existingQuestionnaire.IsPublished() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnairePublished, "已发布的问卷不能修改")
	}

	// 移除问题
	if err := existingQuestionnaire.RemoveQuestion(questionID); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireInvalidQuestion, "移除问题失败")
	}

	// 保存更新
	if err := e.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireUpdateFailed, "移除问题失败")
	}

	// 返回结果
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// 问卷状态管理相关业务

// PublishQuestionnaire 发布问卷
// 业务场景：用户将问卷发布供他人填写
func (e *QuestionnaireEditor) PublishQuestionnaire(ctx context.Context, questionnaireID string) (*QuestionnaireDTO, error) {
	// 验证参数
	if err := e.validateQuestionnaireID(questionnaireID); err != nil {
		return nil, err
	}

	// 获取现有问卷
	existingQuestionnaire, err := e.getQuestionnaireByID(ctx, questionnaireID)
	if err != nil {
		return nil, err
	}

	// 检查问卷状态
	if existingQuestionnaire.IsPublished() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireAlreadyPublished, "问卷已发布")
	}

	// 验证问卷是否可以发布
	if !existingQuestionnaire.CanPublish() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidContent, "问卷内容不完整，无法发布")
	}

	// 发布问卷
	existingQuestionnaire.Publish()

	// 保存更新
	if err := e.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnairePublishFailed, "发布问卷失败")
	}

	// 返回结果
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// UnpublishQuestionnaire 取消发布问卷
// 业务场景：用户将已发布的问卷撤回到草稿状态
func (e *QuestionnaireEditor) UnpublishQuestionnaire(ctx context.Context, questionnaireID string) (*QuestionnaireDTO, error) {
	// 验证参数
	if err := e.validateQuestionnaireID(questionnaireID); err != nil {
		return nil, err
	}

	// 获取现有问卷
	existingQuestionnaire, err := e.getQuestionnaireByID(ctx, questionnaireID)
	if err != nil {
		return nil, err
	}

	// 检查问卷状态
	if !existingQuestionnaire.IsPublished() {
		return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotPublished, "问卷未发布")
	}

	// 取消发布问卷
	existingQuestionnaire.Unpublish()

	// 保存更新
	if err := e.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireUnpublishFailed, "取消发布问卷失败")
	}

	// 返回结果
	result := &QuestionnaireDTO{}
	result.FromDomain(existingQuestionnaire)
	return result, nil
}

// ArchiveQuestionnaire 归档问卷
// 业务场景：用户将问卷归档，不再使用
func (e *QuestionnaireEditor) ArchiveQuestionnaire(ctx context.Context, questionnaireID string) error {
	// 验证参数
	if err := e.validateQuestionnaireID(questionnaireID); err != nil {
		return err
	}

	// 获取现有问卷
	existingQuestionnaire, err := e.getQuestionnaireByID(ctx, questionnaireID)
	if err != nil {
		return err
	}

	// 归档问卷
	existingQuestionnaire.Archive()

	// 保存更新
	if err := e.questionnaireRepo.Update(ctx, existingQuestionnaire); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireArchiveFailed, "归档问卷失败")
	}

	return nil
}

// DeleteQuestionnaire 删除问卷
// 业务场景：用户删除问卷（软删除或硬删除）
func (e *QuestionnaireEditor) DeleteQuestionnaire(ctx context.Context, questionnaireID string) error {
	// 验证参数
	if err := e.validateQuestionnaireID(questionnaireID); err != nil {
		return err
	}

	// 检查问卷是否存在
	_, err := e.getQuestionnaireByID(ctx, questionnaireID)
	if err != nil {
		return err
	}

	// 删除问卷
	if err := e.questionnaireRepo.Remove(ctx, questionnaire.NewQuestionnaireID(questionnaireID)); err != nil {
		return internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireDeleteFailed, "删除问卷失败")
	}

	return nil
}

// 辅助方法

func (e *QuestionnaireEditor) validateCreateParams(title, description, creatorID string) error {
	if title == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidTitle, "问卷标题不能为空")
	}
	if len(title) < 2 || len(title) > 200 {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidTitle, "问卷标题长度必须在2-200个字符之间")
	}
	if creatorID == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidCreator, "创建者ID不能为空")
	}
	return nil
}

func (e *QuestionnaireEditor) validateQuestionnaireID(questionnaireID string) error {
	if questionnaireID == "" {
		return internalErrors.NewWithCode(internalErrors.ErrQuestionnaireInvalidID, "问卷ID不能为空")
	}
	return nil
}

func (e *QuestionnaireEditor) getQuestionnaireByID(ctx context.Context, questionnaireID string) (*questionnaire.Questionnaire, error) {
	existingQuestionnaire, err := e.questionnaireRepo.FindByID(ctx, questionnaire.NewQuestionnaireID(questionnaireID))
	if err != nil {
		if err == questionnaire.ErrQuestionnaireNotFound {
			return nil, internalErrors.NewWithCode(internalErrors.ErrQuestionnaireNotFound, "问卷不存在")
		}
		return nil, internalErrors.WrapWithCode(err, internalErrors.ErrQuestionnaireQueryFailed, "查询问卷失败")
	}
	return existingQuestionnaire, nil
}

// generateQuestionnaireCode 生成问卷代码
func generateQuestionnaireCode() string {
	// 简单实现，实际应该使用更复杂的生成策略
	return "Q" + time.Now().Format("20060102150405")
}
