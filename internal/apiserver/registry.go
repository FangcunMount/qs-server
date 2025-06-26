package apiserver

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers/user"
)

// ComponentType 组件类型
type ComponentType string

const (
	// RepositoryType 仓储层组件类型
	RepositoryType ComponentType = "repository"
	// ServiceType 服务层组件类型
	ServiceType ComponentType = "service"
	// HandlerType 处理器组件类型
	HandlerType ComponentType = "handler"
)

// AutoDiscoveryFactory 自动发现组件工厂函数类型
type AutoDiscoveryFactory func(container *AutoDiscoveryContainer) (interface{}, error)

// ComponentMeta 组件元数据
type ComponentMeta struct {
	Name          string               // 组件名称，如 "user", "questionnaire"
	Type          ComponentType        // 组件类型
	Factory       AutoDiscoveryFactory // 工厂函数
	Dependencies  []string             // 依赖的组件名称
	InterfaceType reflect.Type         // 实现的接口类型
	ConcreteType  reflect.Type         // 具体实现类型
}

// GlobalRegistry 全局组件注册表
type GlobalRegistry struct {
	mu         sync.RWMutex
	components map[string]*ComponentMeta
}

var globalRegistry = &GlobalRegistry{
	components: make(map[string]*ComponentMeta),
}

// RegisterRepository 注册存储库组件
func RegisterRepository(name string, factory AutoDiscoveryFactory, interfaceType reflect.Type, dependencies ...string) {
	globalRegistry.register(&ComponentMeta{
		Name:          name,
		Type:          RepositoryType,
		Factory:       factory,
		Dependencies:  dependencies,
		InterfaceType: interfaceType,
	})
}

// RegisterService 注册服务组件
func RegisterService(name string, factory AutoDiscoveryFactory, interfaceType reflect.Type, dependencies ...string) {
	globalRegistry.register(&ComponentMeta{
		Name:          name,
		Type:          ServiceType,
		Factory:       factory,
		Dependencies:  dependencies,
		InterfaceType: interfaceType,
	})
}

// RegisterHandler 注册处理器组件
func RegisterHandler(name string, factory AutoDiscoveryFactory, dependencies ...string) {
	globalRegistry.register(&ComponentMeta{
		Name:          name,
		Type:          HandlerType,
		Factory:       factory,
		Dependencies:  dependencies,
		InterfaceType: reflect.TypeOf((*handlers.Handler)(nil)).Elem(),
	})
}

// register 内部注册方法
func (r *GlobalRegistry) register(meta *ComponentMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.components[fmt.Sprintf("%s:%s", meta.Type, meta.Name)] = meta

	fmt.Printf("📝 Registered %s component: %s (dependencies: %v)\n",
		meta.Type, meta.Name, meta.Dependencies)
}

// GetComponents 获取指定类型的所有组件
func (r *GlobalRegistry) GetComponents(componentType ComponentType) map[string]*ComponentMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*ComponentMeta)
	for _, meta := range r.components {
		if meta.Type == componentType {
			result[meta.Name] = meta
		}
	}
	return result
}

// GetAllComponents 获取所有组件
func (r *GlobalRegistry) GetAllComponents() map[string]*ComponentMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*ComponentMeta)
	for key, meta := range r.components {
		result[key] = meta
	}
	return result
}

// SortByDependencies 根据依赖关系排序组件
func (r *GlobalRegistry) SortByDependencies(components map[string]*ComponentMeta) ([]*ComponentMeta, error) {
	var sorted []*ComponentMeta
	visited := make(map[string]bool)
	inProgress := make(map[string]bool)

	var visit func(name string) error
	visit = func(name string) error {
		if inProgress[name] {
			return fmt.Errorf("circular dependency detected: %s", name)
		}
		if visited[name] {
			return nil
		}

		// 找到对应的组件
		var component *ComponentMeta
		for _, meta := range components {
			if meta.Name == name {
				component = meta
				break
			}
		}
		if component == nil {
			return fmt.Errorf("component not found: %s", name)
		}

		inProgress[name] = true

		// 递归访问依赖
		for _, dep := range component.Dependencies {
			if err := visit(dep); err != nil {
				return err
			}
		}

		inProgress[name] = false
		visited[name] = true
		sorted = append(sorted, component)
		return nil
	}

	// 访问所有组件
	for _, meta := range components {
		if !visited[meta.Name] {
			if err := visit(meta.Name); err != nil {
				return nil, err
			}
		}
	}

	return sorted, nil
}

