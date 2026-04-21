package container

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	redis "github.com/redis/go-redis/v9"
	"github.com/silenceper/wechat/v2/cache"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi"
	wechatPort "github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi/port"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/pkg/event"

	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
)

// modulePool 模块池
var modulePool = make(map[string]assembler.Module)

// Container 主容器
// 组合所有业务模块和基础设施组件
type Container struct {
	// 基础设施
	mysqlDB                      *gorm.DB
	mongoDB                      *mongo.Database
	redisCache                   redis.UniversalClient
	staticRedisCache             redis.UniversalClient
	objectRedisCache             redis.UniversalClient
	queryRedisCache              redis.UniversalClient
	metaRedisCache               redis.UniversalClient
	sdkRedisCache                redis.UniversalClient
	staticRedisHandle            *redisplane.Handle
	objectRedisHandle            *redisplane.Handle
	queryRedisHandle             *redisplane.Handle
	metaRedisHandle              *redisplane.Handle
	sdkRedisHandle               *redisplane.Handle
	lockRedisHandle              *redisplane.Handle
	cacheOptions                 ContainerCacheOptions
	policyCatalog                *cachepolicy.PolicyCatalog
	hotsetRecorder               scaleCache.HotsetRecorder
	hotsetInspector              scaleCache.HotsetInspector
	WarmupCoordinator            cachegov.Coordinator
	CacheGovernanceStatusService cachegov.StatusService
	planEntryURL                 string
	statisticsRepairWindowDays   int

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
	QRCodeGenerator wechatPort.QRCodeGenerator            // 小程序码生成器（可选）
	SubscribeSender wechatPort.MiniProgramSubscribeSender // 小程序订阅消息发送器（可选）

	// 应用层服务
	QRCodeService                      qrcodeApp.QRCodeService                            // 小程序码生成服务（可选）
	MiniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService // 小程序 task 消息服务（可选）

	// 容器状态
	initialized bool
	silent      bool
}

func firstPositiveDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
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
	// StaticRedisHandle 静态/半静态对象缓存 family handle。
	StaticRedisHandle *redisplane.Handle
	// QueryRedisHandle 查询结果缓存 family handle。
	QueryRedisHandle *redisplane.Handle
	// ObjectRedisHandle 对象视图缓存 family handle。
	ObjectRedisHandle *redisplane.Handle
	// SDKRedisHandle SDK token/cache family handle。
	SDKRedisHandle *redisplane.Handle
	// MetaRedisHandle query version token 与 hotset 等元数据缓存 family handle。
	MetaRedisHandle *redisplane.Handle
	// LockRedisHandle lock/lease Redis runtime handle.
	LockRedisHandle *redisplane.Handle
	// PlanEntryBaseURL 测评计划任务入口基础地址
	PlanEntryBaseURL string
	// StatisticsRepairWindowDays 统计夜间批处理默认回补窗口
	StatisticsRepairWindowDays int
	// Silent suppresses container stdout bootstrap/cleanup prints.
	Silent bool
}

// ContainerCacheOptions 缓存控制配置
type ContainerCacheOptions struct {
	DisableEvaluationCache bool
	DisableStatisticsCache bool
	TTL                    ContainerCacheTTLOptions
	TTLJitterRatio         float64
	StatisticsWarmup       *cachegov.StatisticsWarmupConfig
	Warmup                 ContainerWarmupOptions
	CompressPayload        bool
	Static                 ContainerCacheFamilyOptions
	Object                 ContainerCacheFamilyOptions
	Query                  ContainerCacheFamilyOptions
	Meta                   ContainerCacheFamilyOptions
	SDK                    ContainerCacheFamilyOptions
	Lock                   ContainerCacheFamilyOptions
}

type ContainerWarmupOptions struct {
	Enable          bool
	StartupStatic   bool
	StartupQuery    bool
	HotsetEnable    bool
	HotsetTopN      int64
	MaxItemsPerKind int64
}

// ContainerCacheFamilyOptions 定义单个缓存 family 的对象级策略。
// Redis 路由由 redisplane 统一提供，这里只保留 TTL、negative、压缩与 singleflight 语义。
type ContainerCacheFamilyOptions struct {
	TTL            time.Duration
	NegativeTTL    time.Duration
	TTLJitterRatio float64
	Compress       *bool
	Singleflight   *bool
	Negative       *bool
}

