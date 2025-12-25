package container

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/messaging"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"

	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
)

// modulePool 模块池
var modulePool = make(map[string]assembler.Module)

// Container 主容器
// 组合所有业务模块和基础设施组件
type Container struct {
	// 基础设施
	mysqlDB    *gorm.DB
	mongoDB    *mongo.Database
	redisCache redis.UniversalClient
	redisStore redis.UniversalClient

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

	// 容器状态
	initialized bool
}

// NewContainer 创建容器
func NewContainer(mysqlDB *gorm.DB, mongoDB *mongo.Database, redisCache redis.UniversalClient, redisStore redis.UniversalClient) *Container {
	return &Container{
		mysqlDB:       mysqlDB,
		mongoDB:       mongoDB,
		redisCache:    redisCache,
		redisStore:    redisStore,
		publisherMode: eventconfig.PublishModeLogging, // 默认使用日志模式
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
}

// NewContainerWithOptions 创建带配置的容器
func NewContainerWithOptions(mysqlDB *gorm.DB, mongoDB *mongo.Database, redisCache redis.UniversalClient, redisStore redis.UniversalClient, opts ContainerOptions) *Container {
	c := NewContainer(mysqlDB, mongoDB, redisCache, redisStore)
	c.mqPublisher = opts.MQPublisher

	// 根据环境或显式配置确定发布器模式
	if opts.PublisherMode != "" {
		c.publisherMode = opts.PublisherMode
	} else if opts.Env != "" {
		c.publisherMode = eventconfig.PublishModeFromEnv(opts.Env)
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
	fmt.Printf("📋 Event config loaded (events.yaml)\n")

	// 初始化事件发布器（所有模块共享）
	c.initEventPublisher()
	fmt.Printf("📡 Event publisher initialized (mode=%s)\n", c.publisherMode)

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

	// 初始化 CodesService（基于 redisStore）
	c.initCodesService()

	c.initialized = true
	fmt.Printf("🏗️  Container initialized successfully\n")

	return nil
}

// WarmupCache 预热缓存（异步执行，不阻塞）
func (c *Container) WarmupCache(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}

	// 预热量表缓存
	if c.ScaleModule != nil && c.ScaleModule.Repo != nil {
		var warmupSvc *scaleCache.WarmupService
		// 如果问卷 Repository 可用，创建包含问卷的预热服务
		if c.SurveyModule != nil && c.SurveyModule.Questionnaire != nil && c.SurveyModule.Questionnaire.Repo != nil {
			warmupSvc = scaleCache.NewWarmupServiceWithQuestionnaire(
				c.ScaleModule.Repo,
				c.SurveyModule.Questionnaire.Repo,
			)
		} else {
			warmupSvc = scaleCache.NewWarmupService(c.ScaleModule.Repo)
		}

		if err := warmupSvc.WarmupDefaultScales(ctx); err != nil {
			// 预热失败不影响服务启动，仅记录日志
			return fmt.Errorf("scale cache warmup failed: %w", err)
		}
	}

	// 统计查询结果缓存预热（可选）
	// 注意：统计查询结果缓存 TTL 较短（5分钟），预热主要用于减少首次查询延迟
	// 建议：只在有明确需求时使用（如已知活跃组织、常用问卷等）
	// 可以通过配置或环境变量控制是否启用
	// if c.StatisticsModule != nil {
	// 	config := scaleCache.StatisticsWarmupConfig{
	// 		OrgIDs: []int64{1}, // 从配置读取
	// 		QuestionnaireCodes: []string{"QS001", "QS002"}, // 从配置读取
	// 	}
	// 	if err := scaleCache.WarmupStatisticsCache(ctx, config,
	// 		c.StatisticsModule.SystemStatisticsService,
	// 		c.StatisticsModule.QuestionnaireStatisticsService,
	// 		c.StatisticsModule.PlanStatisticsService,
	// 	); err != nil {
	// 		// 预热失败不影响服务启动
	// 	}
	// }

	return nil
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
	// 传入 Redis 客户端（用于问卷缓存装饰器）
	if err := surveyModule.Initialize(c.mongoDB, c.eventPublisher, c.redisCache); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	c.SurveyModule = surveyModule
	modulePool["survey"] = surveyModule

	fmt.Printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}

// initScaleModule 初始化 Scale 模块
func (c *Container) initScaleModule() error {
	scaleModule := assembler.NewScaleModule()
	// 传入问卷仓库（如果 SurveyModule 已初始化）
	var questionnaireRepo interface{}
	if c.SurveyModule != nil && c.SurveyModule.Questionnaire != nil {
		questionnaireRepo = c.SurveyModule.Questionnaire.Repo
	}
	// 传入 Redis 客户端（用于缓存装饰器）
	if err := scaleModule.Initialize(c.mongoDB, c.eventPublisher, questionnaireRepo, c.redisCache); err != nil {
		return fmt.Errorf("failed to initialize scale module: %w", err)
	}

	c.ScaleModule = scaleModule
	modulePool["scale"] = scaleModule

	fmt.Printf("📦 Scale module initialized\n")
	return nil
}

// initActorModule 初始化 Actor 模块
func (c *Container) initActorModule() error {
	actorModule := assembler.NewActorModule()

	// 获取 guardianshipSvc（如果 IAM 模块已启用）
	var guardianshipSvc *iam.GuardianshipService
	var identitySvc *iam.IdentityService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		guardianshipSvc = c.IAMModule.GuardianshipService()
		identitySvc = c.IAMModule.IdentityService()
	}

	if err := actorModule.Initialize(c.mysqlDB, guardianshipSvc, identitySvc, c.redisCache); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	c.ActorModule = actorModule
	modulePool["actor"] = actorModule

	fmt.Printf("📦 Actor module initialized\n")
	return nil
}

