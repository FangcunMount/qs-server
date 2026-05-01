package scale

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 量表生命周期服务实现
// 行为者：量表设计者/管理员
type lifecycleService struct {
	repo                 scale.Repository
	questionnaireCatalog questionnairecatalog.Catalog
	lifecycle            scale.Lifecycle
	baseInfo             scale.BaseInfo
	eventPublisher       event.EventPublisher
	listCache            scalelistcache.PublishedListCache
}

// NewLifecycleService 创建量表生命周期服务
func NewLifecycleService(
	repo scale.Repository,
	questionnaireCatalog questionnairecatalog.Catalog,
	eventPublisher event.EventPublisher,
	listCache scalelistcache.PublishedListCache,
) ScaleLifecycleService {
	return &lifecycleService{
		repo:                 repo,
		questionnaireCatalog: questionnaireCatalog,
		lifecycle:            scale.NewLifecycle(),
		baseInfo:             scale.BaseInfo{},
		eventPublisher:       eventPublisher,
		listCache:            listCache,
	}
}

// Publish 发布量表
func (s *lifecycleService) Publish(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 3. 如果问卷版本为空，自动从问卷仓库获取最新版本
	if err := s.ensureQuestionnaireVersion(ctx, code, m); err != nil {
		return nil, err
	}

	// 4. 执行生命周期操作并持久化
	return s.executeLifecycleOperation(ctx, m, func(ctx context.Context, scale *scale.MedicalScale) error {
		return s.lifecycle.Publish(ctx, scale)
	})
}

// Unpublish 下架量表
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 3. 执行生命周期操作并持久化
	return s.executeLifecycleOperation(ctx, m, func(ctx context.Context, scale *scale.MedicalScale) error {
		return s.lifecycle.Unpublish(ctx, scale)
	})
}

// Archive 归档量表
func (s *lifecycleService) Archive(ctx context.Context, code string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 3. 执行生命周期操作并持久化
	return s.executeLifecycleOperation(ctx, m, func(ctx context.Context, scale *scale.MedicalScale) error {
		return s.lifecycle.Archive(ctx, scale)
	})
}

// ===================== 私有辅助方法 =====================

func (s *lifecycleService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	logScaleListCacheError(ctx, s.listCache.Rebuild(ctx))
}

// generateScaleCode 生成量表编码
func (s *lifecycleService) generateScaleCode(code string) (meta.Code, error) {
	if code != "" {
		return meta.NewCode(code), nil
	}
	return meta.GenerateCode()
}

// getScaleByCode 根据编码获取量表
func (s *lifecycleService) getScaleByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	return m, nil
}

// getScaleAndValidateEditable 获取量表并验证是否可编辑
func (s *lifecycleService) getScaleAndValidateEditable(ctx context.Context, code string) (*scale.MedicalScale, error) {
	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 判断量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	return m, nil
}

// ensureQuestionnaireVersion 确保量表有关联的问卷版本
// 如果版本为空，自动从问卷仓库获取最新版本
func (s *lifecycleService) ensureQuestionnaireVersion(ctx context.Context, scaleCode string, m *scale.MedicalScale) error {
	if m.GetQuestionnaireCode().IsEmpty() {
		return nil
	}

	if err := s.validateMedicalScaleQuestionnaireBinding(ctx, m.GetQuestionnaireCode().Value(), m.GetQuestionnaireVersion(), scaleCode); err != nil {
		return err
	}

	if m.GetQuestionnaireVersion() != "" {
		return nil
	}

	questionnaireCode := m.GetQuestionnaireCode().Value()
	logger.L(ctx).Infow("问卷版本为空，自动获取最新版本",
		"scale_code", scaleCode,
		"questionnaire_code", questionnaireCode,
	)

	// 从问卷仓库获取问卷
	if s.questionnaireCatalog == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}
	q, err := s.questionnaireCatalog.FindPublishedQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷失败")
	}
	if q == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}

	// 更新量表的问卷版本
	latestVersion := q.Version
	logger.L(ctx).Infow("自动设置问卷版本",
		"scale_code", scaleCode,
		"questionnaire_code", questionnaireCode,
		"version", latestVersion,
	)
	if err := s.baseInfo.UpdateQuestionnaire(m, m.GetQuestionnaireCode(), latestVersion); err != nil {
		return errors.WrapC(err, errorCode.ErrInvalidArgument, "更新问卷版本失败")
	}

	// 保存更新后的量表
	if err := s.repo.Update(ctx, m); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存问卷版本失败")
	}

	return nil
}

func (s *lifecycleService) validateMedicalScaleQuestionnaireBinding(
	ctx context.Context,
	questionnaireCode string,
	questionnaireVersion string,
	currentScaleCode string,
) error {
	if questionnaireCode == "" {
		return nil
	}

	if s.questionnaireCatalog == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}
	q, err := s.questionnaireCatalog.FindQuestionnaire(ctx, questionnaireCode)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷失败")
	}
	if q == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}
	if q.Type != "MedicalScale" {
		return errors.WithCode(errorCode.ErrInvalidArgument, "量表只能关联 MedicalScale 类型问卷")
	}

	if questionnaireVersion != "" {
		versioned, err := s.questionnaireCatalog.FindQuestionnaireVersion(ctx, questionnaireCode, questionnaireVersion)
		if err != nil {
			return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷版本失败")
		}
		if versioned == nil {
			return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷版本不存在")
		}
		if versioned.Type != "MedicalScale" {
			return errors.WithCode(errorCode.ErrInvalidArgument, "量表只能关联 MedicalScale 类型问卷")
		}
	}

	boundScale, err := s.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		if scale.IsNotFound(err) {
			return nil
		}
		return errors.WrapC(err, errorCode.ErrDatabase, "查询问卷关联量表失败")
	}
	if boundScale == nil {
		return nil
	}
	if currentScaleCode != "" && boundScale.GetCode().String() == currentScaleCode {
		return nil
	}
	return errors.WithCode(errorCode.ErrInvalidArgument, "该问卷已关联其他量表")
}

// lifecycleOperation 生命周期操作函数类型
type lifecycleOperation func(ctx context.Context, scale *scale.MedicalScale) error

// executeLifecycleOperation 执行生命周期操作并持久化
// 统一的处理流程：执行操作 -> 持久化 -> 发布事件 -> 返回结果
func (s *lifecycleService) executeLifecycleOperation(
	ctx context.Context,
	m *scale.MedicalScale,
	operation lifecycleOperation,
) (*ScaleResult, error) {
	// 1. 执行生命周期操作
	if err := operation(ctx, m); err != nil {
		return nil, err
	}

	// 2. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表状态失败")
	}

	// 3. 发布聚合根收集的领域事件
	s.publishEvents(ctx, m)

	// 4. 重建全局列表缓存
	s.refreshListCache(ctx)

	return toScaleResult(m), nil
}

// publishEvents 发布聚合根收集的领域事件
func (s *lifecycleService) publishEvents(ctx context.Context, m *scale.MedicalScale) {
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, m, nil, nil)
}

func (s *lifecycleService) publishScaleChanged(ctx context.Context, m *scale.MedicalScale, action scale.ChangeAction) {
	if s.eventPublisher == nil || m == nil {
		return
	}
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, eventing.Collect(
		scale.NewScaleChangedEvent(
			m.GetID().Uint64(),
			m.GetCode().String(),
			"",
			m.GetTitle(),
			action,
			time.Now(),
		),
	), nil, nil)
}