func resolvePolicySwitch(explicit *bool, defaultValue bool) cachepolicy.PolicySwitch {
	if explicit != nil {
		return cachepolicy.PolicySwitchFromBool(*explicit)
	}
	return cachepolicy.PolicySwitchFromBool(defaultValue)
}

func redisHandleClient(handle *redisplane.Handle) redis.UniversalClient {
	if handle == nil {
		return nil
	}
	return handle.Client
}

func redisHandleBuilder(handle *redisplane.Handle) *rediskey.Builder {
	if handle == nil {
		return nil
	}
	return handle.Builder
}

// ContainerCacheTTLOptions 缓存 TTL 配置（0 表示使用默认值）
type ContainerCacheTTLOptions struct {
	Scale            time.Duration
	ScaleList        time.Duration
	Questionnaire    time.Duration
	AssessmentDetail time.Duration
	AssessmentList   time.Duration
	Testee           time.Duration
	Plan             time.Duration
	Negative         time.Duration
}

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
	c.planEntryURL = opts.PlanEntryBaseURL
	c.statisticsRepairWindowDays = opts.StatisticsRepairWindowDays
	c.silent = opts.Silent
	c.staticRedisHandle = opts.StaticRedisHandle
	c.queryRedisHandle = opts.QueryRedisHandle
	c.objectRedisHandle = opts.ObjectRedisHandle
	c.metaRedisHandle = opts.MetaRedisHandle
	c.sdkRedisHandle = opts.SDKRedisHandle
	c.lockRedisHandle = opts.LockRedisHandle
	c.staticRedisCache = redisHandleClient(c.staticRedisHandle)
	c.queryRedisCache = redisHandleClient(c.queryRedisHandle)
	c.objectRedisCache = redisHandleClient(c.objectRedisHandle)
	c.metaRedisCache = redisHandleClient(c.metaRedisHandle)
	c.sdkRedisCache = redisHandleClient(c.sdkRedisHandle)

	c.policyCatalog = cachepolicy.NewPolicyCatalog(map[redisplane.Family]cachepolicy.CachePolicy{
		redisplane.FamilyStatic: {
			Compress:     resolvePolicySwitch(opts.Cache.Static.Compress, opts.Cache.CompressPayload),
			Singleflight: resolvePolicySwitch(opts.Cache.Static.Singleflight, true),
			Negative:     resolvePolicySwitch(opts.Cache.Static.Negative, false),
			NegativeTTL:  firstPositiveDuration(opts.Cache.Static.NegativeTTL, opts.Cache.TTL.Negative),
			JitterRatio:  firstPositiveFloat(opts.Cache.Static.TTLJitterRatio, opts.Cache.TTLJitterRatio),
		},
		redisplane.FamilyObject: {
			Compress:     resolvePolicySwitch(opts.Cache.Object.Compress, opts.Cache.CompressPayload),
			Singleflight: resolvePolicySwitch(opts.Cache.Object.Singleflight, true),
			Negative:     resolvePolicySwitch(opts.Cache.Object.Negative, false),
			NegativeTTL:  firstPositiveDuration(opts.Cache.Object.NegativeTTL, opts.Cache.TTL.Negative),
			JitterRatio:  firstPositiveFloat(opts.Cache.Object.TTLJitterRatio, opts.Cache.TTLJitterRatio),
		},
		redisplane.FamilyQuery: {
			TTL:          opts.Cache.Query.TTL,
			NegativeTTL:  firstPositiveDuration(opts.Cache.Query.NegativeTTL, opts.Cache.TTL.Negative),
			Compress:     resolvePolicySwitch(opts.Cache.Query.Compress, opts.Cache.CompressPayload),
			Singleflight: resolvePolicySwitch(opts.Cache.Query.Singleflight, false),
			Negative:     resolvePolicySwitch(opts.Cache.Query.Negative, false),
			JitterRatio:  firstPositiveFloat(opts.Cache.Query.TTLJitterRatio, opts.Cache.TTLJitterRatio),
		},
		redisplane.FamilySDK: {
			Compress:     resolvePolicySwitch(opts.Cache.SDK.Compress, false),
			Singleflight: resolvePolicySwitch(opts.Cache.SDK.Singleflight, false),
			Negative:     resolvePolicySwitch(opts.Cache.SDK.Negative, false),
			NegativeTTL:  opts.Cache.SDK.NegativeTTL,
			JitterRatio:  firstPositiveFloat(opts.Cache.SDK.TTLJitterRatio, opts.Cache.TTLJitterRatio),
		},
		redisplane.FamilyLock: {
			Compress:     resolvePolicySwitch(opts.Cache.Lock.Compress, false),
			Singleflight: resolvePolicySwitch(opts.Cache.Lock.Singleflight, false),
			Negative:     resolvePolicySwitch(opts.Cache.Lock.Negative, false),
			NegativeTTL:  opts.Cache.Lock.NegativeTTL,
			JitterRatio:  firstPositiveFloat(opts.Cache.Lock.TTLJitterRatio, opts.Cache.TTLJitterRatio),
		},
	}, map[cachepolicy.CachePolicyKey]cachepolicy.CachePolicy{
		cachepolicy.PolicyScale: {
			TTL: opts.Cache.TTL.Scale,
		},
		cachepolicy.PolicyScaleList: {
			TTL:          opts.Cache.TTL.ScaleList,
			Singleflight: cachepolicy.PolicySwitchDisabled,
		},
		cachepolicy.PolicyQuestionnaire: {
			TTL:         opts.Cache.TTL.Questionnaire,
			NegativeTTL: opts.Cache.TTL.Negative,
			Negative:    cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyAssessmentDetail: {
			TTL:          opts.Cache.TTL.AssessmentDetail,
			Singleflight: cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyAssessmentList: {
			TTL:          opts.Cache.TTL.AssessmentList,
			Singleflight: cachepolicy.PolicySwitchDisabled,
		},
		cachepolicy.PolicyTestee: {
			TTL:         opts.Cache.TTL.Testee,
			NegativeTTL: opts.Cache.TTL.Negative,
			Negative:    cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyPlan: {
			TTL:          opts.Cache.TTL.Plan,
			Singleflight: cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyStatsQuery: {
			Singleflight: cachepolicy.PolicySwitchDisabled,
		},
	})
	c.hotsetRecorder = scaleCache.NewRedisHotsetStore(c.metaRedisCache, redisHandleBuilder(c.metaRedisHandle), scaleCache.HotsetOptions{
		Enable:          opts.Cache.Warmup.HotsetEnable,
		TopN:            opts.Cache.Warmup.HotsetTopN,
		MaxItemsPerKind: opts.Cache.Warmup.MaxItemsPerKind,
	})
	if inspector, ok := c.hotsetRecorder.(scaleCache.HotsetInspector); ok {
		c.hotsetInspector = inspector
	}

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
	if c.SurveyModule != nil && c.ScaleModule != nil {
		c.SurveyModule.SetScaleRepository(c.ScaleModule.Repo)
	}

	// 初始化 Actor 模块
	if err := c.initActorModule(); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	// 初始化 Evaluation 模块
	if err := c.initEvaluationModule(); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}

	// 将评估服务注入到 Actor 模块（因为 Actor 模块在 Evaluation 模块之前初始化）
	if c.ActorModule != nil && c.EvaluationModule != nil {
		c.ActorModule.SetEvaluationServices(
			c.EvaluationModule.ManagementService,
			c.EvaluationModule.ScoreQueryService,
		)
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

	if c.ActorModule != nil && c.ActorModule.TesteeAccessService != nil {
		if c.EvaluationModule != nil {
			c.EvaluationModule.SetTesteeAccessService(c.ActorModule.TesteeAccessService)
		}
		if c.PlanModule != nil {
			c.PlanModule.SetTesteeAccessService(c.ActorModule.TesteeAccessService)
		}
		if c.StatisticsModule != nil {
			c.StatisticsModule.SetTesteeAccessService(c.ActorModule.TesteeAccessService)
		}
	}

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

// WarmupCache 预热缓存（异步执行，不阻塞）
func (c *Container) WarmupCache(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}
	if c.WarmupCoordinator != nil {
		if err := c.WarmupCoordinator.WarmStartup(ctx); err != nil {
			return fmt.Errorf("cache governance startup warmup failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("cache governance warmup coordinator is unavailable")
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

// initSurveyModule 初始化 Survey 模块（包含问卷和答卷子模块）
func (c *Container) initSurveyModule() error {
	surveyModule := assembler.NewSurveyModule()
	var identitySvc *iam.IdentityService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		identitySvc = c.IAMModule.IdentityService()
	}
	// 传入 Redis 客户端（用于问卷缓存装饰器）
	if err := surveyModule.Initialize(
		c.mongoDB,
		c.eventPublisher,
		c.staticRedisCache,
		redisHandleBuilder(c.staticRedisHandle),
		identitySvc,
		c.policyCatalog.Policy(cachepolicy.PolicyQuestionnaire),
		c.hotsetRecorder,
	); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	c.SurveyModule = surveyModule
	modulePool["survey"] = surveyModule

	c.printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}

// initScaleModule 初始化 Scale 模块
func (c *Container) initScaleModule() error {
	scaleModule := assembler.NewScaleModule()
	var identitySvc *iam.IdentityService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		identitySvc = c.IAMModule.IdentityService()
	}
	// 传入问卷仓库（如果 SurveyModule 已初始化）
	var questionnaireRepo interface{}
	if c.SurveyModule != nil && c.SurveyModule.Questionnaire != nil {
		questionnaireRepo = c.SurveyModule.Questionnaire.Repo
	}
	// 传入 Redis 客户端（用于缓存装饰器）
	if err := scaleModule.Initialize(
		c.mongoDB,
		c.eventPublisher,
		questionnaireRepo,
		c.staticRedisCache,
		redisHandleBuilder(c.staticRedisHandle),
		identitySvc,
		c.policyCatalog.Policy(cachepolicy.PolicyScale),
		c.policyCatalog.Policy(cachepolicy.PolicyScaleList),
		c.hotsetRecorder,
	); err != nil {
		return fmt.Errorf("failed to initialize scale module: %w", err)
	}

	c.ScaleModule = scaleModule
	modulePool["scale"] = scaleModule

	c.printf("📦 Scale module initialized\n")
	return nil
}

// initActorModule 初始化 Actor 模块
func (c *Container) initActorModule() error {
	actorModule := assembler.NewActorModule()

	// 获取 guardianshipSvc（如果 IAM 模块已启用）
	var guardianshipSvc *iam.GuardianshipService
	var identitySvc *iam.IdentityService
	var operationAccountSvc *iam.OperationAccountService
	var opAuthz *iam.OperatorAuthzBundle
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		guardianshipSvc = c.IAMModule.GuardianshipService()
		identitySvc = c.IAMModule.IdentityService()
		operationAccountSvc = c.IAMModule.OperationAccountService()
		opAuthz = &iam.OperatorAuthzBundle{
			Assignment: iam.NewAuthzAssignmentClient(c.IAMModule.Client()),
			Snapshot:   c.IAMModule.AuthzSnapshotLoader(),
		}
	}

	if err := actorModule.Initialize(
		c.mysqlDB,
		guardianshipSvc,
		identitySvc,
		c.objectRedisCache,
		redisHandleBuilder(c.objectRedisHandle),
		c.policyCatalog.Policy(cachepolicy.PolicyTestee),
		opAuthz,
		operationAccountSvc,
	); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	c.ActorModule = actorModule
	modulePool["actor"] = actorModule

	c.printf("📦 Actor module initialized\n")
	return nil
}

// initEvaluationModule 初始化 Evaluation 模块
func (c *Container) initEvaluationModule() error {
	evaluationModule := assembler.NewEvaluationModule()
	// 传入 ScaleRepo、AnswerSheetRepo、QuestionnaireRepo、EventPublisher 和 Redis 客户端
	// 注意：参数顺序必须与 EvaluationModule.Initialize 中的 params 索引一致
	// params[0]: MySQL, params[1]: MongoDB, params[2]: ScaleRepo, params[3]: AnswerSheetRepo, params[4]: QuestionnaireRepo, params[5]: EventPublisher, params[6]: Redis
	redisClient := c.objectRedisCache
	if c.cacheOptions.DisableEvaluationCache {
		redisClient = nil
	}
	queryRedisClient := c.queryRedisCache
	if c.cacheOptions.DisableEvaluationCache {
		queryRedisClient = nil
	}
	var versionStore scaleCache.VersionTokenStore
	if queryRedisClient != nil && c.metaRedisCache != nil {
		versionStore = scaleCache.NewRedisVersionTokenStoreWithKind(c.metaRedisCache, string(cachepolicy.PolicyAssessmentList))
	}
	if err := evaluationModule.Initialize(
		c.mysqlDB,
		c.mongoDB,
		c.ScaleModule.Repo,
		c.SurveyModule.AnswerSheet.Repo,
		c.SurveyModule.Questionnaire.Repo, // params[4]: QuestionnaireRepo
		c.eventPublisher,                  // params[5]: EventPublisher
		redisClient,                       // params[6]: Redis 客户端（用于缓存）
		redisHandleBuilder(c.objectRedisHandle),
		c.policyCatalog.Policy(cachepolicy.PolicyAssessmentDetail),
		queryRedisClient,
		redisHandleBuilder(c.queryRedisHandle),
		c.policyCatalog.Policy(cachepolicy.PolicyAssessmentList),
		versionStore,
	); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}

	c.EvaluationModule = evaluationModule
	modulePool["evaluation"] = evaluationModule

	c.printf("📦 Evaluation module initialized\n")
	return nil
}

// initPlanModule 初始化 Plan 模块
func (c *Container) initPlanModule() error {
	planModule := assembler.NewPlanModule()
	// 传入 ScaleRepo 用于通过 code 查找 scale，以及 Redis 客户端用于缓存
	var scaleRepo scale.Repository
	if c.ScaleModule != nil {
		scaleRepo = c.ScaleModule.Repo
	}
	if err := planModule.Initialize(
		c.mysqlDB,
		c.eventPublisher,
		scaleRepo,
		c.objectRedisCache,
		redisHandleBuilder(c.objectRedisHandle),
		c.policyCatalog.Policy(cachepolicy.PolicyPlan),
		c.planEntryURL,
	); err != nil {
		return fmt.Errorf("failed to initialize plan module: %w", err)
	}

	c.PlanModule = planModule
	modulePool["plan"] = planModule

	c.printf("📦 Plan module initialized\n")
	return nil
}

// initStatisticsModule 初始化 Statistics 模块
func (c *Container) initStatisticsModule() error {
	statisticsModule := assembler.NewStatisticsModule()
	// 传入 MySQL 和 Redis 客户端
	redisClient := c.redisCache
	if c.cacheOptions.DisableStatisticsCache {
		redisClient = nil
	}
	var answerSheetRepo interface{}
	if c.SurveyModule != nil && c.SurveyModule.AnswerSheet != nil {
		answerSheetRepo = c.SurveyModule.AnswerSheet.Repo
	}
	if !c.cacheOptions.DisableStatisticsCache {
		redisClient = c.queryRedisCache
	}
	if err := statisticsModule.Initialize(
		c.mysqlDB,
		redisClient,
		redisHandleBuilder(c.queryRedisHandle),
		answerSheetRepo,
		c.statisticsRepairWindowDays,
		c.policyCatalog.Policy(cachepolicy.PolicyStatsQuery),
		c.hotsetRecorder,
		redislock.NewManager("apiserver", "statistics_sync", c.lockRedisHandle),
	); err != nil {
		return fmt.Errorf("failed to initialize statistics module: %w", err)
	}

	c.StatisticsModule = statisticsModule
	modulePool["statistics"] = statisticsModule

	c.printf("📦 Statistics module initialized\n")
	return nil
}

func (c *Container) initWarmupCoordinator() error {
	if c == nil {
		return nil
	}
	var warmScale func(context.Context, string) error
	var warmQuestionnaire func(context.Context, string) error
	var warmScaleList func(context.Context) error
	if c.staticRedisCache != nil {
		warmScale = c.warmScaleCacheTarget
		warmQuestionnaire = c.warmQuestionnaireCacheTarget
		warmScaleList = c.warmScaleListTarget
	}
	var warmStatsSystem func(context.Context, int64) error
	var warmStatsQuestionnaire func(context.Context, int64, string) error
	var warmStatsPlan func(context.Context, int64, uint64) error
	if c.queryRedisCache != nil && !c.cacheOptions.DisableStatisticsCache {
		warmStatsSystem = c.warmSystemStatsTarget
		warmStatsQuestionnaire = c.warmQuestionnaireStatsTarget
		warmStatsPlan = c.warmPlanStatsTarget
	}
	c.WarmupCoordinator = cachegov.NewCoordinator(cachegov.Config{
		Enable:          c.cacheOptions.Warmup.Enable,
		StartupStatic:   c.cacheOptions.Warmup.StartupStatic,
		StartupQuery:    c.cacheOptions.Warmup.StartupQuery,
		HotsetEnable:    c.cacheOptions.Warmup.HotsetEnable,
		HotsetTopN:      c.cacheOptions.Warmup.HotsetTopN,
		MaxItemsPerKind: c.cacheOptions.Warmup.MaxItemsPerKind,
	}, cachegov.Dependencies{
		Runtime:                         cachegov.NewFamilyRuntime(c.staticRedisHandle, c.queryRedisHandle),
		StatisticsSeeds:                 c.cacheOptions.StatisticsWarmup,
		Hotset:                          c.hotsetRecorder,
		ListPublishedScaleCodes:         c.listPublishedScaleCodes,
		ListPublishedQuestionnaireCodes: c.listPublishedQuestionnaireCodes,
		LookupScaleQuestionnaireCode:    c.lookupScaleQuestionnaireCode,
		WarmScale:                       warmScale,
		WarmQuestionnaire:               warmQuestionnaire,
		WarmScaleList:                   warmScaleList,
		WarmStatsSystem:                 warmStatsSystem,
		WarmStatsQuestionnaire:          warmStatsQuestionnaire,
		WarmStatsPlan:                   warmStatsPlan,
	})
	if c.StatisticsModule != nil {
		c.StatisticsModule.SetWarmupCoordinator(c.WarmupCoordinator)
	}
	c.CacheGovernanceStatusService = cachegov.NewStatusService("apiserver", nil, c.hotsetInspector, c.WarmupCoordinator)
	return nil
}

func (c *Container) listPublishedScaleCodes(ctx context.Context) ([]string, error) {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := c.ScaleModule.Repo.FindSummaryList(ctx, page, pageSize, map[string]interface{}{
			"status": scale.StatusPublished.Value(),
		})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			codes = append(codes, item.GetCode().String())
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (c *Container) listPublishedQuestionnaireCodes(ctx context.Context) ([]string, error) {
	if c == nil || c.SurveyModule == nil || c.SurveyModule.Questionnaire == nil || c.SurveyModule.Questionnaire.Repo == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := c.SurveyModule.Questionnaire.Repo.FindBasePublishedList(ctx, page, pageSize, map[string]interface{}{
			"status": "published",
		})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			codes = append(codes, item.GetCode().String())
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (c *Container) lookupScaleQuestionnaireCode(ctx context.Context, code string) (string, error) {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil {
		return "", nil
	}
	item, err := c.ScaleModule.Repo.FindByCode(ctx, code)
	if err != nil || item == nil {
		return "", err
	}
	return item.GetQuestionnaireCode().String(), nil
}

func (c *Container) warmScaleCacheTarget(ctx context.Context, code string) error {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := c.ScaleModule.Repo.(*scaleCache.CachedScaleRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := c.ScaleModule.Repo.FindByCode(ctx, code)
	return err
}

func (c *Container) warmQuestionnaireCacheTarget(ctx context.Context, code string) error {
	if c == nil || c.SurveyModule == nil || c.SurveyModule.Questionnaire == nil || c.SurveyModule.Questionnaire.Repo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := c.SurveyModule.Questionnaire.Repo.(*scaleCache.CachedQuestionnaireRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := c.SurveyModule.Questionnaire.Repo.FindBaseByCode(ctx, code)
	return err
}

func (c *Container) warmScaleListTarget(ctx context.Context) error {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.ListCache == nil {
		return nil
	}
	return c.ScaleModule.ListCache.Rebuild(ctx)
}

func (c *Container) warmSystemStatsTarget(ctx context.Context, orgID int64) error {
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.SystemStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.SystemStatisticsService.GetSystemStatistics(ctx, orgID)
	return err
}

func (c *Container) warmQuestionnaireStatsTarget(ctx context.Context, orgID int64, code string) error {
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.QuestionnaireStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.QuestionnaireStatisticsService.GetQuestionnaireStatistics(ctx, orgID, code)
	return err
}

func (c *Container) warmPlanStatsTarget(ctx context.Context, orgID int64, planID uint64) error {
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.PlanStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.PlanStatisticsService.GetPlanStatistics(ctx, orgID, planID)
	return err
}

// initCodesService 初始化 CodesService
func (c *Container) initCodesService() {
	// 如果已经有实现则不覆盖
	if c.CodesService != nil {
		return
	}
	c.CodesService = codesapp.NewService()
	c.printf("🔑 CodesService initialized\n")
}

// initQRCodeGenerator 初始化小程序码生成器（基础设施层）
func (c *Container) initQRCodeGenerator() {
	// 创建微信 SDK 缓存适配器（使用 Redis，如果 Redis 不可用则使用内存缓存）
	var wechatCache cache.Cache
	if c.sdkRedisCache != nil {
		// 使用 Redis 缓存适配器
		wechatCache = wechatapi.NewRedisCacheAdapterWithBuilder(c.sdkRedisCache, redisHandleBuilder(c.sdkRedisHandle))
	} else {
		// 降级使用内存缓存
		wechatCache = cache.NewMemory()
	}

	c.QRCodeGenerator = wechatapi.NewQRCodeGenerator(wechatCache)
	c.SubscribeSender = wechatapi.NewSubscribeSender(wechatCache)
	c.printf("📱 QRCode generator initialized (infrastructure layer)\n")
}

// InitQRCodeService 初始化小程序码生成服务（应用层）
// 从配置中读取 wechat_app_id，然后从 IAM 查询微信应用信息
func (c *Container) InitQRCodeService(wechatOptions *options.WeChatOptions) {
	// 如果基础设施层未初始化，则应用层服务也不初始化
	if c.QRCodeGenerator == nil {
		c.printf("⚠️  QRCode service not initialized (generator not available)\n")
		return
	}

	// 如果未提供配置，则不初始化
	if wechatOptions == nil {
		c.printf("⚠️  QRCode service not initialized (wechat options not provided)\n")
		return
	}

	// 检查是否有配置
	if wechatOptions.WeChatAppID == "" && (wechatOptions.AppID == "" || wechatOptions.AppSecret == "") {
		c.printf("⚠️  QRCode service not initialized (missing config: wechat-app-id or app-id/app-secret)\n")
		return
	}

	if wechatOptions.PagePath == "" {
		c.printf("⚠️  QRCode service not initialized (missing page-path)\n")
		return
	}

	// 获取 WeChatAppService（如果 IAM 模块已初始化）
	var wechatAppService *iam.WeChatAppService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		wechatAppService = c.IAMModule.WeChatAppService()
	}

	// 创建应用层服务配置
	config := &qrcodeApp.Config{
		PagePath: wechatOptions.PagePath,
	}

	// 优先使用 IAM 查询（通过 WeChatAppID）
	if wechatOptions.WeChatAppID != "" {
		config.WeChatAppID = wechatOptions.WeChatAppID
		c.printf("📱 QRCode service will use IAM to query wechat app (wechat_app_id: %s)\n", wechatOptions.WeChatAppID)
	} else {
		// 降级：使用直接配置
		config.AppID = wechatOptions.AppID
		config.AppSecret = wechatOptions.AppSecret
		c.printf("📱 QRCode service will use direct config (app_id: %s)\n", wechatOptions.AppID)
	}

	// 创建应用层服务，封装基础设施层调用
	c.QRCodeService = qrcodeApp.NewService(
		c.QRCodeGenerator,
		config,
		wechatAppService,
	)
	c.printf("📱 QRCode service initialized (application layer, page_path: %s)\n", wechatOptions.PagePath)
}

// InitMiniProgramTaskNotificationService 初始化 task.opened 小程序消息服务。
func (c *Container) InitMiniProgramTaskNotificationService(wechatOptions *options.WeChatOptions) {
	if c.SubscribeSender == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (subscribe sender not available)\n")
		return
	}
	if c.ActorModule == nil || c.ActorModule.TesteeQueryService == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (testee query service not available)\n")
		return
	}
	if c.PlanModule == nil || c.PlanModule.TaskRepo == nil || c.PlanModule.PlanRepo == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (plan repositories not available)\n")
		return
	}
	if wechatOptions == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (wechat options not provided)\n")
		return
	}
	if strings.TrimSpace(wechatOptions.TaskOpenedTemplateID) == "" {
		c.printf("⚠️  MiniProgram task notification service not initialized (missing task-opened-template-id)\n")
		return
	}
	if wechatOptions.WeChatAppID == "" && (wechatOptions.AppID == "" || wechatOptions.AppSecret == "") {
		c.printf("⚠️  MiniProgram task notification service not initialized (missing wechat app config)\n")
		return
	}

	var wechatAppService *iam.WeChatAppService
	var guardianshipSvc *iam.GuardianshipService
	var identitySvc *iam.IdentityService
	var scaleQueryService scaleApp.ScaleQueryService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		wechatAppService = c.IAMModule.WeChatAppService()
		guardianshipSvc = c.IAMModule.GuardianshipService()
		identitySvc = c.IAMModule.IdentityService()
	}
	if c.ScaleModule != nil {
		scaleQueryService = c.ScaleModule.QueryService
	}

	c.MiniProgramTaskNotificationService = notificationApp.NewMiniProgramTaskNotificationService(
		c.ActorModule.TesteeQueryService,
		c.PlanModule.TaskRepo,
		c.PlanModule.PlanRepo,
		scaleQueryService,
		guardianshipSvc,
		identitySvc,
		wechatAppService,
		c.SubscribeSender,
		&notificationApp.Config{
			WeChatAppID:          wechatOptions.WeChatAppID,
			PagePath:             wechatOptions.PagePath,
			AppID:                wechatOptions.AppID,
			AppSecret:            wechatOptions.AppSecret,
			TaskOpenedTemplateID: wechatOptions.TaskOpenedTemplateID,
		},
	)
	c.printf("📨 MiniProgram task notification service initialized (template_id: %s)\n", wechatOptions.TaskOpenedTemplateID)
}

// HealthCheck 健康检查
func (c *Container) HealthCheck(ctx context.Context) error {
	// 检查 IAM 连接
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		if err := c.IAMModule.HealthCheck(ctx); err != nil {
			return fmt.Errorf("IAM health check failed: %w", err)
		}
	}

	// 检查MySQL连接
	if c.mysqlDB != nil {
		sqlDB, err := c.mysqlDB.DB()
		if err != nil {
			return fmt.Errorf("failed to get mysql db: %w", err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("mysql ping failed: %w", err)
		}
	}

	// 检查MongoDB连接（如果有）
	if c.mongoDB != nil {
		if err := c.mongoDB.Client().Ping(ctx, nil); err != nil {
			return fmt.Errorf("mongodb ping failed: %w", err)
		}
	}

	// 检查 Redis 连接
	if c.redisCache != nil {
		if err := c.redisCache.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis cache ping failed: %w", err)
		}
	}

	// 检查模块健康状态
	if err := c.checkModulesHealth(ctx); err != nil {
		return fmt.Errorf("modules health check failed: %w", err)
	}

	return nil
}

