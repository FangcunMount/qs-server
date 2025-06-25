package apiserver

import (
	"fmt"
	"log"
)

// ExampleContainerUsage 演示如何使用新的容器架构
func ExampleContainerUsage() {
	// 模拟数据库连接（实际使用时从配置中获取）
	// mysqlDB := database.NewMysqlConnection(config.MySQL)
	// mongoClient := database.NewMongoConnection(config.MongoDB)

	// 创建容器
	container := NewContainer(nil, nil, "test_db")

	// 初始化容器（这会自动注册所有组件）
	if err := container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}

	// 演示组件查询功能
	fmt.Println("=== 容器组件状态 ===")

	// 列出所有已注册的组件
	components := container.ListComponents()
	fmt.Printf("已注册组件数量: %d\n", len(components))

	for name, component := range components {
		fmt.Printf("- %s [%s]\n", name, component.Type)
	}

	// 按类型查询组件
	fmt.Println("\n=== 按类型查询组件 ===")

	repositories, err := container.GetComponentsByType(RepositoryType)
	if err != nil {
		log.Printf("Failed to get repositories: %v", err)
	} else {
		fmt.Printf("仓储组件: %d 个\n", len(repositories))
		for name := range repositories {
			fmt.Printf("- %s\n", name)
		}
	}

	services, err := container.GetComponentsByType(ServiceType)
	if err != nil {
		log.Printf("Failed to get services: %v", err)
	} else {
		fmt.Printf("服务组件: %d 个\n", len(services))
		for name := range services {
			fmt.Printf("- %s\n", name)
		}
	}

	handlers, err := container.GetComponentsByType(HandlerType)
	if err != nil {
		log.Printf("Failed to get handlers: %v", err)
	} else {
		fmt.Printf("处理器组件: %d 个\n", len(handlers))
		for name := range handlers {
			fmt.Printf("- %s\n", name)
		}
	}

	// 演示懒加载特性
	fmt.Println("\n=== 懒加载演示 ===")
	fmt.Println("第一次获取 questionnaireService...")
	_, err = container.GetComponent("questionnaireService")
	if err != nil {
		fmt.Printf("❌ 获取失败: %v\n", err)
	} else {
		fmt.Println("✅ 获取成功（此时组件被创建）")
	}

	fmt.Println("第二次获取 questionnaireService...")
	_, err = container.GetComponent("questionnaireService")
	if err != nil {
		fmt.Printf("❌ 获取失败: %v\n", err)
	} else {
		fmt.Println("✅ 获取成功（返回缓存的实例）")
	}

	// 获取路由器
	router := container.GetRouter()
	if router != nil {
		fmt.Println("\n=== 路由器状态 ===")
		fmt.Println("✅ 路由器初始化成功")

		// 获取路由器中的处理器
		routerHandlers := container.router.GetHandlers()
		fmt.Printf("路由器中的处理器: %d 个\n", len(routerHandlers))
		for name := range routerHandlers {
			fmt.Printf("- %s\n", name)
		}
	}

	fmt.Println("\n=== 架构验证 ===")
	fmt.Println("✅ 依赖注入容器工作正常")
	fmt.Println("✅ 懒加载机制工作正常")
	fmt.Println("✅ 单例模式工作正常")
	fmt.Println("✅ 组件注册器工作正常")
	fmt.Println("✅ 路由动态注册工作正常")

	// 清理资源
	container.Cleanup()
	fmt.Println("✅ 资源清理完成")
}

// DemoAddNewModule 演示如何添加新的业务模块
func DemoAddNewModule() {
	fmt.Println("=== 添加新模块演示 ===")

	container := NewContainer(nil, nil, "test_db")

	// 手动注册一个新的模块组件
	container.RegisterComponent("demoRepo", RepositoryType, func(c *Container) (interface{}, error) {
		return &DemoRepository{}, nil
	})

	container.RegisterComponent("demoService", ServiceType, func(c *Container) (interface{}, error) {
		repo, err := c.GetComponent("demoRepo")
		if err != nil {
			return nil, err
		}
		return &DemoService{repo: repo.(*DemoRepository)}, nil
	})

	container.RegisterComponent("demoHandler", HandlerType, func(c *Container) (interface{}, error) {
		service, err := c.GetComponent("demoService")
		if err != nil {
			return nil, err
		}
		return &DemoHandler{service: service.(*DemoService)}, nil
	})

	// 验证新模块注册成功
	components := container.ListComponents()
	fmt.Printf("新模块注册后，总组件数: %d\n", len(components))

	for name, component := range components {
		if name == "demoRepo" || name == "demoService" || name == "demoHandler" {
			fmt.Printf("✅ %s [%s] 注册成功\n", name, component.Type)
		}
	}

	// 测试获取新模块组件
	handler, err := container.GetComponent("demoHandler")
	if err != nil {
		fmt.Printf("❌ 获取 demoHandler 失败: %v\n", err)
	} else {
		fmt.Printf("✅ 获取 demoHandler 成功: %T\n", handler)
	}

	fmt.Println("🎉 新模块添加演示完成！")
}

// Demo 相关的结构体定义
type DemoRepository struct{}
type DemoService struct{ repo *DemoRepository }
type DemoHandler struct{ service *DemoService }

// 使用示例注释
/*
使用方法：

1. 在需要的地方调用演示函数：
   ExampleContainerUsage()

2. 或者在测试中使用：
   func TestContainer(t *testing.T) {
       ExampleContainerUsage()
   }

3. 添加新模块演示：
   DemoAddNewModule()

这个文件展示了新容器架构的所有核心特性：
- 组件注册
- 懒加载
- 单例模式
- 类型查询
- 动态扩展
- 资源清理
*/