// initEvaluationModule 初始化 Evaluation 模块
func (c *Container) initEvaluationModule() error {
	evaluationModule := assembler.NewEvaluationModule()
	// 传入 ScaleRepo、AnswerSheetRepo、QuestionnaireRepo、EventPublisher 和 Redis 客户端
	// 注意：参数顺序必须与 EvaluationModule.Initialize 中的 params 索引一致
	// params[0]: MySQL, params[1]: MongoDB, params[2]: ScaleRepo, params[3]: AnswerSheetRepo, params[4]: QuestionnaireRepo, params[5]: EventPublisher, params[6]: Redis
	if err := evaluationModule.Initialize(
		c.mysqlDB,
		c.mongoDB,
		c.ScaleModule.Repo,
		c.SurveyModule.AnswerSheet.Repo,
		c.SurveyModule.Questionnaire.Repo, // params[4]: QuestionnaireRepo
		c.eventPublisher,                  // params[5]: EventPublisher
		c.redisCache,                      // params[6]: Redis 客户端（用于缓存）
	); err != nil {
		return fmt.Errorf("failed to initialize evaluation module: %w", err)
	}

	c.EvaluationModule = evaluationModule
	modulePool["evaluation"] = evaluationModule

	fmt.Printf("📦 Evaluation module initialized\n")
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
	if err := planModule.Initialize(c.mysqlDB, c.eventPublisher, scaleRepo, c.redisCache); err != nil {
		return fmt.Errorf("failed to initialize plan module: %w", err)
	}

	c.PlanModule = planModule
	modulePool["plan"] = planModule

	fmt.Printf("📦 Plan module initialized\n")
	return nil
}

// initStatisticsModule 初始化 Statistics 模块
func (c *Container) initStatisticsModule() error {
	statisticsModule := assembler.NewStatisticsModule()
	// 传入 MySQL 和 Redis 客户端
	if err := statisticsModule.Initialize(c.mysqlDB, c.redisCache); err != nil {
		return fmt.Errorf("failed to initialize statistics module: %w", err)
	}

	c.StatisticsModule = statisticsModule
	modulePool["statistics"] = statisticsModule

	fmt.Printf("📦 Statistics module initialized\n")
	return nil
}

// initCodesService 初始化 CodesService
func (c *Container) initCodesService() {
	// 如果已经有实现则不覆盖
	if c.CodesService != nil {
		return
	}
	if c.redisStore != nil {
		c.CodesService = codesapp.NewService(c.redisStore)
		fmt.Printf("🔑 CodesService initialized using redisStore\n")
		return
	}
	if c.redisCache != nil {
		c.CodesService = codesapp.NewService(c.redisCache)
		fmt.Printf("🔑 CodesService initialized using redisCache\n")
		return
	}
	// 无 redis 时使用 nil 或者 NewService 会回退到时间戳实现
	c.CodesService = codesapp.NewService(nil)
	fmt.Printf("🔑 CodesService initialized using fallback (no redis)\n")
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
	if c.redisStore != nil {
		if err := c.redisStore.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis store ping failed: %w", err)
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
	fmt.Printf("🧹 Cleaning up container resources...\n")

	// 清理 IAM 模块
	if c.IAMModule != nil {
		if err := c.IAMModule.Close(); err != nil {
			return fmt.Errorf("failed to cleanup IAM module: %w", err)
		}
		fmt.Printf("   ✅ IAM module cleaned up\n")
	}

	for _, module := range modulePool {
		if err := module.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup module: %w", err)
		}
		fmt.Printf("   ✅ %s module cleaned up\n", module.ModuleInfo().Name)
	}

	c.initialized = false
	fmt.Printf("🏁 Container cleanup completed\n")

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
			"redis":   c.redisCache != nil || c.redisStore != nil,
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
