package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
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

type scaleModuleDeps struct {
	mongoDB           *mongo.Database
	eventPublisher    event.EventPublisher
	questionnaireRepo domainQuestionnaire.Repository
	redisClient       redis.UniversalClient
	cacheBuilder      *rediskey.Builder
	identityService   *iam.IdentityService
	scalePolicy       cachepolicy.CachePolicy
	scaleListPolicy   cachepolicy.CachePolicy
	hotsetRecorder    scaleCache.HotsetRecorder
	observer          *scaleCache.Observer
}

// NewScaleModule 创建 Scale 模块
func NewScaleModule() *ScaleModule {
	return &ScaleModule{}
}

// Initialize 初始化 Scale 模块
// params[0]: *mongo.Database
// params[1]: event.EventPublisher (可选，默认使用 NopEventPublisher)
// params[2]: questionnaire.Repository (可选，用于自动获取问卷版本)
// params[3]: redis.UniversalClient (可选，用于量表缓存装饰器与列表缓存)
// params[4]: *rediskey.Builder (可选，用于静态缓存 key builder)
// params[5]: *iam.IdentityService (可选，用于姓名补全)
// params[6]: cachepolicy.CachePolicy (可选，用于量表详情缓存策略)
// params[7]: cachepolicy.CachePolicy (可选，用于量表列表缓存策略)
// params[8]: scaleCache.HotsetRecorder (可选，用于热点治理)
func (m *ScaleModule) Initialize(params ...interface{}) error {
	deps, err := parseScaleModuleDeps(params)
	if err != nil {
		return err
	}
	m.eventPublisher = deps.eventPublisher

	// 初始化 repository 层（基础实现）
	baseRepo := scaleInfra.NewRepository(deps.mongoDB)
	// 如果提供了 Redis 客户端，使用缓存装饰器
	if deps.redisClient != nil {
		m.Repo = scaleCache.NewCachedScaleRepositoryWithBuilderPolicyAndObserver(baseRepo, deps.redisClient, deps.cacheBuilder, deps.scalePolicy, deps.observer)
	} else {
		m.Repo = baseRepo
	}

	// 初始化量表全局列表缓存
	var listCache *scaleApp.ScaleListCache
	if deps.redisClient != nil {
		listCache = scaleApp.NewScaleListCacheWithPolicyAndKeyBuilder(
			deps.redisClient,
			m.Repo,
			deps.identityService,
			deps.cacheBuilder,
			deps.scaleListPolicy,
		)
	}
	m.ListCache = listCache

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	m.LifecycleService = scaleApp.NewLifecycleService(m.Repo, deps.questionnaireRepo, m.eventPublisher, listCache)
	m.FactorService = scaleApp.NewFactorService(m.Repo, listCache, m.eventPublisher)
	m.QueryService = scaleApp.NewQueryService(m.Repo, deps.identityService, listCache, deps.hotsetRecorder)
	m.CategoryService = scaleApp.NewCategoryService()

	// 初始化 handler 层
	// 注意：QRCodeService 在容器初始化后才创建，需要通过 SetQRCodeService 方法单独设置
	m.Handler = handler.NewScaleHandler(
		m.LifecycleService,
		m.FactorService,
		m.QueryService,
		m.CategoryService,
		nil, // QRCodeService 稍后通过 SetQRCodeService 设置
	)

	return nil
}

func parseScaleModuleDeps(params []interface{}) (*scaleModuleDeps, error) {
	if len(params) < 1 {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is required")
	}

	mongoDB, ok := params[0].(*mongo.Database)
	if !ok || mongoDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	deps := &scaleModuleDeps{
		mongoDB:        mongoDB,
		eventPublisher: event.NewNopEventPublisher(),
	}
	applyOptionalParam(params, 1, func(publisher event.EventPublisher) {
		if publisher != nil {
			deps.eventPublisher = publisher
		}
	})
	applyOptionalParam(params, 2, func(repo domainQuestionnaire.Repository) {
		if repo != nil {
			deps.questionnaireRepo = repo
		}
	})
	applyOptionalParam(params, 3, func(client redis.UniversalClient) {
		if client != nil {
			deps.redisClient = client
		}
	})
	applyOptionalParam(params, 4, func(builder *rediskey.Builder) {
		deps.cacheBuilder = builder
	})
	applyOptionalParam(params, 5, func(svc *iam.IdentityService) {
		deps.identityService = svc
	})
	applyOptionalParam(params, 6, func(policy cachepolicy.CachePolicy) {
		deps.scalePolicy = policy
	})
	applyOptionalParam(params, 7, func(policy cachepolicy.CachePolicy) {
		deps.scaleListPolicy = policy
	})
	applyOptionalParam(params, 8, func(recorder scaleCache.HotsetRecorder) {
		deps.hotsetRecorder = recorder
	})
	applyOptionalParam(params, 9, func(observer *scaleCache.Observer) {
		deps.observer = observer
	})
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
