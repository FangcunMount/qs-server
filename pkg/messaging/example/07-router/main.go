// Package main 演示 Router 的使用
// 统一路由管理、全局中间件、局部中间件
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

func main() {
	log.Println("=== Router 演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	// ========== 演示 1: 基础路由 ==========
	demonstrateBasicRouter(bus)
	time.Sleep(3 * time.Second)

	// ========== 演示 2: 全局中间件 ==========
	demonstrateGlobalMiddleware(bus)
	time.Sleep(3 * time.Second)

	// ========== 演示 3: 局部中间件 ==========
	demonstrateLocalMiddleware(bus)
	time.Sleep(3 * time.Second)

	// ========== 演示 4: 中间件执行顺序 ==========
	demonstrateMiddlewareOrder(bus)
	time.Sleep(3 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// ========== 演示 1: 基础路由 ==========

func demonstrateBasicRouter(bus messaging.EventBus) {
	log.Println("【演示 1】基础路由 - 注册多个处理器")

	logger := log.New(os.Stdout, "[Router] ", log.LstdFlags)
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	// 注册多个处理器
	router.AddHandler("user.created", "user-handler", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  → 处理用户创建: %s", string(msg.Payload))
		return msg.Ack()
	})

	router.AddHandler("order.created", "order-handler", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  → 处理订单创建: %s", string(msg.Payload))
		return msg.Ack()
	})

	router.AddHandler("payment.completed", "payment-handler", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  → 处理支付完成: %s", string(msg.Payload))
		return msg.Ack()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发布到不同 Topic
	log.Println("发布到不同 Topic...")
	publisher := bus.Publisher()

	events := []struct {
		topic string
		data  string
	}{
		{"user.created", "用户: Alice"},
		{"order.created", "订单: ORDER-001"},
		{"payment.completed", "支付: 100.00"},
	}

	for _, event := range events {
		publisher.Publish(context.Background(), event.topic, []byte(event.data))
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(time.Second)
	router.Stop()
	log.Println("\nRouter 统一管理了 3 个处理器")
}

// ========== 演示 2: 全局中间件 ==========

func demonstrateGlobalMiddleware(bus messaging.EventBus) {
	log.Println("\n【演示 2】全局中间件 - 应用到所有处理器")

	logger := log.New(os.Stdout, "[Global] ", log.LstdFlags)
	router := bus.Router()

	// 添加全局中间件
	router.AddMiddleware(messaging.RecoverMiddleware(logger))
	router.AddMiddleware(messaging.LoggerMiddleware(logger))
	router.AddMiddleware(messaging.TimeoutMiddleware(5 * time.Second))

	log.Println("全局中间件:")
	log.Println("  1. RecoverMiddleware（捕获 panic）")
	log.Println("  2. LoggerMiddleware（记录日志）")
	log.Println("  3. TimeoutMiddleware（超时控制）")

	// 注册处理器
	router.AddHandler("demo.global", "handler-1", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  → Handler 1 处理: %s", string(msg.Payload))
		return msg.Ack()
	})

	router.AddHandler("demo.global", "handler-2", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  → Handler 2 处理: %s", string(msg.Payload))
		return msg.Ack()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发布消息
	log.Println("发布消息（观察全局中间件）...")
	publisher := bus.Publisher()

	for i := 1; i <= 2; i++ {
		data := []byte(fmt.Sprintf("消息-%d", i))
		publisher.Publish(context.Background(), "demo.global", data)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(time.Second)
	router.Stop()
	log.Println("\n所有处理器都应用了全局中间件")
}

// ========== 演示 3: 局部中间件 ==========

func demonstrateLocalMiddleware(bus messaging.EventBus) {
	log.Println("\n【演示 3】局部中间件 - 只应用到特定处理器")

	logger := log.New(os.Stdout, "[Local] ", log.LstdFlags)
	router := bus.Router()

	// 全局中间件
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	// 处理器 1: 不需要重试
	router.AddHandler("demo.local", "handler-no-retry", func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → Handler（无重试）处理消息")
		return msg.Ack()
	})

	var attemptCount int32
	// 处理器 2: 需要重试
	router.AddHandlerWithMiddleware(
		"demo.local",
		"handler-with-retry",
		func(ctx context.Context, msg *messaging.Message) error {
			count := atomic.AddInt32(&attemptCount, 1)
			log.Printf("  → Handler（有重试）尝试 #%d", count)

			if count < 3 {
				return fmt.Errorf("模拟失败")
			}

			return msg.Ack()
		},
		messaging.RetryMiddleware(3, 500*time.Millisecond), // 局部中间件
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发布消息
	log.Println("发布消息（观察局部中间件）...")
	publisher := bus.Publisher()
	publisher.Publish(context.Background(), "demo.local", []byte("测试"))

	time.Sleep(3 * time.Second)
	router.Stop()
	log.Println("\n只有第二个处理器应用了 RetryMiddleware")
}

// ========== 演示 4: 中间件执行顺序 ==========

func demonstrateMiddlewareOrder(bus messaging.EventBus) {
	log.Println("\n【演示 4】中间件执行顺序 - 洋葱模型")

	router := bus.Router()

	// 自定义中间件：打印执行顺序
	createMiddleware := func(name string) messaging.Middleware {
		return func(next messaging.Handler) messaging.Handler {
			return func(ctx context.Context, msg *messaging.Message) error {
				log.Printf("  → [%s] 前置处理", name)
				err := next(ctx, msg)
				log.Printf("  ← [%s] 后置处理", name)
				return err
			}
		}
	}

	// 全局中间件
	router.AddMiddleware(createMiddleware("Global-1"))
	router.AddMiddleware(createMiddleware("Global-2"))

	// 局部中间件
	router.AddHandlerWithMiddleware(
		"demo.order",
		"order-handler",
		func(ctx context.Context, msg *messaging.Message) error {
			log.Println("  ★ [Handler] 执行业务逻辑")
			return msg.Ack()
		},
		createMiddleware("Local-1"),
		createMiddleware("Local-2"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发布消息（观察执行顺序）...")
	log.Println("期望顺序:")
	log.Println("  请求: Global-1 → Global-2 → Local-1 → Local-2 → Handler")
	log.Println("  响应: Handler → Local-2 → Local-1 → Global-2 → Global-1")

	publisher := bus.Publisher()
	publisher.Publish(context.Background(), "demo.order", []byte("测试消息"))

	time.Sleep(2 * time.Second)
	router.Stop()
}

// 核心知识点：
//
// 1. Router 的作用
//    • 统一管理多个订阅者
//    • 应用全局中间件
//    • 简化配置和生命周期管理
//
// 2. Router 接口
//    type MessageRouter interface {
//        AddHandler(topic, channel string, handler Handler)
//        AddHandlerWithMiddleware(topic, channel string, handler Handler, middlewares ...Middleware)
//        AddMiddleware(middleware Middleware)
//        Run(ctx context.Context)
//        Stop()
//    }
//
// 3. 全局中间件 vs 局部中间件
//    • 全局中间件: AddMiddleware() - 应用到所有处理器
//    • 局部中间件: AddHandlerWithMiddleware() - 只应用到特定处理器
//
// 4. 中间件执行顺序（洋葱模型）
//    请求流: 外层 → 内层 → Handler
//    响应流: Handler → 内层 → 外层
//
//    顺序: Global-1 → Global-2 → Local-1 → Local-2 → Handler
//
// 5. 推荐的中间件顺序
//    1. RecoverMiddleware（最外层，捕获 panic）
//    2. LoggerMiddleware（记录日志）
//    3. TracingMiddleware（追踪）
//    4. MetricsMiddleware（指标）
//    5. TimeoutMiddleware（超时控制）
//    6. RetryMiddleware（重试）
//    7. 业务中间件（认证、限流等）
//    8. Handler（最内层，业务逻辑）
//
// 应用场景：
//
// 1. 统一路由管理
//    • 微服务中有多个事件处理器
//    • 需要统一的中间件配置
//    • 简化生命周期管理
//
// 2. 全局中间件
//    • 所有处理器都需要的功能
//    • 日志、监控、恢复、追踪
//    • 提升代码复用
//
// 3. 局部中间件
//    • 只有特定处理器需要的功能
//    • 重试、超时、限流
//    • 灵活控制
//
// 4. 中间件组合
//    • 不同处理器需要不同的中间件组合
//    • 灵活配置
//
// Router vs 直接 Subscribe：
//
// 直接 Subscribe:
// ```go
// subscriber.Subscribe(ctx, "topic", "channel", handler)
// ```
// • 简单直接
// • 每个订阅单独管理
// • 不支持全局中间件
//
// 使用 Router:
// ```go
// router := bus.Router()
// router.AddMiddleware(...)  // 全局中间件
// router.AddHandler("topic", "channel", handler)
// router.Run(ctx)
// ```
// • 统一管理
// • 支持全局中间件
// • 简化生命周期
//
// 最佳实践：
// ✅ 使用 Router 统一管理所有订阅者
// ✅ 全局中间件放在 AddMiddleware()
// ✅ 局部中间件放在 AddHandlerWithMiddleware()
// ✅ 按推荐顺序添加中间件
// ✅ RecoverMiddleware 放在最外层
// ✅ 使用 Context 控制 Router 生命周期
// ✅ 调用 Stop() 优雅停止
//
// 注意事项：
// ⚠️ 全局中间件会应用到所有处理器（注意性能）
// ⚠️ 中间件顺序很重要（影响执行逻辑）
// ⚠️ 不要在中间件中做耗时操作
// ⚠️ 中间件要处理错误
// ⚠️ Stop() 后不会处理新消息
// ⚠️ 多次调用 AddMiddleware() 会累加
//
// 性能考虑：
// • 中间件有性能开销（每层 ~0.1ms）
// • 不要添加过多中间件
// • 耗时操作放在异步中间件
// • 使用性能分析工具优化
//
// 错误处理：
// • 中间件返回 error 会中断链
// • 使用 RecoverMiddleware 捕获 panic
// • 记录错误日志
// • 返回合适的错误信息
//
// 测试建议：
// • 测试中间件执行顺序
// • 测试全局和局部中间件
// • 测试错误处理
// • 测试 panic 恢复
// • 测试生命周期（Start/Stop）
