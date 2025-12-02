// Package main 演示基础中间件的使用
// Logger、Retry、Timeout、Recover
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

func main() {
	log.Println("=== 基础中间件演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Middleware] ", log.LstdFlags)

	// ========== 演示 1: Logger 中间件 ==========
	demonstrateLogger(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 2: Retry 中间件 ==========
	demonstrateRetry(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 3: Timeout 中间件 ==========
	demonstrateTimeout(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 4: Recover 中间件 ==========
	demonstrateRecover(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 5: 组合使用 ==========
	demonstrateCombined(bus, logger)
	time.Sleep(5 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// demonstrateLogger 演示日志中间件
func demonstrateLogger(bus messaging.EventBus, logger *log.Logger) {
	log.Println("【演示 1】Logger 中间件 - 记录消息处理日志")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	router.AddHandler("demo.logger", "logger-demo", func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → 处理消息中...")
		time.Sleep(100 * time.Millisecond)
		return msg.Ack()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发布消息
	bus.Publisher().Publish(context.Background(), "demo.logger", []byte("测试日志"))
	time.Sleep(time.Second)
	router.Stop()
}

// demonstrateRetry 演示重试中间件
func demonstrateRetry(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 2】Retry 中间件 - 自动重试失败的消息")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	attemptCount := 0
	handler := func(ctx context.Context, msg *messaging.Message) error {
		attemptCount++
		log.Printf("  → 第 %d 次尝试处理", attemptCount)

		// 前 2 次失败，第 3 次成功
		if attemptCount < 3 {
			return errors.New("模拟失败")
		}

		log.Println("  ✅ 处理成功")
		return msg.Ack()
	}

	// 添加重试中间件：最多重试 3 次，延迟 500ms
	router.AddHandlerWithMiddleware(
		"demo.retry",
		"retry-demo",
		handler,
		messaging.RetryMiddleware(3, 500*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发布消息（观察重试过程）...")
	bus.Publisher().Publish(context.Background(), "demo.retry", []byte("测试重试"))

	time.Sleep(3 * time.Second)
	router.Stop()
}

// demonstrateTimeout 演示超时中间件
func demonstrateTimeout(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 3】Timeout 中间件 - 限制处理时间")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	slowHandler := func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → 开始处理（模拟慢操作）...")
		time.Sleep(3 * time.Second) // 故意超时
		log.Println("  → 处理完成")
		return msg.Ack()
	}

	// 添加超时中间件：1 秒超时
	router.AddHandlerWithMiddleware(
		"demo.timeout",
		"timeout-demo",
		slowHandler,
		messaging.TimeoutMiddleware(time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发布消息（观察超时处理）...")
	bus.Publisher().Publish(context.Background(), "demo.timeout", []byte("测试超时"))

	time.Sleep(2 * time.Second)
	router.Stop()
}

// demonstrateRecover 演示恢复中间件
func demonstrateRecover(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 4】Recover 中间件 - 捕获 Panic")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	panicHandler := func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → 开始处理...")
		panic("模拟程序崩溃！") // 故意触发 panic
	}

	// 添加恢复中间件：捕获 panic
	router.AddHandlerWithMiddleware(
		"demo.recover",
		"recover-demo",
		panicHandler,
		messaging.RecoverMiddleware(logger),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发布消息（观察 panic 恢复）...")
	bus.Publisher().Publish(context.Background(), "demo.recover", []byte("测试恢复"))

	time.Sleep(time.Second)
	router.Stop()
	log.Println("  ✅ 程序没有崩溃，继续运行")
}

// demonstrateCombined 演示组合使用多个中间件
func demonstrateCombined(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 5】组合中间件 - 最佳实践")

	router := bus.Router()

	// 全局中间件（应用到所有处理器）
	router.AddMiddleware(messaging.RecoverMiddleware(logger)) // 最外层：捕获 panic
	router.AddMiddleware(messaging.LoggerMiddleware(logger))  // 日志记录

	// 局部中间件（只应用到特定处理器）
	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → 执行业务逻辑...")
		time.Sleep(500 * time.Millisecond)
		return msg.Ack()
	}

	router.AddHandlerWithMiddleware(
		"demo.combined",
		"combined-demo",
		handler,
		messaging.TimeoutMiddleware(2*time.Second),         // 超时 2 秒
		messaging.RetryMiddleware(2, 500*time.Millisecond), // 重试 2 次
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发布消息（观察中间件链执行顺序）...")
	log.Println("中间件顺序：Recover → Logger → Timeout → Retry → Handler")

	bus.Publisher().Publish(context.Background(), "demo.combined", []byte("测试组合"))

	time.Sleep(2 * time.Second)
	router.Stop()
}

// 核心知识点：
//
// 1. Logger 中间件
//    - 记录消息处理的开始、结束、耗时
//    - 记录成功和失败情况
//    - 用于调试和监控
//
// 2. Retry 中间件
//    - 自动重试失败的消息
//    - 支持指数退避（避免雪崩）
//    - 设置最大重试次数
//
// 3. Timeout 中间件
//    - 限制消息处理时间
//    - 防止慢操作阻塞队列
//    - 超时后返回错误
//
// 4. Recover 中间件
//    - 捕获 panic，防止程序崩溃
//    - 将 panic 转换为 error
//    - 应该放在最外层
//
// 5. 中间件执行顺序（洋葱模型）
//    请求：Recover → Logger → Timeout → Retry → Handler
//    响应：Handler → Retry → Timeout → Logger → Recover
//
// 推荐的中间件顺序：
// 1. RecoverMiddleware（最外层）
// 2. LoggerMiddleware
// 3. TracingMiddleware
// 4. TimeoutMiddleware
// 5. RetryMiddleware
// 6. 业务中间件（认证、限流等）
// 7. Handler（最内层）
//
// 最佳实践：
// ✅ 全局中间件放在 router.AddMiddleware()
// ✅ 局部中间件放在 AddHandlerWithMiddleware()
// ✅ Recover 中间件放在最外层
// ✅ 根据需要组合使用，不要过度使用
// ✅ 注意中间件的执行顺序
