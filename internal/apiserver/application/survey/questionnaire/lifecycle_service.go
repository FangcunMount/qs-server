package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("创建问卷",
		"action", "create",
		"title", dto.Title,
		"type", dto.Type,
		"has_code", dto.Code != "",
		"has_version", dto.Version != "",
	)

	// 1. 生成问卷编码（允许外部传入以支持导入场景）
	var code meta.Code
	var err error
	if dto.Code != "" {
		code = meta.NewCode(dto.Code)
		l.Debugw("使用外部提供的问卷编码",
			"action", "create",
			"code", dto.Code,
		)
	} else {
		code, err = meta.GenerateCode()
		if err != nil {
			l.Errorw("生成问卷编码失败",
				"action", "create",
				"result", "failed",
				"error", err.Error(),
			)
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "生成问卷编码失败")
		}
		l.Debugw("生成新的问卷编码",
			"action", "create",
			"code", code.String(),
		)
	}

	version := questionnaire.NewVersion("1.0")
	if dto.Version != "" {
		version = questionnaire.NewVersion(dto.Version)
	}
	qType := questionnaire.NormalizeQuestionnaireType(dto.Type)

	// 2. 创建问卷领域模型
	l.Debugw("创建问卷领域模型",
		"action", "create",
		"code", code.String(),
		"version", version.String(),
		"type", qType.String(),
	)
	q, err := questionnaire.NewQuestionnaire(
		meta.NewCode(code.String()),
		dto.Title,
		questionnaire.WithDesc(dto.Description),
		questionnaire.WithImgUrl(dto.ImgUrl),
		questionnaire.WithVersion(version),
		questionnaire.WithStatus(questionnaire.STATUS_DRAFT),
		questionnaire.WithType(qType),
	)
	if err != nil {
		l.Errorw("创建问卷领域模型失败",
			"action", "create",
			"code", code.String(),
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "创建问卷失败")
	}

	// 3. 持久化
	l.Debugw("保存问卷到数据库",
		"action", "create",
		"code", code.String(),
	)
	if err := s.repo.Create(ctx, q); err != nil {
		l.Errorw("保存问卷失败",
			"action", "create",
			"code", code.String(),
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	duration := time.Since(startTime)
	l.Debugw("创建问卷成功",
		"action", "create",
		"code", code.String(),
		"status", q.GetStatus().String(),
		"duration_ms", duration.Milliseconds(),
	)

	return toQuestionnaireResult(q), nil
}

// SaveDraft 保存草稿并更新版本
func (s *lifecycleService) SaveDraft(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("保存草稿",
		"action", "save_draft",
		"code", code,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, code, "save_draft"); err != nil {
		return nil, err
	}

	// 2. 获取现有问卷
	q, err := s.findQuestionnaireByCode(ctx, code, "save_draft")
	if err != nil {
		return nil, err
	}

	// 3. 只能保存草稿状态的问卷
	if !q.IsDraft() {
		l.Warnw("只能保存草稿状态的问卷",
			"action", "save_draft",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "只能保存草稿状态的问卷")
	}

	// 4. 递增小版本号（使用 Versioning 领域服务）
	oldVersion := q.GetVersion().String()
	l.Debugw("递增小版本号",
		"action", "save_draft",
		"code", code,
		"old_version", oldVersion,
	)
	versioning := questionnaire.Versioning{}
	if err := versioning.IncrementMinorVersion(q); err != nil {
		l.Errorw("更新版本号失败",
			"action", "save_draft",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "更新版本号失败")
	}

	// 5. 持久化
	if err := s.persistQuestionnaire(ctx, q, code, "save_draft", "草稿"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "save_draft", code, startTime,
		"version", q.GetVersion().String(),
	)

	return toQuestionnaireResult(q), nil
}

// UpdateBasicInfo 更新基本信息
func (s *lifecycleService) UpdateBasicInfo(ctx context.Context, dto UpdateQuestionnaireBasicInfoDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("更新基本信息",
		"action", "update_basic_info",
		"code", dto.Code,
		"title", dto.Title,
		"type", dto.Type,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, dto.Code, "update_basic_info"); err != nil {
		return nil, err
	}
	if dto.Title == "" {
		l.Warnw("问卷标题为空",
			"action", "update_basic_info",
			"code", dto.Code,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷标题不能为空")
	}

	// 2. 获取现有问卷
	q, err := s.findQuestionnaireByCode(ctx, dto.Code, "update_basic_info")
	if err != nil {
		return nil, err
	}

	// 3. 判断问卷状态
	if err := s.checkArchivedStatus(ctx, q, dto.Code, "update_basic_info", "编辑"); err != nil {
		return nil, err
	}

	// 4. 更新基本信息
	l.Debugw("更新基本信息",
		"action", "update_basic_info",
		"code", dto.Code,
	)
	baseInfo := questionnaire.BaseInfo{}
	if err := baseInfo.UpdateAll(q, dto.Title, dto.Description, dto.ImgUrl, questionnaire.NormalizeQuestionnaireType(dto.Type)); err != nil {
		l.Errorw("更新基本信息失败",
			"action", "update_basic_info",
			"code", dto.Code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidInput, "更新基本信息失败")
	}

	// 5. 持久化
	if err := s.persistQuestionnaire(ctx, q, dto.Code, "update_basic_info", "基本信息"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "update_basic_info", dto.Code, startTime)

	return toQuestionnaireResult(q), nil
}

// Publish 发布问卷
func (s *lifecycleService) Publish(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("发布问卷",
		"action", "publish",
		"code", code,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, code, "publish"); err != nil {
		return nil, err
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, code, "publish")
	if err != nil {
		return nil, err
	}

	// 3. 检查问卷状态
	if err := s.checkArchivedStatus(ctx, q, code, "publish", "发布"); err != nil {
		return nil, err
	}
	if q.IsPublished() {
		l.Warnw("问卷已发布，不能重复发布",
			"action", "publish",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷已发布，不能重复发布")
	}

	// 4. 检查问题列表
	questionsCount := len(q.GetQuestions())
	l.Debugw("检查问题列表",
		"action", "publish",
		"code", code,
		"questions_count", questionsCount,
	)
	if questionsCount == 0 {
		l.Warnw("问卷没有问题，不能发布",
			"action", "publish",
			"code", code,
			"result", "invalid_question",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问卷没有问题，不能发布")
	}

	// 5. 发布问卷（Lifecycle 会自动递增大版本号并更新状态为已发布）
	l.Debugw("执行发布流程",
		"action", "publish",
		"code", code,
		"current_version", q.GetVersion().String(),
	)
	if err := s.lifecycle.Publish(ctx, q); err != nil {
		l.Errorw("发布问卷失败",
			"action", "publish",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	// 6. 持久化
	if err := s.persistQuestionnaire(ctx, q, code, "publish", "状态"); err != nil {
		return nil, err
	}

	// 7. 发布聚合根收集的领域事件
	s.publishEvents(ctx, q)

	s.logSuccess(ctx, "publish", code, startTime,
		"version", q.GetVersion().String(),
		"status", q.GetStatus().String(),
	)

	return toQuestionnaireResult(q), nil
}

// Unpublish 下架问卷
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("下架问卷",
		"action", "unpublish",
		"code", code,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, code, "unpublish"); err != nil {
		return nil, err
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, code, "unpublish")
	if err != nil {
		return nil, err
	}

	// 3. 检查问卷状态
	if err := s.checkArchivedStatus(ctx, q, code, "unpublish", "下架"); err != nil {
		return nil, err
	}
	if q.IsDraft() {
		l.Warnw("问卷是草稿状态，不需要下架",
			"action", "unpublish",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷是草稿状态，不需要下架")
	}

	// 4. 下架问卷（更新状态为草稿）
	l.Debugw("执行下架流程",
		"action", "unpublish",
		"code", code,
		"current_status", q.GetStatus().String(),
	)
	if err := s.lifecycle.Unpublish(ctx, q); err != nil {
		l.Errorw("下架问卷失败",
			"action", "unpublish",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	// 5. 持久化
	if err := s.persistQuestionnaire(ctx, q, code, "unpublish", "状态"); err != nil {
		return nil, err
	}

	// 6. 发布聚合根收集的领域事件
	s.publishEvents(ctx, q)

	s.logSuccess(ctx, "unpublish", code, startTime,
		"status", q.GetStatus().String(),
	)

	return toQuestionnaireResult(q), nil
}

// Archive 归档问卷
func (s *lifecycleService) Archive(ctx context.Context, code string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("归档问卷",
		"action", "archive",
		"code", code,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, code, "archive"); err != nil {
		return nil, err
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, code, "archive")
	if err != nil {
		return nil, err
	}

	// 3. 检查问卷状态
	if q.IsArchived() {
		l.Warnw("问卷已归档，不能重复归档",
			"action", "archive",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "问卷已归档，不能重复归档")
	}

	// 4. 归档问卷
	l.Debugw("执行归档流程",
		"action", "archive",
		"code", code,
		"current_status", q.GetStatus().String(),
	)
	if err := s.lifecycle.Archive(ctx, q); err != nil {
		l.Errorw("归档问卷失败",
			"action", "archive",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, err
	}

	// 5. 持久化
	if err := s.persistQuestionnaire(ctx, q, code, "archive", "状态"); err != nil {
		return nil, err
	}

	// 6. 发布聚合根收集的领域事件
	s.publishEvents(ctx, q)

	s.logSuccess(ctx, "archive", code, startTime,
		"status", q.GetStatus().String(),
	)

	return toQuestionnaireResult(q), nil
}

// Delete 删除问卷
func (s *lifecycleService) Delete(ctx context.Context, code string) error {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("删除问卷",
		"action", "delete",
		"code", code,
	)

	// 1. 验证输入参数
	if err := s.validateCode(ctx, code, "delete"); err != nil {
		return err
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, code, "delete")
	if err != nil {
		return err
	}

	// 3. 只能删除草稿状态的问卷
	if !q.IsDraft() {
		l.Warnw("只能删除草稿状态的问卷",
			"action", "delete",
			"code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidStatus, "只能删除草稿状态的问卷")
	}

	// 4. 删除问卷
	l.Debugw("执行硬删除",
		"action", "delete",
		"code", code,
	)
	if err := s.repo.HardDelete(ctx, code); err != nil {
		l.Errorw("删除问卷失败",
			"action", "delete",
			"code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "删除问卷失败")
	}

	s.logSuccess(ctx, "delete", code, startTime)

	return nil
}

// validateCode 验证问卷编码
func (s *lifecycleService) validateCode(ctx context.Context, code string, action string) error {
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
func (s *lifecycleService) findQuestionnaireByCode(ctx context.Context, code string, action string) (*questionnaire.Questionnaire, error) {
	logger.L(ctx).Debugw("查询问卷",
		"action", action,
		"code", code,
	)
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
	return q, nil
}

// checkArchivedStatus 检查问卷是否已归档
func (s *lifecycleService) checkArchivedStatus(ctx context.Context, q *questionnaire.Questionnaire, code string, action string, operation string) error {
	if q.IsArchived() {
		logger.L(ctx).Warnw("问卷已归档，不能执行操作",
			"action", action,
			"code", code,
			"operation", operation,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能执行该操作")
	}
	return nil
}

// persistQuestionnaire 持久化问卷
func (s *lifecycleService) persistQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire, code string, action string, operation string) error {
	logger.L(ctx).Debugw("保存问卷",
		"action", action,
		"code", code,
		"operation", operation,
	)
	if err := s.repo.Update(ctx, q); err != nil {
		logger.L(ctx).Errorw("保存问卷失败",
			"action", action,
			"code", code,
			"operation", operation,
			"result", "failed",
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}
	return nil
}

// logSuccess 记录成功日志
func (s *lifecycleService) logSuccess(ctx context.Context, action string, code string, startTime time.Time, extraFields ...interface{}) {
	duration := time.Since(startTime)
	fields := []interface{}{
		"action", action,
		"code", code,
		"duration_ms", duration.Milliseconds(),
	}
	fields = append(fields, extraFields...)
	logger.L(ctx).Debugw("操作成功", fields...)
}

// publishEvents 发布聚合根收集的领域事件
func (s *lifecycleService) publishEvents(ctx context.Context, q *questionnaire.Questionnaire) {
	if s.eventPublisher == nil {
		return
	}

	events := q.Events()
	for _, evt := range events {
		// 事件发布失败不影响主流程
		_ = s.eventPublisher.Publish(ctx, evt)
	}

	// 清空已发布的事件
	q.ClearEvents()
}