// checkModulesHealth 检查模块健康状态
func (c *Container) checkModulesHealth(ctx context.Context) error {
	for _, module := range modulePool {
		if err := module.CheckHealth(); err != nil {
			return fmt.Errorf("module health check failed: %w", err)
		}
	}
	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() error {
	c.printf("🧹 Cleaning up container resources...\n")

	// 清理 IAM 模块
	if c.IAMModule != nil {
		if err := c.IAMModule.Close(); err != nil {
			return fmt.Errorf("failed to cleanup IAM module: %w", err)
		}
		c.printf("   ✅ IAM module cleaned up\n")
	}

	for _, module := range modulePool {
		if err := module.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup module: %w", err)
		}
		c.printf("   ✅ %s module cleaned up\n", module.ModuleInfo().Name)
	}

	c.initialized = false
	c.printf("🏁 Container cleanup completed\n")

	return nil
}

// GetContainerInfo 获取容器信息
func (c *Container) GetContainerInfo() map[string]interface{} {
	modules := make(map[string]interface{})
	for _, module := range modulePool {
		modules[module.ModuleInfo().Name] = module.ModuleInfo()
	}

	return map[string]interface{}{
		"name":         "apiserver-container",
		"version":      "2.0.0",
		"architecture": "hexagonal",
		"initialized":  c.initialized,
		"modules":      modules,
		"infrastructure": map[string]bool{
			"mysql":   c.mysqlDB != nil,
			"mongodb": c.mongoDB != nil,
			"redis":   c.redisCache != nil,
		},
	}
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}

