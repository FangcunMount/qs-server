package lifecycle

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/editable"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// CacheSignalNotifier 缓存失效信令发布端口（best-effort，非领域事件）。
type CacheSignalNotifier interface {
	NotifyScaleCacheChanged(ctx context.Context, code, action string)
}

// lifecycleService 量表生命周期服务实现
// 行为者：量表设计者/管理员
type lifecycleService struct {
	repo                    lifecycleRepository
	questionnaireCatalog    questionnairecatalog.Catalog
	questionnairePublisher  QuestionnairePublisher
	lifecycle               scaledefinition.Lifecycle
	baseInfo                scaledefinition.BaseInfo
	eventPublisher          event.EventPublisher
	listCache               scalelistcache.PublishedListCache
	cacheSignalNotifier     CacheSignalNotifier
	interpretationPublisher ScalePublisher
	assessmentPublisher     AssessmentSnapshotPublisher
	modelRepo               modelcatalogport.ModelRepository
	publishedRepo           modelcatalogport.PublishedModelRepository
	publisher               publication.Publisher
}

type lifecycleRepository interface {
	Create(ctx context.Context, scale *scaledefinition.MedicalScale) error
	CreatePublishedSnapshot(ctx context.Context, scale *scaledefinition.MedicalScale, active bool) error
	FindByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error)
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scaledefinition.MedicalScale, error)
	Update(ctx context.Context, scale *scaledefinition.MedicalScale) error
	SetActivePublishedVersion(ctx context.Context, code, scaleVersion string) error
	ClearActivePublishedVersion(ctx context.Context, code string) error
	Remove(ctx context.Context, code string) error
}

// ScalePublisher syncs published scales into published_assessment_models.
type ScalePublisher interface {
	PublishPublishedScale(ctx context.Context, scale *scaledefinition.MedicalScale) error
}

// AssessmentSnapshotPublisher syncs published scales through the AssessmentModel snapshot path.
type AssessmentSnapshotPublisher interface {
	PublishAssessmentSnapshot(ctx context.Context, scale *scaledefinition.MedicalScale) error
}

// QuestionnairePublisher 是nar行 port 供 scale 发布 到。
// publish draft 问卷 和 get its activated version。
type QuestionnairePublisher interface {
	PublishQuestionnaire(ctx context.Context, code string) (string, error)
}

// ServiceOption 配置lifecycle service 协作者。
type ServiceOption func(*lifecycleService)

// WithQuestionnairePublisher 注入问卷 lifecycle service 使用。
// 到 publish draft 问卷 在之前 scale 快照 是 activated。
func WithQuestionnairePublisher(publisher QuestionnairePublisher) ServiceOption {
	return func(s *lifecycleService) {
		s.questionnairePublisher = publisher
	}
}

// WithCacheSignalNotifier 注入best-effort 缓存 in校验 notifier。
func WithCacheSignalNotifier(notifier CacheSignalNotifier) ServiceOption {
	return func(s *lifecycleService) {
		s.cacheSignalNotifier = notifier
	}
}

// WithScalePublisher injects the published-model sync port for scale publish.
func WithScalePublisher(publisher ScalePublisher) ServiceOption {
	return func(s *lifecycleService) {
		s.interpretationPublisher = publisher
	}
}

// WithAssessmentSnapshotPublisher injects the v2 AssessmentModel publication bridge for scale publish.
func WithAssessmentSnapshotPublisher(publisher AssessmentSnapshotPublisher) ServiceOption {
	return func(s *lifecycleService) {
		s.assessmentPublisher = publisher
	}
}

// WithAssessmentModelRepository injects the target AssessmentModel draft repository.
func WithAssessmentModelRepository(repo modelcatalogport.ModelRepository) ServiceOption {
	return func(s *lifecycleService) {
		s.modelRepo = repo
	}
}

// WithPublishedModelRepository injects the published AssessmentModel repository.
func WithPublishedModelRepository(repo modelcatalogport.PublishedModelRepository) ServiceOption {
	return func(s *lifecycleService) {
		s.publishedRepo = repo
	}
}

// WithPublicationPublisher injects the AssessmentModel publication coordinator.
func WithPublicationPublisher(publisher publication.Publisher) ServiceOption {
	return func(s *lifecycleService) {
		s.publisher = publisher
	}
}

