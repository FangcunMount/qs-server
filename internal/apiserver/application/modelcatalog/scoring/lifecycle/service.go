package lifecycle

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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
	questionnaireCatalog   questionnairecatalog.Catalog
	questionnairePublisher QuestionnairePublisher
	eventPublisher         event.EventPublisher
	listCache              scalelistcache.PublishedListCache
	cacheSignalNotifier    CacheSignalNotifier
	modelRepo              modelcatalogport.ModelRepository
	publishedRepo          modelcatalogport.PublishedModelRepository
	publisher              publication.Publisher
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
	questionnaireCatalog questionnairecatalog.Catalog,
	eventPublisher event.EventPublisher,
	listCache scalelistcache.PublishedListCache,
	opts ...ServiceOption,
) ports.ScaleLifecycleService {
	service := &lifecycleService{
		questionnaireCatalog: questionnaireCatalog,
		eventPublisher:       eventPublisher,
		listCache:            listCache,
	}
	for _, opt := range opts {
		opt(service)
	}
	service.requireAuthoringStores()
	return service
}

func (s *lifecycleService) requireAuthoringStores() {
	if s == nil || s.modelRepo == nil || s.publishedRepo == nil || s.publisher.ModelRepo == nil || s.publisher.Repo == nil {
		panic("lifecycle: assessment model authoring stores are required")
	}
}

// Publish 发布量表
func (s *lifecycleService) Publish(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	return s.publishAssessmentModel(ctx, code)
}

// Unpublish 下架量表
func (s *lifecycleService) Unpublish(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	return s.unpublishAssessmentModel(ctx, code)
}

// Archive 归档量表
func (s *lifecycleService) Archive(ctx context.Context, code string) (*shared.ScaleResult, error) {
	if code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	return s.archiveAssessmentModel(ctx, code)
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

func (s *lifecycleService) notifyCacheChanged(ctx context.Context, code, action string) {
	if s == nil || s.cacheSignalNotifier == nil || code == "" {
		return
	}
	s.cacheSignalNotifier.NotifyScaleCacheChanged(ctx, code, action)
}

func (s *lifecycleService) publishScaleChangedEvent(ctx context.Context, model *domain.AssessmentModel, action scaledefinition.ChangeAction) {
	if evt, ok := assessmentstore.ScaleChangedEvent(model, action); ok {
		eventing.PublishCollectedEvents(ctx, s.eventPublisher, eventing.Collect(evt), nil, nil)
	}
}