// GetLoadedModules 获取已加载的模块列表
func (c *Container) GetLoadedModules() []string {
	modules := make([]string, 0)

	for _, module := range modulePool {
		modules = append(modules, module.ModuleInfo().Name)
	}

	return modules
}

func (c *Container) HotsetInspector() scaleCache.HotsetInspector {
	if c == nil {
		return nil
	}
	return c.hotsetInspector
}

// PrintContainerInfo 打印容器信息
func (c *Container) PrintContainerInfo() {
	info := c.GetContainerInfo()

	fmt.Printf("🏗️  Container Information:\n")
	fmt.Printf("   Name: %s\n", info["name"].(string))
	fmt.Printf("   Version: %s\n", info["version"].(string))
	fmt.Printf("   Architecture: %s\n", info["architecture"].(string))
	fmt.Printf("   Initialized: %v\n", info["initialized"].(bool))

	infra := info["infrastructure"].(map[string]bool)
	fmt.Printf("   Infrastructure:\n")
	if infra["mysql"] {
		fmt.Printf("     • MySQL: ✅\n")
	} else {
		fmt.Printf("     • MySQL: ❌\n")
	}
	if infra["mongodb"] {
		fmt.Printf("     • MongoDB: ✅\n")
	} else {
		fmt.Printf("     • MongoDB: ❌\n")
	}

	fmt.Printf("   Loaded Modules:\n")
	for _, module := range c.GetLoadedModules() {
		fmt.Printf("     • %s\n", module)
	}
}