// AutoDiscoveryContainer 自动发现容器
type AutoDiscoveryContainer struct {
	// 外部依赖（基础设施）
	mysqlDB       *gorm.DB
	mongoClient   *mongo.Client
	mongoDatabase string

	// 已实例化的组件
	repositories map[string]interface{}
	services     map[string]interface{}
	handlers     map[string]handlers.Handler

	// 集中的路由管理器
	router    *Router
	ginEngine *gin.Engine
}

// NewAutoDiscoveryContainer 创建自动发现容器
func NewAutoDiscoveryContainer(mysqlDB *gorm.DB, mongoClient *mongo.Client, mongoDatabase string) *AutoDiscoveryContainer {
	return &AutoDiscoveryContainer{
		mysqlDB:       mysqlDB,
		mongoClient:   mongoClient,
		mongoDatabase: mongoDatabase,
		repositories:  make(map[string]interface{}),
		services:      make(map[string]interface{}),
		handlers:      make(map[string]handlers.Handler),
		router:        NewRouter(),
		ginEngine:     gin.New(),
	}
}

// Initialize 自动发现并初始化所有组件
func (c *AutoDiscoveryContainer) Initialize() error {
	fmt.Println("🚀 Starting automatic component discovery and registration...")

	// 1. 按依赖顺序初始化存储库
	if err := c.initializeRepositories(); err != nil {
		return fmt.Errorf("failed to initialize repositories: %w", err)
	}

	// 2. 按依赖顺序初始化服务
	if err := c.initializeServices(); err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}

	// 3. 按依赖顺序初始化处理器
	if err := c.initializeHandlers(); err != nil {
		return fmt.Errorf("failed to initialize handlers: %w", err)
	}

	// 4. 配置具体的handler到路由管理器
	if err := c.configureRouter(); err != nil {
		return fmt.Errorf("failed to configure router: %w", err)
	}

	// 5. 注册所有路由
	if err := c.initializeRoutes(); err != nil {
		return fmt.Errorf("failed to initialize routes: %w", err)
	}

	fmt.Println("✅ Automatic component discovery completed successfully!")
	return nil
}

// initializeRepositories 初始化存储库组件
func (c *AutoDiscoveryContainer) initializeRepositories() error {
	components := globalRegistry.GetComponents(RepositoryType)
	sorted, err := globalRegistry.SortByDependencies(components)
	if err != nil {
		return err
	}

	fmt.Printf("📦 Discovered %d repository components\n", len(sorted))

	for _, meta := range sorted {
		instance, err := meta.Factory(c)
		if err != nil {
			return fmt.Errorf("failed to create repository %s: %w", meta.Name, err)
		}

		c.repositories[meta.Name] = instance
		fmt.Printf("  ✓ Initialized repository: %s\n", meta.Name)
	}

	return nil
}

// initializeServices 初始化服务组件
func (c *AutoDiscoveryContainer) initializeServices() error {
	components := globalRegistry.GetComponents(ServiceType)
	sorted, err := globalRegistry.SortByDependencies(components)
	if err != nil {
		return err
	}

	fmt.Printf("🔧 Discovered %d service components\n", len(sorted))

	for _, meta := range sorted {
		instance, err := meta.Factory(c)
		if err != nil {
			return fmt.Errorf("failed to create service %s: %w", meta.Name, err)
		}

		c.services[meta.Name] = instance
		fmt.Printf("  ✓ Initialized service: %s\n", meta.Name)
	}

	return nil
}

// initializeHandlers 初始化处理器组件
func (c *AutoDiscoveryContainer) initializeHandlers() error {
	components := globalRegistry.GetComponents(HandlerType)
	sorted, err := globalRegistry.SortByDependencies(components)
	if err != nil {
		return err
	}

	fmt.Printf("🌐 Discovered %d handler components\n", len(sorted))

	for _, meta := range sorted {
		instance, err := meta.Factory(c)
		if err != nil {
			return fmt.Errorf("failed to create handler %s: %w", meta.Name, err)
		}

		handler, ok := instance.(handlers.Handler)
		if !ok {
			return fmt.Errorf("handler %s does not implement handlers.Handler interface", meta.Name)
		}

		c.handlers[meta.Name] = handler
		fmt.Printf("  ✓ Initialized handler: %s\n", meta.Name)
	}

	return nil
}

