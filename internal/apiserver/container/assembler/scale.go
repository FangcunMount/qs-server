package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
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
// params[3]: redis.UniversalClient (可选，用于缓存装饰器)
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

	// 初始化 repository 层（基础实现）
	baseRepo := scaleInfra.NewRepository(mongoDB)

	// 如果提供了 Redis 客户端，使用缓存装饰器
	if redisClient != nil {
		m.Repo = scaleCache.NewCachedScaleRepository(baseRepo, redisClient)
	} else {
		m.Repo = baseRepo
	}

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	m.LifecycleService = scaleApp.NewLifecycleService(m.Repo, questionnaireRepo, m.eventPublisher)
	m.FactorService = scaleApp.NewFactorService(m.Repo)
	m.QueryService = scaleApp.NewQueryService(m.Repo)
	m.CategoryService = scaleApp.NewCategoryService()

	// 初始化 handler 层
	m.Handler = handler.NewScaleHandler(
		m.LifecycleService,
		m.FactorService,
		m.QueryService,
		m.CategoryService,
	)

	return nil
}

// Cleanup 清理模块资源
func (m *ScaleModule) Cleanup() error {
	return nil
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
