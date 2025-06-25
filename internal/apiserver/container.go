package apiserver

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/composite"
	mongoAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mongodb"
	mysqlQuestionnaireAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mysql/questionnaire"
	mysqlUserAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mysql/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/services"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// ComponentType 组件类型
type ComponentType string

const (
	// 仓储层组件类型
	RepositoryType ComponentType = "repository"
	// 服务层组件类型
	ServiceType ComponentType = "service"
	// 处理器组件类型
	HandlerType ComponentType = "handler"
)

// ComponentFactory 组件工厂函数类型
type ComponentFactory func(container *Container) (interface{}, error)

// ComponentDefinition 组件定义
type ComponentDefinition struct {
	Name     string           // 组件名称
	Type     ComponentType    // 组件类型
	Factory  ComponentFactory // 工厂函数
	Instance interface{}      // 单例实例
}

// Container 依赖注入容器
// 使用注册器模式，支持动态扩展组件
type Container struct {
	// 外部依赖（基础设施）
	mysqlDB       *gorm.DB
	mongoClient   *mongo.Client
	mongoDatabase string

	// 组件注册表
	components map[string]*ComponentDefinition

	// 路由配置器
	router *Router
}

// NewContainer 创建新的依赖注入容器
func NewContainer(mysqlDB *gorm.DB, mongoClient *mongo.Client, mongoDatabase string) *Container {
	return &Container{
		mysqlDB:       mysqlDB,
		mongoClient:   mongoClient,
		mongoDatabase: mongoDatabase,
		components:    make(map[string]*ComponentDefinition),
	}
}

// RegisterComponent 注册组件
func (c *Container) RegisterComponent(name string, componentType ComponentType, factory ComponentFactory) {
	c.components[name] = &ComponentDefinition{
		Name:    name,
		Type:    componentType,
		Factory: factory,
	}
}

// GetComponent 获取组件实例（懒加载 + 单例）
func (c *Container) GetComponent(name string) (interface{}, error) {
	component, exists := c.components[name]
	if !exists {
		return nil, fmt.Errorf("component '%s' not registered", name)
	}

	// 懒加载：只有在第一次获取时才创建
	if component.Instance == nil {
		instance, err := component.Factory(c)
		if err != nil {
			return nil, fmt.Errorf("failed to create component '%s': %w", name, err)
		}
		component.Instance = instance
	}

	return component.Instance, nil
}

// GetComponentsByType 按类型获取所有组件
func (c *Container) GetComponentsByType(componentType ComponentType) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for name, component := range c.components {
		if component.Type == componentType {
			instance, err := c.GetComponent(name)
			if err != nil {
				return nil, err
			}
			result[name] = instance
		}
	}

	return result, nil
}

// MustGetComponent 获取组件实例（如果失败则panic）
func (c *Container) MustGetComponent(name string) interface{} {
	component, err := c.GetComponent(name)
	if err != nil {
		panic(fmt.Sprintf("failed to get component '%s': %v", name, err))
	}
	return component
}

// GetMysqlDB 获取MySQL数据库连接
func (c *Container) GetMysqlDB() *gorm.DB {
	return c.mysqlDB
}

// GetMongoClient 获取MongoDB客户端
func (c *Container) GetMongoClient() *mongo.Client {
	return c.mongoClient
}

// GetMongoDatabase 获取MongoDB数据库名
func (c *Container) GetMongoDatabase() string {
	return c.mongoDatabase
}

// Initialize 初始化所有组件
func (c *Container) Initialize() error {
	// 1. 注册核心组件
	if err := c.registerCoreComponents(); err != nil {
		return fmt.Errorf("failed to register core components: %w", err)
	}

	// 2. 初始化路由配置器
	if err := c.initializeRouter(); err != nil {
		return fmt.Errorf("failed to initialize router: %w", err)
	}

	return nil
}

// registerCoreComponents 注册核心组件
// 这个方法可以被扩展或替换为配置驱动的方式
func (c *Container) registerCoreComponents() error {
	// 注册问卷相关组件
	c.registerQuestionnaireComponents()

	// 注册用户相关组件
	c.registerUserComponents()

	// 可以继续添加其他模块的组件注册
	// c.registerScaleComponents()
	// c.registerResponseComponents()
	// c.registerEvaluationComponents()

	return nil
}

// initializeRouter 初始化路由配置器
func (c *Container) initializeRouter() error {
	// 获取所有处理器
	handlers, err := c.GetComponentsByType(HandlerType)
	if err != nil {
		return fmt.Errorf("failed to get handlers: %w", err)
	}

	// 创建路由配置器
	c.router = NewRouter()

	// 注册所有处理器的路由
	for name, handler := range handlers {
		if err := c.registerHandlerRoutes(name, handler); err != nil {
			return fmt.Errorf("failed to register routes for handler '%s': %w", name, err)
		}
	}

	return nil
}

