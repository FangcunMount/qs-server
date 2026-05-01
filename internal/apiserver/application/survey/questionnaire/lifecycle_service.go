package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 问卷生命周期服务实现
// 行为者：问卷设计者/管理员
type lifecycleService struct {
	repo           questionnaire.Repository
	scaleSyncer    ScaleQuestionnaireBindingSyncer
	validator      questionnaire.Validator
	lifecycle      questionnaire.Lifecycle
	eventPublisher event.EventPublisher
}

// NewLifecycleService 创建问卷生命周期服务
func NewLifecycleService(
	repo questionnaire.Repository,
	scaleSyncer ScaleQuestionnaireBindingSyncer,
	validator questionnaire.Validator,
	lifecycle questionnaire.Lifecycle,
	eventPublisher event.EventPublisher,
) QuestionnaireLifecycleService {
	return &lifecycleService{
		repo:           repo,
		scaleSyncer:    scaleSyncer,
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

	q, err := s.createQuestionnaire(ctx, l, dto)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)
	l.Debugw("创建问卷成功",
		"action", "create",
		"code", q.GetCode().String(),
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

	q, err := s.saveDraftQuestionnaire(ctx, l, code)
	if err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "save_draft", code, startTime,
		"version", q.GetVersion().String(),
	)

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

	if err := s.publishQuestionnaireVersion(ctx, l, q, code); err != nil {
		return nil, err
	}

	s.publishEvents(ctx, q)

	s.logSuccess(ctx, "publish", code, startTime,
		"version", q.GetVersion().String(),
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

	if err := s.deleteQuestionnaire(ctx, l, code); err != nil {
		return err
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

func (s *lifecycleService) loadEditableHead(ctx context.Context, code string, action string, operation string) (*questionnaire.Questionnaire, error) {
	q, err := s.findQuestionnaireByCode(ctx, code, action)
	if err != nil {
		return nil, err
	}
	if err := s.checkArchivedStatus(ctx, q, code, action, operation); err != nil {
		return nil, err
	}
	if err := ensureEditableHead(ctx, s.repo, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "派生工作草稿失败")
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
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, q, nil, nil)
}

func (s *lifecycleService) syncScaleQuestionnaireVersion(ctx context.Context, questionnaireCode, version string) error {
	if s.scaleSyncer == nil || questionnaireCode == "" || version == "" {
		return nil
	}

	q, err := s.repo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		if questionnaire.IsNotFound(err) {
			return nil
		}
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "查询问卷失败")
	}
	if q == nil || q.GetType() != questionnaire.TypeMedicalScale {
		return nil
	}

	return s.scaleSyncer.SyncQuestionnaireVersion(ctx, questionnaireCode, version)
}
