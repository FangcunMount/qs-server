package container

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/messaging"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	wechatPort "github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi/port"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"

	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
)

// Container 主容器
// 组合所有业务模块和基础设施组件
type Container struct {
	// 基础设施
	mysqlDB                    *gorm.DB
	mongoDB                    *mongo.Database
	redisCache                 redis.UniversalClient
	cacheOptions               ContainerCacheOptions
	cache                      *cachebootstrap.Subsystem
	planEntryURL               string
	statisticsRepairWindowDays int

	// 消息队列（可选）
	mqPublisher messaging.Publisher

	// 事件发布器（统一管理）
	eventPublisher event.EventPublisher
	publisherMode  eventconfig.PublishMode

	// 业务模块
	SurveyModule     *assembler.SurveyModule     // Survey 模块（包含问卷和答卷子模块）
	ScaleModule      *assembler.ScaleModule      // Scale 模块
	ActorModule      *assembler.ActorModule      // Actor 模块
	EvaluationModule *assembler.EvaluationModule // Evaluation 模块（测评、得分、报告）
	PlanModule       *assembler.PlanModule       // Plan 模块（测评计划）
	StatisticsModule *assembler.StatisticsModule // Statistics 模块（统计）
	IAMModule        *IAMModule                  // IAM 集成模块
	CodesService     codesapp.CodesService       // CodesService 应用服务（code 申请）

	// 基础设施服务
	QRCodeGenerator       wechatPort.QRCodeGenerator            // 小程序码生成器（可选）
	SubscribeSender       wechatPort.MiniProgramSubscribeSender // 小程序订阅消息发送器（可选）
	QRCodeObjectStore     objectstorageport.PublicObjectStore   // 二维码对象存储（可选）
	QRCodeObjectKeyPrefix string                                // 二维码对象 key 前缀

	// 应用层服务
	QRCodeService                      qrcodeApp.QRCodeService                            // 小程序码生成服务（可选）
	MiniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService // 小程序 task 消息服务（可选）

	// 容器状态
	initialized bool
	silent      bool
	modules     map[string]assembler.Module
	moduleOrder []string
}

// NewContainer 创建容器
func NewContainer(mysqlDB *gorm.DB, mongoDB *mongo.Database, redisCache redis.UniversalClient) *Container {
	return &Container{
		mysqlDB:       mysqlDB,
		mongoDB:       mongoDB,
		redisCache:    redisCache,
		publisherMode: eventconfig.PublishModeLogging, // 默认使用日志模式
		cacheOptions:  ContainerCacheOptions{},
		initialized:   false,
		modules:       make(map[string]assembler.Module),
	}
}

// ContainerOptions 容器配置选项
type ContainerOptions struct {
	// MQPublisher 消息队列发布器（可选，传入则启用 MQ 模式）
	MQPublisher messaging.Publisher
	// PublisherMode 事件发布器模式（mq, logging, nop）
	PublisherMode eventconfig.PublishMode
	// Env 环境名称（prod, dev, test），用于自动选择发布器模式
	Env string
	// Cache 缓存控制选项
	Cache ContainerCacheOptions
	// CacheSubsystem cache 子系统组合根。
	CacheSubsystem *cachebootstrap.Subsystem
	// PlanEntryBaseURL 测评计划任务入口基础地址
	PlanEntryBaseURL string
	// StatisticsRepairWindowDays 统计夜间批处理默认回补窗口
	StatisticsRepairWindowDays int
	// Silent suppresses container stdout bootstrap/cleanup prints.
	Silent bool
}

type ContainerCacheOptions = cachebootstrap.CacheOptions

type ContainerWarmupOptions = cachebootstrap.WarmupOptions

// ContainerCacheFamilyOptions 定义单个缓存 family 的对象级策略。
type ContainerCacheFamilyOptions = cachebootstrap.CacheFamilyOptions

func (c *Container) registerModule(name string, module assembler.Module) {
	if c == nil || name == "" || module == nil {
		return
	}
	if c.modules == nil {
		c.modules = make(map[string]assembler.Module)
	}
	if _, exists := c.modules[name]; !exists {
		c.moduleOrder = append(c.moduleOrder, name)
	}
	c.modules[name] = module
}

