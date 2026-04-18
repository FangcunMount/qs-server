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

// NewScaleModule 创建 Scale 模块
func NewScaleModule() *ScaleModule {
	return &ScaleModule{}
}

// Initialize 初始化 Scale 模块
// params[0]: *mongo.Database
// params[1]: event.EventPublisher (可选，默认使用 NopEventPublisher)
// params[2]: questionnaire.Repository (可选，用于自动获取问卷版本)
// params[3]: redis.UniversalClient (可选，用于量表缓存装饰器与列表缓存)
// params[4]: string (可选，用于静态缓存 namespace)
// params[5]: *iam.IdentityService (可选，用于姓名补全)
// params[6]: scaleCache.CachePolicy (可选，用于量表详情缓存策略)
// params[7]: scaleCache.CachePolicy (可选，用于量表列表缓存策略)
// params[8]: scaleCache.HotsetRecorder (可选，用于热点治理)
func (m *ScaleModule) Initialize(params ...interface{}) error {
	if len(params) < 1 {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is required")
	}

	mongoDB, ok := params[0].(*mongo.Database)
	if !ok || mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}

	// 获取事件发布器（可选参数）
	if len(params) > 1 {
		if ep, ok := params[1].(event.EventPublisher); ok && ep != nil {
			m.eventPublisher = ep
		}
	}
	if m.eventPublisher == nil {
		m.eventPublisher = event.NewNopEventPublisher()
	}

	// 获取问卷仓库（可选参数）
	var questionnaireRepo domainQuestionnaire.Repository
	if len(params) > 2 {
		if qr, ok := params[2].(domainQuestionnaire.Repository); ok && qr != nil {
			questionnaireRepo = qr
		}
	}

	// 获取 Redis 客户端（可选参数，用于缓存装饰器）
	var redisClient redis.UniversalClient
	if len(params) > 3 {
		if rc, ok := params[3].(redis.UniversalClient); ok && rc != nil {
			redisClient = rc
		}
	}
	var cacheNamespace string
	if len(params) > 4 {
		if ns, ok := params[4].(string); ok {
			cacheNamespace = ns
		}
	}
	// 获取 IAM IdentityService（可选参数，用于姓名补全）
	var identitySvc *iam.IdentityService
	if len(params) > 5 {
		if svc, ok := params[5].(*iam.IdentityService); ok {
			identitySvc = svc
		}
	}
	var scalePolicy scaleCache.CachePolicy
	if len(params) > 6 {
		if policy, ok := params[6].(scaleCache.CachePolicy); ok {
			scalePolicy = policy
		}
	}
	var scaleListPolicy scaleCache.CachePolicy
	if len(params) > 7 {
		if policy, ok := params[7].(scaleCache.CachePolicy); ok {
			scaleListPolicy = policy
		}
	}
	var hotset scaleCache.HotsetRecorder
	if len(params) > 8 {
		if recorder, ok := params[8].(scaleCache.HotsetRecorder); ok {
			hotset = recorder
		}
	}

	// 初始化 repository 层（基础实现）
	baseRepo := scaleInfra.NewRepository(mongoDB)
	cacheBuilder := rediskey.NewBuilderWithNamespace(cacheNamespace)

	// 如果提供了 Redis 客户端，使用缓存装饰器
	if redisClient != nil {
		m.Repo = scaleCache.NewCachedScaleRepositoryWithBuilderAndPolicy(baseRepo, redisClient, cacheBuilder, scalePolicy)
	} else {
		m.Repo = baseRepo
	}

	// 初始化量表全局列表缓存
	var listCache *scaleApp.ScaleListCache
	if redisClient != nil {
		listCache = scaleApp.NewScaleListCacheWithPolicyAndKeyBuilder(
			redisClient,
			m.Repo,
			identitySvc,
			scaleCache.NewCacheKeyBuilderWithNamespace(cacheNamespace),
			scaleListPolicy,
		)
	}
	m.ListCache = listCache

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	m.LifecycleService = scaleApp.NewLifecycleService(m.Repo, questionnaireRepo, m.eventPublisher, listCache)
	m.FactorService = scaleApp.NewFactorService(m.Repo, listCache, m.eventPublisher)
	m.QueryService = scaleApp.NewQueryService(m.Repo, identitySvc, listCache, hotset)
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
