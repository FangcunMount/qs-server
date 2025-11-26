package container

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
)

// modulePool 模块池
var modulePool = make(map[string]assembler.Module)

// Container 主容器
// 组合所有业务模块和基础设施组件
type Container struct {
	// 基础设施
	mysqlDB *gorm.DB
	mongoDB *mongo.Database

	// 业务模块
	SurveyModule          *assembler.SurveyModule // Survey 模块（包含问卷和答卷子模块）
	MedicalScaleModule    *assembler.MedicalScaleModule
	InterpretReportModule *assembler.InterpretReportModule
	ActorModule           *assembler.ActorModule

	// 容器状态
	initialized bool
}

// NewContainer 创建容器
func NewContainer(mysqlDB *gorm.DB, mongoDB *mongo.Database) *Container {
	return &Container{
		mysqlDB:     mysqlDB,
		mongoDB:     mongoDB,
		initialized: false,
	}
}

// Initialize 初始化容器
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	// 初始化 Survey 模块（包含问卷和答卷子模块）
	if err := c.initSurveyModule(); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	// 初始化医学量表模块
	if err := c.initMedicalScaleModule(); err != nil {
		return fmt.Errorf("failed to initialize medical scale module: %w", err)
	}

	// 初始化解读报告模块
	if err := c.initInterpretReportModule(); err != nil {
		return fmt.Errorf("failed to initialize interpret report module: %w", err)
	}

	// 初始化 Actor 模块
	if err := c.initActorModule(); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	c.initialized = true
	fmt.Printf("🏗️  Container initialized successfully\n")

	return nil
}

// initSurveyModule 初始化 Survey 模块（包含问卷和答卷子模块）
func (c *Container) initSurveyModule() error {
	surveyModule := assembler.NewSurveyModule()
	if err := surveyModule.Initialize(c.mongoDB); err != nil {
		return fmt.Errorf("failed to initialize survey module: %w", err)
	}

	c.SurveyModule = surveyModule
	modulePool["survey"] = surveyModule

	fmt.Printf("📦 Survey module initialized (questionnaire + answersheet)\n")
	return nil
}

// initMedicalScaleModule 初始化医学量表模块
func (c *Container) initMedicalScaleModule() error {
	medicalScaleModule := assembler.NewMedicalScaleModule()
	if err := medicalScaleModule.Initialize(c.mongoDB); err != nil {
		return fmt.Errorf("failed to initialize medical scale module: %w", err)
	}

	c.MedicalScaleModule = medicalScaleModule
	modulePool["medicalscale"] = medicalScaleModule

	fmt.Printf("📦 Medical scale module initialized\n")
	return nil
}

// initInterpretReportModule 初始化解读报告模块
func (c *Container) initInterpretReportModule() error {
	interpretReportModule := assembler.NewInterpretReportModule(c.mongoDB)

	c.InterpretReportModule = interpretReportModule
	modulePool["interpretreport"] = interpretReportModule

	fmt.Printf("📦 Interpret report module initialized\n")
	return nil
}

// initActorModule 初始化 Actor 模块
func (c *Container) initActorModule() error {
	actorModule := assembler.NewActorModule()
	if err := actorModule.Initialize(c.mysqlDB); err != nil {
		return fmt.Errorf("failed to initialize actor module: %w", err)
	}

	c.ActorModule = actorModule
	modulePool["actor"] = actorModule

	fmt.Printf("📦 Actor module initialized\n")
	return nil
}

// HealthCheck 健康检查
func (c *Container) HealthCheck(ctx context.Context) error {
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
