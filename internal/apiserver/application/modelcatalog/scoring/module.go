package scale

import (
	"context"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/lifecycle"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/query"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition/hotrank"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// NewLifecycleService 创建量表生命周期应用服务。
func NewLifecycleService(
	repo scaledefinition.Repository,
	questionnaireCatalog questionnairecatalog.Catalog,
	eventPublisher event.EventPublisher,
	listCache scalelistcache.PublishedListCache,
	opts ...lifecycle.ServiceOption,
) ScaleLifecycleService {
	serviceOpts := make([]lifecycle.ServiceOption, 0, len(opts))
	serviceOpts = append(serviceOpts, opts...)
	return lifecycle.NewService(repo, questionnaireCatalog, eventPublisher, listCache, serviceOpts...)
}

// WithQuestionnairePublisher 注入问卷 发布器 用于 scale 发布。
func WithQuestionnairePublisher(publisher lifecycle.QuestionnairePublisher) lifecycle.ServiceOption {
	return lifecycle.WithQuestionnairePublisher(publisher)
}

// WithCacheSignalNotifier 注入best-effort 缓存 in校验 notifier。
func WithCacheSignalNotifier(notifier lifecycle.CacheSignalNotifier) lifecycle.ServiceOption {
	return lifecycle.WithCacheSignalNotifier(notifier)
}

// WithScalePublisher injects the published-model sync port for scale publish.
func WithScalePublisher(publisher lifecycle.ScalePublisher) lifecycle.ServiceOption {
	return lifecycle.WithScalePublisher(publisher)
}

// WithAssessmentSnapshotPublisher injects the AssessmentModel snapshot publish bridge for scale publish.
func WithAssessmentSnapshotPublisher(publisher lifecycle.AssessmentSnapshotPublisher) lifecycle.ServiceOption {
	return lifecycle.WithAssessmentSnapshotPublisher(publisher)
}

// WithAssessmentModelRepository injects the AssessmentModel draft repository for scale authoring.
func WithAssessmentModelRepository(repo modelcatalogport.ModelRepository) lifecycle.ServiceOption {
	return lifecycle.WithAssessmentModelRepository(repo)
}

// WithPublishedModelRepository injects the published AssessmentModel repository for scale publish flows.
func WithPublishedModelRepository(repo modelcatalogport.PublishedModelRepository) lifecycle.ServiceOption {
	return lifecycle.WithPublishedModelRepository(repo)
}

// WithPublicationPublisher injects the AssessmentModel publication coordinator.
func WithPublicationPublisher(publisher publication.Publisher) lifecycle.ServiceOption {
	return lifecycle.WithPublicationPublisher(publisher)
}

// NewScalePublicationPublisher builds the scale publication coordinator.
func NewScalePublicationPublisher(modelRepo modelcatalogport.ModelRepository, publishedRepo modelcatalogport.PublishedModelRepository) publication.Publisher {
	return assessmentstore.NewPublicationPublisher(modelRepo, publishedRepo)
}

// NewAssessmentSnapshotPublisher creates the scale legacy-to-AssessmentModel publish bridge.
func NewAssessmentSnapshotPublisher(modelRepo modelcatalogport.ModelRepository, publishedRepo modelcatalogport.PublishedModelRepository) lifecycle.AssessmentSnapshotPublisher {
	return legacyadapter.NewAssessmentSnapshotPublisher(modelRepo, publishedRepo)
}

// QuestionnairePublisherFunc 适配函数 到 scale lifecycle's。
// 问卷 发布 port。
type QuestionnairePublisherFunc func(ctx context.Context, code string) (string, error)

// PublishQuestionnaire implements lifecycle.问卷发布器。
func (f QuestionnairePublisherFunc) PublishQuestionnaire(ctx context.Context, code string) (string, error) {
	return f(ctx, code)
}

// NewFactorService 创建量表因子编辑应用服务。
func NewFactorService(repo scaledefinition.Repository, listCache scalelistcache.PublishedListCache, eventPublisher event.EventPublisher, opts ...factor.ServiceOption) ScaleFactorService {
	return factor.NewService(repo, listCache, eventPublisher, opts...)
}

// WithFactorAssessmentModelRepository injects the AssessmentModel draft repository for scale factor authoring.
func WithFactorAssessmentModelRepository(repo modelcatalogport.ModelRepository) factor.ServiceOption {
	return factor.WithAssessmentModelRepository(repo)
}

// NewQueryService 创建量表查询应用服务。
func NewQueryService(repo scaledefinition.Repository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...hotrank.ReadModel) ScaleQueryService {
	return query.NewQueryService(repo, reader, identitySvc, listCache, hotset, hotRankReaders...)
}

// NewQueryServiceWithHotListCache 创建带热门量表列表缓存的查询服务。
func NewQueryServiceWithHotListCache(
	repo scaledefinition.Repository,
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	hotRankReaders ...hotrank.ReadModel,
) ScaleQueryService {
	return query.NewQueryServiceWithHotListCache(repo, reader, identitySvc, listCache, hotListCache, hotset, hotRankReaders...)
}

func NewQueryServiceWithModelCatalogSources(
	repo scaledefinition.Repository,
	reader scalereadmodel.ScaleReader,
	identitySvc iambridge.IdentityResolver,
	listCache scalelistcache.PublishedListCache,
	hotListCache scalelistcache.HotListCache,
	hotset cachetarget.HotsetRecorder,
	sources query.ModelCatalogSources,
	hotRankReaders ...hotrank.ReadModel,
) ScaleQueryService {
	return query.NewQueryServiceWithModelCatalogSources(repo, reader, identitySvc, listCache, hotListCache, hotset, sources, hotRankReaders...)
}

// NewQueryServiceWithReadModel 创建使用显式 read model 的量表查询服务。
func NewQueryServiceWithReadModel(repo scaledefinition.Repository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...hotrank.ReadModel) ScaleQueryService {
	return query.NewQueryServiceWithReadModel(repo, reader, identitySvc, listCache, hotset, hotRankReaders...)
}

// NewCategoryService 创建量表分类选项服务。
func NewCategoryService() ScaleCategoryService {
	return query.NewCategoryService()
}

// NewQRCodeQueryService 创建量表小程序码查询服务。
func NewQRCodeQueryService(generator ScaleQRCodeGenerator) ScaleQRCodeQueryService {
	return query.NewQRCodeQueryService(generator)
}

// NewQuestionnaireBindingSyncer 创建问卷发布后同步量表绑定版本的服务。
func NewQuestionnaireBindingSyncer(repo scaledefinition.Repository) *QuestionnaireBindingSyncer {
	return lifecycle.NewQuestionnaireBindingSyncer(repo)
}

// NewScaleHotRankProjectionHook 注册热门量表投影钩子。
func NewScaleHotRankProjectionHook(projection hotrank.Projection) appEventing.OutboxBeforePublishHook {
	return query.NewScaleHotRankProjectionHook(projection)
}