// registerHandlerRoutes 注册处理器路由
func (c *Container) registerHandlerRoutes(name string, handler interface{}) error {
	// 使用反射或类型断言来注册不同类型的处理器路由
	// 这里可以根据实际需要实现路由注册逻辑
	switch name {
	case "questionnaireHandler":
		return c.router.RegisterQuestionnaireRoutes(handler)
	case "userHandler":
		return c.router.RegisterUserRoutes(handler)
	default:
		// 对于未知的处理器类型，可以尝试通用的路由注册方式
		return c.router.RegisterGenericRoutes(name, handler)
	}
}

// GetRouter 获取路由器引擎
func (c *Container) GetRouter() *gin.Engine {
	return c.router.GetEngine()
}

// ListComponents 列出所有已注册的组件
func (c *Container) ListComponents() map[string]ComponentDefinition {
	result := make(map[string]ComponentDefinition)
	for name, component := range c.components {
		result[name] = *component
	}
	return result
}

// Cleanup 清理资源
func (c *Container) Cleanup() {
	// 清理所有组件实例
	for _, component := range c.components {
		if component.Instance != nil {
			// 如果组件实现了 Cleanup 接口，调用其清理方法
			if cleaner, ok := component.Instance.(interface{ Cleanup() }); ok {
				cleaner.Cleanup()
			}
		}
	}

	// 清理数据库连接
	if c.mongoClient != nil {
		_ = c.mongoClient.Disconnect(nil)
	}
}

// registerQuestionnaireComponents 注册问卷相关组件
func (c *Container) registerQuestionnaireComponents() {
	// 注册 MySQL 问卷仓储
	c.RegisterComponent("mysqlQuestionnaireRepo", RepositoryType, func(container *Container) (interface{}, error) {
		return mysqlQuestionnaireAdapter.NewRepository(container.mysqlDB, nil, ""), nil
	})

	// 注册 MongoDB 文档仓储（如果可用）
	if c.mongoClient != nil {
		c.RegisterComponent("mongoDocumentRepo", RepositoryType, func(container *Container) (interface{}, error) {
			return mongoAdapter.NewQuestionnaireDocumentRepository(container.mongoClient, container.mongoDatabase), nil
		})

		// 注册组合仓储
		c.RegisterComponent("questionnaireRepo", RepositoryType, func(container *Container) (interface{}, error) {
			mysqlRepo, err := container.GetComponent("mysqlQuestionnaireRepo")
			if err != nil {
				return nil, err
			}
			mongoRepo, err := container.GetComponent("mongoDocumentRepo")
			if err != nil {
				return nil, err
			}
			return composite.NewQuestionnaireCompositeRepository(
				mysqlRepo.(storage.QuestionnaireRepository),
				mongoRepo.(storage.QuestionnaireDocumentRepository),
			), nil
		})
	} else {
		// 如果 MongoDB 不可用，问卷仓储直接使用 MySQL 实现
		c.RegisterComponent("questionnaireRepo", RepositoryType, func(container *Container) (interface{}, error) {
			return container.GetComponent("mysqlQuestionnaireRepo")
		})
	}

	// 注册问卷服务
	c.RegisterComponent("questionnaireService", ServiceType, func(container *Container) (interface{}, error) {
		repo, err := container.GetComponent("questionnaireRepo")
		if err != nil {
			return nil, err
		}
		return services.NewQuestionnaireService(repo.(storage.QuestionnaireRepository)), nil
	})

	// 注册问卷处理器
	c.RegisterComponent("questionnaireHandler", HandlerType, func(container *Container) (interface{}, error) {
		service, err := container.GetComponent("questionnaireService")
		if err != nil {
			return nil, err
		}
		return handlers.NewQuestionnaireHandler(service.(*services.QuestionnaireService)), nil
	})
}

// registerUserComponents 注册用户相关组件
func (c *Container) registerUserComponents() {
	// 注册用户仓储
	c.RegisterComponent("userRepo", RepositoryType, func(container *Container) (interface{}, error) {
		return mysqlUserAdapter.NewRepository(container.mysqlDB), nil
	})

	// 注册用户服务
	c.RegisterComponent("userService", ServiceType, func(container *Container) (interface{}, error) {
		repo, err := container.GetComponent("userRepo")
		if err != nil {
			return nil, err
		}
		return services.NewUserService(repo.(storage.UserRepository)), nil
	})

	// 注册用户处理器（如果存在的话）
	// c.RegisterComponent("userHandler", HandlerType, func(container *Container) (interface{}, error) {
	//     service, err := container.GetComponent("userService")
	//     if err != nil {
	//         return nil, err
	//     }
	//     return handlers.NewUserHandler(service.(*services.UserService)), nil
	// })
}
