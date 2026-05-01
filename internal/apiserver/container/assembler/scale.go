package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ScaleModule Scale 模块（量表子域）
// 按照 DDD 限界上下文组织
type ScaleModule struct {
	// repository 层
	Repo scale.Repository

	// handler 层
	Handler *handler.ScaleHandler

	// service 层 - 按行为者组织
	LifecycleService scaleApp.ScaleLifecycleService
	FactorService    scaleApp.ScaleFactorService
	QueryService     scaleApp.ScaleQueryService
	CategoryService  scaleApp.ScaleCategoryService
	ListCache        *scaleApp.ScaleListCache

	// 事件发布器（由容器统一注入）
	eventPublisher event.EventPublisher
}

// ScaleModuleDeps 定义 Scale 模块的显式构造依赖。
type ScaleModuleDeps struct {
	MongoDB           *mongo.Database
	EventPublisher    event.EventPublisher
	QuestionnaireRepo domainQuestionnaire.Repository
	RedisClient       redis.UniversalClient
	CacheBuilder      *keyspace.Builder
	IdentityService   *iam.IdentityService
	ScalePolicy       cachepolicy.CachePolicy
	ScaleListPolicy   cachepolicy.CachePolicy
	HotsetRecorder    cachetarget.HotsetRecorder
	Observer          *observability.ComponentObserver
	MongoLimiter      backpressure.Acquirer
}

// NewScaleModule 创建 Scale 模块。
func NewScaleModule(deps ScaleModuleDeps) (*ScaleModule, error) {
	normalized, err := normalizeScaleModuleDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &ScaleModule{}
	module.eventPublisher = normalized.EventPublisher

	// 初始化 repository 层（基础实现）
	baseRepo := scaleInfra.NewRepository(normalized.MongoDB, mongoBase.BaseRepositoryOptions{Limiter: normalized.MongoLimiter})
	// 如果提供了 Redis 客户端，使用缓存装饰器
	if normalized.RedisClient != nil {
		module.Repo = scaleCache.NewCachedScaleRepositoryWithBuilderPolicyAndObserver(baseRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.ScalePolicy, normalized.Observer)
	} else {
		module.Repo = baseRepo
	}

	// 初始化量表全局列表缓存
	var listCache *scaleApp.ScaleListCache
	if normalized.RedisClient != nil {
		listCache = scaleApp.NewScaleListCacheWithPolicyAndKeyBuilder(
			cacheentry.NewRedisCache(normalized.RedisClient),
			module.Repo,
			normalized.IdentityService,
			normalized.CacheBuilder,
			normalized.ScaleListPolicy,
		)
	}
	module.ListCache = listCache

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	module.LifecycleService = scaleApp.NewLifecycleService(module.Repo, normalized.QuestionnaireRepo, module.eventPublisher, listCache)
	module.FactorService = scaleApp.NewFactorService(module.Repo, listCache, module.eventPublisher)
	hotRankReader := scaleCache.NewRedisScaleHotRank(normalized.RedisClient, normalized.CacheBuilder)
	module.QueryService = scaleApp.NewQueryService(module.Repo, normalized.IdentityService, listCache, normalized.HotsetRecorder, hotRankReader)
	module.CategoryService = scaleApp.NewCategoryService()

	// 初始化 handler 层
	// 注意：QRCodeService 在容器初始化后才创建，需要通过 SetQRCodeService 方法单独设置
	module.Handler = handler.NewScaleHandler(
		module.LifecycleService,
		module.FactorService,
		module.QueryService,
		module.CategoryService,
		nil, // QRCodeService 稍后通过 SetQRCodeService 设置
	)

	return module, nil
}

func normalizeScaleModuleDeps(deps ScaleModuleDeps) (ScaleModuleDeps, error) {
	if deps.MongoDB == nil {
		return ScaleModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// Cleanup 清理模块资源
func (m *ScaleModule) Cleanup() error {
	return nil
}

// SetQRCodeService 设置二维码服务（用于跨模块依赖注入）
func (m *ScaleModule) SetQRCodeService(qrCodeService qrcodeApp.QRCodeService) {
	if m.Handler != nil {
		// 重新创建 Handler，传入 QRCodeService
		m.Handler = handler.NewScaleHandler(
			m.LifecycleService,
			m.FactorService,
			m.QueryService,
			m.CategoryService,
			qrCodeService,
		)
	}
}

// CheckHealth 检查模块健康状态
func (m *ScaleModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *ScaleModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "scale",
		Version:     "2.0.0",
		Description: "量表管理模块（重构版）",
	}
}