func (c *Container) loadedModules() []assembler.Module {
	if c == nil || len(c.moduleOrder) == 0 {
		return nil
	}
	modules := make([]assembler.Module, 0, len(c.moduleOrder))
	for _, name := range c.moduleOrder {
		if module := c.modules[name]; module != nil {
			modules = append(modules, module)
		}
	}
	return modules
}

// ContainerCacheTTLOptions 缓存 TTL 配置（0 表示使用默认值）
type ContainerCacheTTLOptions = cachebootstrap.CacheTTLOptions

// NewContainerWithOptions 创建带配置的容器
func NewContainerWithOptions(mysqlDB *gorm.DB, mongoDB *mongo.Database, redisCache redis.UniversalClient, opts ContainerOptions) *Container {
	c := NewContainer(mysqlDB, mongoDB, redisCache)
	c.mqPublisher = opts.MQPublisher

	// 根据环境或显式配置确定发布器模式
	if opts.PublisherMode != "" {
		c.publisherMode = opts.PublisherMode
	} else if opts.Env != "" {
		c.publisherMode = eventconfig.PublishModeFromEnv(opts.Env)
	}

	c.cacheOptions = opts.Cache
	c.cache = opts.CacheSubsystem
	c.planEntryURL = opts.PlanEntryBaseURL
	c.statisticsRepairWindowDays = opts.StatisticsRepairWindowDays
	c.silent = opts.Silent

	return c
}

// Initialize 初始化容器
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	// 加载事件配置（发布器依赖此配置进行路由）
	if err := eventconfig.Initialize("configs/events.yaml"); err != nil {
		return fmt.Errorf("failed to load event config: %w", err)
	}
	c.printf("📋 Event config loaded (events.yaml)\n")

	// 初始化事件发布器（所有模块共享）
	c.initEventPublisher()
	c.printf("📡 Event publisher initialized (mode=%s)\n", c.publisherMode)

	// 初始化 IAM 模块（优先，因为其他模块可能依赖）
	// 注意：这里需要传入 IAMOptions，在实际调用时需要从外部传入
	// 暂时留空，在 InitializeWithOptions 方法中初始化

	// 初始化 Survey 模块（包含问卷和答卷子模块）
	if err := c.initSurveyModule(); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	// 初始化 Scale 模块
	if err := c.initScaleModule(); err != nil {
		return fmt.Errorf("failed to initialize scale module: %w", err)
	}
	c.wireSurveyScaleDependencies()

	// 初始化 Actor 模块
	if err := c.initActorModule(); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	// 初始化 Evaluation 模块
	if err := c.initEvaluationModule(); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}

	// 将评估服务注入到 Actor 模块（因为 Actor 模块在 Evaluation 模块之前初始化）
	c.wireActorEvaluationDependencies()

	// 初始化 Plan 模块
	if err := c.initPlanModule(); err != nil {
		return fmt.Errorf("failed to initialize plan module: %w", err)
	}

	// 初始化 Statistics 模块
	if err := c.initStatisticsModule(); err != nil {
		return fmt.Errorf("failed to initialize statistics module: %w", err)
	}
	if err := c.initWarmupCoordinator(); err != nil {
		return fmt.Errorf("failed to initialize cache governance warmup coordinator: %w", err)
	}

	c.wireProtectedScopeDependencies()

	// 初始化 CodesService
	c.initCodesService()

	// 初始化小程序码生成器（基础设施层）
	c.initQRCodeGenerator()

	c.initialized = true
	c.printf("🏗️  Container initialized successfully\n")

	return nil
}

func (c *Container) printf(format string, args ...interface{}) {
	if c != nil && c.silent {
		return
	}
	fmt.Printf(format, args...)
}

// initEventPublisher 初始化事件发布器
func (c *Container) initEventPublisher() {
	c.eventPublisher = eventconfig.NewRoutingPublisher(eventconfig.RoutingPublisherOptions{
		Mode:        c.publisherMode,
		Source:      event.SourceAPIServer,
		MQPublisher: c.mqPublisher,
	})
}

// GetEventPublisher 获取事件发布器（供模块使用）
func (c *Container) GetEventPublisher() event.EventPublisher {
	if c.eventPublisher == nil {
		// 如果未初始化，返回空实现
		return event.NewNopEventPublisher()
	}
	return c.eventPublisher
}