// NewService 创建量表生命周期应用服务。
func NewService(
	repo lifecycleRepository,
	questionnaireCatalog questionnairecatalog.Catalog,
	eventPublisher event.EventPublisher,
	listCache scalelistcache.PublishedListCache,
	opts ...ServiceOption,
) ports.ScaleLifecycleService {
	service := &lifecycleService{
		repo:                 repo,
		questionnaireCatalog: questionnaireCatalog,
		lifecycle:            scaledefinition.NewLifecycle(),
		baseInfo:             scaledefinition.BaseInfo{},
		eventPublisher:       eventPublisher,
		listCache:            listCache,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// Publish 发布量表
func (s *lifecycleService) Publish(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if s.usesAssessmentModelPublishStore() {
		return s.publishAssessmentModel(ctx, code)
	}

	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if m.IsPublished() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已发布，不能重复发布")
	}

	if err := s.ensureBoundQuestionnairePublished(ctx, code, m); err != nil {
		return nil, err
	}

	if err := s.lifecycle.Publish(ctx, m); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "发布量表失败")
	}
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表状态失败")
	}
	if err := s.repo.CreatePublishedSnapshot(ctx, m, true); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表发布快照失败")
	}
	if err := s.repo.SetActivePublishedVersion(ctx, code, m.GetScaleVersion()); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "切换量表发布快照失败")
	}
	if err := s.syncInterpretationRules(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "同步解释模型规则失败")
	}

	s.publishEvents(ctx, m)
	s.refreshListCache(ctx)
	s.notifyCacheChanged(ctx, code, "published")

	return shared.ToScaleResult(m), nil
}

// Unpublish 下架量表
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if s.usesAssessmentModelPublishStore() {
		return s.unpublishAssessmentModel(ctx, code)
	}

	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	result, err := s.executeLifecycleOperation(ctx, m, func(ctx context.Context, med *scaledefinition.MedicalScale) error {
		return s.lifecycle.Unpublish(ctx, med)
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.ClearActivePublishedVersion(ctx, code); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "清空量表发布快照失败")
	}
	s.refreshListCache(ctx)
	return result, nil
}

// Archive 归档量表
func (s *lifecycleService) Archive(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if s.usesAssessmentModelPublishStore() {
		return s.archiveAssessmentModel(ctx, code)
	}

	m, err := s.getScaleByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	result, err := s.executeLifecycleOperation(ctx, m, func(ctx context.Context, med *scaledefinition.MedicalScale) error {
		return s.lifecycle.Archive(ctx, med)
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.ClearActivePublishedVersion(ctx, code); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "清空量表发布快照失败")
	}
	s.refreshListCache(ctx)
	return result, nil
}

func (s *lifecycleService) ensureHeadEditable(ctx context.Context, m *scaledefinition.MedicalScale) error {
	return editable.EnsureHeadEditable(ctx, s.repo, m)
}

func (s *lifecycleService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	shared.LogScaleListCacheError(ctx, s.listCache.Rebuild(ctx))
}

func (s *lifecycleService) generateScaleCode(code string) (meta.Code, error) {
	if code != "" {
		return meta.NewCode(code), nil
	}
	return meta.GenerateCode()
}

func (s *lifecycleService) getScaleByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error) {
	m, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	return m, nil
}

type lifecycleOperation func(ctx context.Context, med *scaledefinition.MedicalScale) error

func (s *lifecycleService) executeLifecycleOperation(
	ctx context.Context,
	m *scaledefinition.MedicalScale,
	operation lifecycleOperation,
) (*shared.ScaleResult, error) {
	if err := operation(ctx, m); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	}

	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表状态失败")
	}

	s.publishEvents(ctx, m)
	s.refreshListCache(ctx)

	return shared.ToScaleResult(m), nil
}

func (s *lifecycleService) publishEvents(ctx context.Context, m *scaledefinition.MedicalScale) {
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, m, nil, nil)
}

func (s *lifecycleService) notifyCacheChanged(ctx context.Context, code, action string) {
	if s == nil || s.cacheSignalNotifier == nil || code == "" {
		return
	}
	s.cacheSignalNotifier.NotifyScaleCacheChanged(ctx, code, action)
}

func (s *lifecycleService) syncInterpretationRules(ctx context.Context, m *scaledefinition.MedicalScale) error {
	if s != nil && s.assessmentPublisher != nil {
		return s.assessmentPublisher.PublishAssessmentSnapshot(ctx, m)
	}
	if s == nil || s.interpretationPublisher == nil {
		return nil
	}
	return s.interpretationPublisher.PublishPublishedScale(ctx, m)
}