// configureRouter 配置路由管理器
func (c *AutoDiscoveryContainer) configureRouter() error {
	fmt.Println("🔧 Configuring centralized router...")

	// 设置容器引用（用于健康检查）
	c.router.SetContainer(c)

	// 设置用户处理器
	if userHandlerInterface, exists := c.handlers["user"]; exists {
		if userHandler, ok := userHandlerInterface.(*user.Handler); ok {
			c.router.SetUserHandler(userHandler)
			fmt.Printf("  ✓ Configured user handler in router\n")
		} else {
			return fmt.Errorf("user handler is not of expected type")
		}
	}

	// 设置问卷处理器
	if questionnaireHandlerInterface, exists := c.handlers["questionnaire"]; exists {
		if questionnaireHandler, ok := questionnaireHandlerInterface.(*questionnaire.Handler); ok {
			c.router.SetQuestionnaireHandler(questionnaireHandler)
			fmt.Printf("  ✓ Configured questionnaire handler in router\n")
		} else {
			return fmt.Errorf("questionnaire handler is not of expected type")
		}
	}

	return nil
}

// initializeRoutes 初始化路由
func (c *AutoDiscoveryContainer) initializeRoutes() error {
	fmt.Println("🔗 Registering routes via centralized router...")

	// 使用集中的路由管理器注册所有路由
	c.router.RegisterRoutes(c.ginEngine)

	fmt.Println("✅ Route registration completed")
	return nil
}

// GetRouter 获取路由器
func (c *AutoDiscoveryContainer) GetRouter() *gin.Engine {
	return c.ginEngine
}

// GetMySQLDB 获取MySQL数据库连接
func (c *AutoDiscoveryContainer) GetMySQLDB() *gorm.DB {
	return c.mysqlDB
}

// GetMongoClient 获取MongoDB客户端
func (c *AutoDiscoveryContainer) GetMongoClient() *mongo.Client {
	return c.mongoClient
}

// GetMongoDatabase 获取MongoDB数据库名
func (c *AutoDiscoveryContainer) GetMongoDatabase() string {
	return c.mongoDatabase
}

// GetRepository 获取存储库实例
func (c *AutoDiscoveryContainer) GetRepository(name string) (interface{}, bool) {
	repo, exists := c.repositories[name]
	return repo, exists
}

// GetService 获取服务实例
func (c *AutoDiscoveryContainer) GetService(name string) (interface{}, bool) {
	service, exists := c.services[name]
	return service, exists
}

// GetHandler 获取处理器实例
func (c *AutoDiscoveryContainer) GetHandler(name string) (handlers.Handler, bool) {
	handler, exists := c.handlers[name]
	return handler, exists
}

// PrintRegistryInfo 打印注册表信息
func (c *AutoDiscoveryContainer) PrintRegistryInfo() {
	fmt.Println("\n📋 Component Registry Summary:")

	allComponents := globalRegistry.GetAllComponents()
	componentTypes := []ComponentType{RepositoryType, ServiceType, HandlerType}

	for _, componentType := range componentTypes {
		fmt.Printf("\n%s Components:\n", componentType)
		for key, meta := range allComponents {
			if meta.Type == componentType {
				fmt.Printf("  • %s (key: %s, deps: %v)\n", meta.Name, key, meta.Dependencies)
			}
		}
	}
	fmt.Println()
}

// 辅助方法
func (c *AutoDiscoveryContainer) getRegisteredRepositories() []string {
	var names []string
	for name := range c.repositories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *AutoDiscoveryContainer) getRegisteredServices() []string {
	var names []string
	for name := range c.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *AutoDiscoveryContainer) getRegisteredHandlers() []string {
	var names []string
	for name := range c.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Cleanup 清理资源
func (c *AutoDiscoveryContainer) Cleanup() {
	fmt.Println("🧹 Cleaning up auto-discovery container components...")

	// 清理处理器
	for name := range c.handlers {
		delete(c.handlers, name)
	}

	// 清理服务
	for name := range c.services {
		delete(c.services, name)
	}

	// 清理存储库
	for name := range c.repositories {
		delete(c.repositories, name)
	}

	fmt.Println("✅ Auto-discovery container cleanup completed")
}
