package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// lifecycleService 量表生命周期服务实现
// 行为者：量表设计者/管理员
type lifecycleService struct {
	repo              scale.Repository
	questionnaireRepo domainQuestionnaire.Repository
	lifecycle         scale.Lifecycle
	baseInfo          scale.BaseInfo
	eventPublisher    event.EventPublisher
	listCache         *ScaleListCache
}

// NewLifecycleService 创建量表生命周期服务
func NewLifecycleService(
	repo scale.Repository,
	questionnaireRepo domainQuestionnaire.Repository,
	eventPublisher event.EventPublisher,
	listCache *ScaleListCache,
) ScaleLifecycleService {
	return &lifecycleService{
		repo:              repo,
		questionnaireRepo: questionnaireRepo,
		lifecycle:         scale.NewLifecycle(),
		baseInfo:          scale.BaseInfo{},
		eventPublisher:    eventPublisher,
		listCache:         listCache,
	}
}

// Create 创建量表
func (s *lifecycleService) Create(ctx context.Context, dto CreateScaleDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	// 2. 生成量表编码
	code, err := s.generateScaleCode(dto.Code)
	if err != nil {
		return nil, err
	}

	// 3. 转换标签列表
	tags := make([]scale.Tag, 0, len(dto.Tags))
	for _, tagStr := range dto.Tags {
		tags = append(tags, scale.NewTag(tagStr))
	}

	// 4. 转换填报人列表
	reporters := make([]scale.Reporter, 0, len(dto.Reporters))
	for _, reporterStr := range dto.Reporters {
		reporters = append(reporters, scale.NewReporter(reporterStr))
	}

	// 5. 转换阶段列表
	stages := make([]scale.Stage, 0, len(dto.Stages))
	for _, stageStr := range dto.Stages {
		stages = append(stages, scale.NewStage(stageStr))
	}

	// 6. 转换使用年龄列表
	applicableAges := make([]scale.ApplicableAge, 0, len(dto.ApplicableAges))
	for _, ageStr := range dto.ApplicableAges {
		applicableAges = append(applicableAges, scale.NewApplicableAge(ageStr))
	}

	// 7. 创建量表领域模型
	m, err := scale.NewMedicalScale(
		code,
		dto.Title,
		scale.WithDescription(dto.Description),
		scale.WithCategory(scale.NewCategory(dto.Category)),
		scale.WithStages(stages),
		scale.WithApplicableAges(applicableAges),
		scale.WithReporters(reporters),
		scale.WithTags(tags),
		scale.WithQuestionnaire(meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion),
		scale.WithStatus(scale.StatusDraft),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建量表失败")
	}

	// 4. 持久化
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	s.refreshListCache(ctx)

	return toScaleResult(m), nil
}

// UpdateBasicInfo 更新基本信息
func (s *lifecycleService) UpdateBasicInfo(ctx context.Context, dto UpdateScaleBasicInfoDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表标题不能为空")
	}

	// 2. 获取现有量表并验证状态
	m, err := s.getScaleAndValidateEditable(ctx, dto.Code)
	if err != nil {
		return nil, err
	}

	// 3. 转换标签列表
	tags := make([]scale.Tag, 0, len(dto.Tags))
	for _, tagStr := range dto.Tags {
		tags = append(tags, scale.NewTag(tagStr))
	}

	// 4. 转换填报人列表
	reporters := make([]scale.Reporter, 0, len(dto.Reporters))
	for _, reporterStr := range dto.Reporters {
		reporters = append(reporters, scale.NewReporter(reporterStr))
	}

	// 5. 转换阶段列表
	stages := make([]scale.Stage, 0, len(dto.Stages))
	for _, stageStr := range dto.Stages {
		stages = append(stages, scale.NewStage(stageStr))
	}

	// 6. 转换使用年龄列表
	applicableAges := make([]scale.ApplicableAge, 0, len(dto.ApplicableAges))
	for _, ageStr := range dto.ApplicableAges {
		applicableAges = append(applicableAges, scale.NewApplicableAge(ageStr))
	}

	// 7. 更新基本信息和分类信息
	if err := s.baseInfo.UpdateAllWithClassification(m, dto.Title, dto.Description, scale.NewCategory(dto.Category), stages, applicableAges, reporters, tags); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新基本信息失败")
	}

	// 4. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表基本信息失败")
	}

	s.refreshListCache(ctx)

	return toScaleResult(m), nil
}

// UpdateQuestionnaire 更新关联的问卷
func (s *lifecycleService) UpdateQuestionnaire(ctx context.Context, dto UpdateScaleQuestionnaireDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.QuestionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷编码不能为空")
	}
	if dto.QuestionnaireVersion == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "问卷版本不能为空")
	}

	// 2. 获取现有量表并验证状态
	m, err := s.getScaleAndValidateEditable(ctx, dto.Code)
	if err != nil {
		return nil, err
	}

	// 3. 更新关联的问卷
	if err := s.baseInfo.UpdateQuestionnaire(m, meta.NewCode(dto.QuestionnaireCode), dto.QuestionnaireVersion); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新关联问卷失败")
	}

	// 4. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表关联问卷失败")
	}

	s.refreshListCache(ctx)

	return toScaleResult(m), nil
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

// Delete 删除量表
func (s *lifecycleService) Delete(ctx context.Context, code string) error {
	// 1. 验证输入参数
	if code == "" {
		return errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}

	// 2. 获取量表
	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return err
	}

	// 3. 只能删除草稿状态的量表
	if !m.IsDraft() {
		return errors.WithCode(errorCode.ErrInvalidArgument, "只能删除草稿状态的量表")
	}

	// 4. 删除量表
	if err := s.repo.Remove(ctx, code); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "删除量表失败")
	}

	s.refreshListCache(ctx)

	return nil
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
	if m.GetQuestionnaireVersion() != "" || m.GetQuestionnaireCode().IsEmpty() {
		return nil
	}

	questionnaireCode := m.GetQuestionnaireCode().Value()
	logger.L(ctx).Infow("问卷版本为空，自动获取最新版本",
		"scale_code", scaleCode,
		"questionnaire_code", questionnaireCode,
	)

	// 从问卷仓库获取问卷
	q, err := s.questionnaireRepo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取关联问卷失败")
	}
	if q == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireNotFound, "关联的问卷不存在")
	}

	// 更新量表的问卷版本
	latestVersion := q.GetVersion().Value()
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
	if s.eventPublisher == nil {
		return
	}

	events := m.Events()
	for _, evt := range events {
		// 事件发布失败不影响主流程
		_ = s.eventPublisher.Publish(ctx, evt)
	}

	// 清空已发布的事件
	m.ClearEvents()
}
