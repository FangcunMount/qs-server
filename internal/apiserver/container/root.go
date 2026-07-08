package container

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/messaging"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"

	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
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
	backpressure               BackpressureOptions
	outboxRelay                ContainerOutboxRelayOptions
	planEntryURL               string
	statisticsRepairWindowDays int
	behaviorFootprintDisabled  []string
	reportStatusConfig         reportstatus.Config
	systemGovernanceOptions    *apiserveroptions.SystemGovernanceOptions
	cacheSignalNotifier        *cachesignal.Notifier

	// 消息队列（可选）
	mqPublisher messaging.Publisher

	// 事件发布器（统一管理）
	eventPublisher event.EventPublisher
	eventCatalog   *eventcatalog.Catalog
	publisherMode  eventruntime.PublishMode

	// 业务模块
	SurveyModule          *SurveyModule          // Survey 模块（包含问卷和答卷子模块）
	AssessmentModelModule *AssessmentModelModule // 测评解释模型资产（量表 + 人格模型目录）
	ScaleModule           *ScaleModule           // Scale 能力别名（AssessmentModelModule.Scale）
	TypologyModelModule   *TypologyModelModule   // Typology 能力别名（AssessmentModelModule.Typology）
	ActorModule           *ActorModule           // Actor 模块
	EvaluationModule      *EvaluationModule      // Evaluation 模块（测评、得分、报告）
	ReportModule          *ReportModule          // Report 模块（报告读模型与 builder registry）
	PlanModule            *PlanModule            // Plan 模块（测评计划）
	StatisticsModule      *StatisticsModule      // Statistics 模块（统计）
	IAMModule             *IAMModule             // IAM 集成模块
	CodesService          codesapp.CodesService  // CodesService 应用服务（code 申请）

	ruleSetCatalog rulesetport.RuleSetCatalog

	// Survey/Scale 基础设施由容器持有，业务模块只暴露应用服务。
	surveyScaleInfra *surveymod.ScaleInfra

	// 基础设施服务
	QRCodeGenerator       wechatmini.QRCodeGenerator            // 小程序码生成器（可选）
	SubscribeSender       wechatmini.MiniProgramSubscribeSender // 小程序订阅消息发送器（可选）
	QRCodeObjectStore     objectstorageport.PublicObjectStore   // 二维码对象存储（可选）
	QRCodeObjectKeyPrefix string                                // 二维码对象 key 前缀

	// 应用层服务
	QRCodeService                      qrcodeApp.QRCodeService                            // 小程序码生成服务（可选）
	MiniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService // 小程序 task 消息服务（可选）

	// 容器状态
	initialized bool
	silent      bool
	modules     map[string]Module
	moduleOrder []string
}

// NewContainer 创建容器
func NewContainer(mysqlDB *gorm.DB, mongoDB *mongo.Database, redisCache redis.UniversalClient) *Container {
	return &Container{
		mysqlDB:       mysqlDB,
		mongoDB:       mongoDB,
		redisCache:    redisCache,
		publisherMode: eventruntime.PublishModeLogging, // 默认使用日志模式
		cacheOptions:  ContainerCacheOptions{},
		initialized:   false,
		modules:       make(map[string]Module),
	}
}

func (c *Container) registerModule(name string, module Module) {
	if c == nil || name == "" || module == nil {
		return
	}
	if c.modules == nil {
		c.modules = make(map[string]Module)
	}
	if _, exists := c.modules[name]; !exists {
		c.moduleOrder = append(c.moduleOrder, name)
	}
	c.modules[name] = module
}

func (c *Container) loadedModules() []Module {
	if c == nil || len(c.moduleOrder) == 0 {
		return nil
	}
	modules := make([]Module, 0, len(c.moduleOrder))
	for _, name := range c.moduleOrder {
		if module := c.modules[name]; module != nil {
			modules = append(modules, module)
		}
	}
	return modules
}

// NewContainerWithOptions 创建带配置的容器
func NewContainerWithOptions(mysqlDB *gorm.DB, mongoDB *mongo.Database, redisCache redis.UniversalClient, opts ContainerOptions) *Container {
	c := NewContainer(mysqlDB, mongoDB, redisCache)
	c.mqPublisher = opts.MQPublisher

	// 根据环境或显式配置确定发布器模式
	if opts.PublisherMode != "" {
		c.publisherMode = opts.PublisherMode
	} else if opts.Env != "" {
		c.publisherMode = eventruntime.PublishModeFromEnv(opts.Env)
	}

	c.eventCatalog = opts.EventCatalog
	c.cacheOptions = opts.Cache
	c.cache = opts.CacheSubsystem
	c.backpressure = opts.Backpressure
	c.outboxRelay = opts.OutboxRelay
	c.planEntryURL = opts.PlanEntryBaseURL
	c.statisticsRepairWindowDays = opts.StatisticsRepairWindowDays
	c.behaviorFootprintDisabled = opts.BehaviorFootprintDisableDurableEventTypes
	c.reportStatusConfig = reportstatus.ConfigFromOptions(opts.ReportStatus, opts.Signaling, "apiserver")
	c.systemGovernanceOptions = opts.SystemGovernance
	c.silent = opts.Silent

	return c
}

// Initialize 初始化容器
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	// 确保 cache singleflight coordinator 初始化
	ensureCacheSingleflightCoordinator()

	// 初始化事件发布器（所有模块共享）
	c.initEventPublisher()
	statisticsApp.InstallFootprintDurableStagingPolicy(
		statisticsApp.NewFootprintDurableStagingPolicy(c.behaviorFootprintDisabled),
	)
	c.printf("📡 Event publisher initialized (mode=%s)\n", c.publisherMode)

	if err := c.initCacheSignalNotifier(); err != nil {
		return fmt.Errorf("failed to initialize cache signal notifier: %w", err)
	}

	// 初始化 IAM 模块（优先，因为其他模块可能依赖）
	// 注意：这里需要传入 IAMOptions，在实际调用时需要从外部传入
	// 暂时留空，在 InitializeWithOptions 方法中初始化

	// 初始化 Survey 模块（包含问卷和答卷子模块）
	if err := c.initSurveyModule(); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	// 初始化 Assessment model 模块（scale + personality catalog）
	if err := c.initModelCatalogModule(); err != nil {
		return fmt.Errorf("failed to initialize assessment model module: %w", err)
	}

	// 初始化 Actor 模块
	if err := c.initActorModule(); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	// 初始化 Report 模块
	if err := c.initReportModule(); err != nil {
		return fmt.Errorf("failed to initialize report module: %w", err)
	}

	// 初始化 Evaluation 模块
	if err := c.initEvaluationModule(); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}

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

	// 初始化 CodesService 与小程序码基础设施
	c.initPlatformModule()

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
	c.eventPublisher = eventruntime.NewRoutingPublisher(eventruntime.RoutingPublisherOptions{
		Mode:        c.publisherMode,
		Source:      event.SourceAPIServer,
		MQPublisher: c.mqPublisher,
		Catalog:     c.eventCatalog,
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
