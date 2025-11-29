package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 问卷生命周期服务实现
// 行为者：问卷设计者/管理员
type lifecycleService struct {
	repo           questionnaire.Repository
	validator      questionnaire.Validator
	lifecycle      questionnaire.Lifecycle
	eventPublisher event.EventPublisher
}

// NewLifecycleService 创建问卷生命周期服务
func NewLifecycleService(
	repo questionnaire.Repository,
	validator questionnaire.Validator,
	lifecycle questionnaire.Lifecycle,
	eventPublisher event.EventPublisher,
) QuestionnaireLifecycleService {
	return &lifecycleService{
		repo:           repo,
		validator:      validator,
		lifecycle:      lifecycle,
		eventPublisher: eventPublisher,
	}
}

// Create 创建问卷
func (s *lifecycleService) Create(ctx context.Context, dto CreateQuestionnaireDTO) (*QuestionnaireResult, error) {
	// 1. 生成问卷编码
	code, err := meta.GenerateCode()
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "生成问卷编码失败")
	}

	// 2. 创建问卷领域模型
	q, err := questionnaire.NewQuestionnaire(
		meta.NewCode(code.String()),
		dto.Title,
		questionnaire.WithDesc(dto.Description),
		questionnaire.WithImgUrl(dto.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewVersion("1.0")),
		questionnaire.WithStatus(questionnaire.STATUS_DRAFT),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "创建问卷失败")
	}

	// 3. 持久化
	if err := s.repo.Create(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	return toQuestionnaireResult(q), nil
}

// SaveDraft 保存草稿并更新版本
func (s *lifecycleService) SaveDraft(ctx context.Context, code string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 获取现有问卷
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 只能保存草稿状态的问卷
	if !q.IsDraft() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "只能保存草稿状态的问卷")
	}

	// 4. 递增小版本号（使用 Versioning 领域服务）
	versioning := questionnaire.Versioning{}
	if err := versioning.IncrementMinorVersion(q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "更新版本号失败")
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷草稿失败")
	}

	return toQuestionnaireResult(q), nil
}

// UpdateBasicInfo 更新基本信息
func (s *lifecycleService) UpdateBasicInfo(ctx context.Context, dto UpdateQuestionnaireBasicInfoDTO) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷标题不能为空")
	}

	// 2. 获取现有问卷
	q, err := s.repo.FindByCode(ctx, dto.Code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 更新基本信息
	baseInfo := questionnaire.BaseInfo{}
	if err := baseInfo.UpdateAll(q, dto.Title, dto.Description, dto.ImgUrl); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "更新基本信息失败")
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷基本信息失败")
	}

	return toQuestionnaireResult(q), nil
}

// Publish 发布问卷
func (s *lifecycleService) Publish(ctx context.Context, code string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 检查问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能发布")
	}
	if q.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷已发布，不能重复发布")
	}

	// 4. 检查问题列表
	if len(q.GetQuestions()) == 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问卷没有问题，不能发布")
	}

	// 5. 发布问卷（Lifecycle 会自动递增大版本号并更新状态为已发布）
	if err := s.lifecycle.Publish(ctx, q); err != nil {
		return nil, err
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷状态失败")
	}

	// 7. 发布问卷发布事件（异步通知缓存更新）
	if s.eventPublisher != nil {
		publishEvent := questionnaire.NewQuestionnairePublishedEvent(
			uint64(q.GetID()),
			q.GetCode().String(),
			q.GetVersion().String(),
			q.GetTitle(),
			time.Now(),
		)
		// 事件发布失败不影响主流程，仅记录日志
		_ = s.eventPublisher.Publish(ctx, publishEvent)
	}

	return toQuestionnaireResult(q), nil
}

// Unpublish 下架问卷
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 检查问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能下架")
	}
	if q.IsDraft() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷是草稿状态，不需要下架")
	}

	// 4. 下架问卷（更新状态为草稿）
	if err := s.lifecycle.Unpublish(ctx, q); err != nil {
		return nil, err
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷状态失败")
	}

	// 6. 发布问卷下架事件（异步通知缓存清除）
	if s.eventPublisher != nil {
		unpublishEvent := questionnaire.NewQuestionnaireUnpublishedEvent(
			uint64(q.GetID()),
			q.GetCode().String(),
			q.GetVersion().String(),
			time.Now(),
		)
		_ = s.eventPublisher.Publish(ctx, unpublishEvent)
	}

	return toQuestionnaireResult(q), nil
}

// Archive 归档问卷
func (s *lifecycleService) Archive(ctx context.Context, code string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 检查问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷已归档，不能重复归档")
	}

	// 4. 归档问卷
	if err := s.lifecycle.Archive(ctx, q); err != nil {
		return nil, err
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷状态失败")
	}

	// 6. 发布问卷归档事件（异步通知清除所有版本缓存）
	if s.eventPublisher != nil {
		archiveEvent := questionnaire.NewQuestionnaireArchivedEvent(
			uint64(q.GetID()),
			q.GetCode().String(),
			q.GetVersion().String(),
			time.Now(),
		)
		_ = s.eventPublisher.Publish(ctx, archiveEvent)
	}

	return toQuestionnaireResult(q), nil
}

// Delete 删除问卷
func (s *lifecycleService) Delete(ctx context.Context, code string) error {
	// 1. 验证输入参数
	if code == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 只能删除草稿状态的问卷
	if !q.IsDraft() {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "只能删除草稿状态的问卷")
	}

	// 4. 删除问卷
	if err := s.repo.HardDelete(ctx, code); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "删除问卷失败")
	}

	return nil
}
