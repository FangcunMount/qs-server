package scale

import (
	"context"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/lifecycle"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/query"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	domscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// NewLifecycleService 创建量表生命周期应用服务。
func NewLifecycleService(
	repo domscale.Repository,
	questionnaireCatalog questionnairecatalog.Catalog,
	eventPublisher event.EventPublisher,
	listCache scalelistcache.PublishedListCache,
	questionnairePublishers ...lifecycle.QuestionnairePublisher,
) ScaleLifecycleService {
	var questionnairePublisher lifecycle.QuestionnairePublisher
	if len(questionnairePublishers) > 0 {
		questionnairePublisher = questionnairePublishers[0]
	}
	return lifecycle.NewService(repo, questionnaireCatalog, eventPublisher, listCache, lifecycle.WithQuestionnairePublisher(questionnairePublisher))
}

// QuestionnairePublisherFunc adapts a function to the scale lifecycle's
// questionnaire publication port.
type QuestionnairePublisherFunc func(ctx context.Context, code string) (string, error)

// PublishQuestionnaire implements lifecycle.QuestionnairePublisher.
func (f QuestionnairePublisherFunc) PublishQuestionnaire(ctx context.Context, code string) (string, error) {
	return f(ctx, code)
}

// NewFactorService 创建量表因子编辑应用服务。
func NewFactorService(repo domscale.Repository, listCache scalelistcache.PublishedListCache, eventPublisher event.EventPublisher) ScaleFactorService {
	return factor.NewService(repo, listCache, eventPublisher)
}

// NewQueryService 创建量表查询应用服务。
func NewQueryService(repo domscale.Repository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...domscale.ScaleHotRankReadModel) ScaleQueryService {
	return query.NewQueryService(repo, reader, identitySvc, listCache, hotset, hotRankReaders...)
}

// NewQueryServiceWithReadModel 创建使用显式 read model 的量表查询服务。
func NewQueryServiceWithReadModel(repo domscale.Repository, reader scalereadmodel.ScaleReader, identitySvc iambridge.IdentityResolver, listCache scalelistcache.PublishedListCache, hotset cachetarget.HotsetRecorder, hotRankReaders ...domscale.ScaleHotRankReadModel) ScaleQueryService {
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
func NewQuestionnaireBindingSyncer(repo domscale.Repository) *QuestionnaireBindingSyncer {
	return lifecycle.NewQuestionnaireBindingSyncer(repo)
}

// NewScaleHotRankProjectionHook 注册热门量表投影钩子。
func NewScaleHotRankProjectionHook(projection domscale.ScaleHotRankProjection) appEventing.OutboxBeforePublishHook {
	return query.NewScaleHotRankProjectionHook(projection)
}
