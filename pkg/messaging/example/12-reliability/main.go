// Package main 演示可靠性保障实践
// 错误处理、重试策略、熔断降级、消息幂等性
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

func main() {
	log.Println("=== 可靠性保障演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Reliability] ", log.LstdFlags)

	// ========== 演示 1: 错误处理 ==========
	demonstrateErrorHandling(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 2: 重试策略 ==========
	demonstrateRetryStrategy(bus, logger)
	time.Sleep(5 * time.Second)

	// ========== 演示 3: 熔断降级 ==========
	demonstrateCircuitBreaker(bus, logger)
	time.Sleep(5 * time.Second)

	// ========== 演示 4: 消息幂等性 ==========
	demonstrateIdempotency(bus, logger)
	time.Sleep(3 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// ========== 演示 1: 错误处理 ==========

// 定义业务错误类型
var (
	ErrTemporary = errors.New("临时错误（可重试）")
	ErrPermanent = errors.New("永久错误（不可重试）")
	ErrTimeout   = errors.New("超时错误")
)

// ErrorClassifier 错误分类器
func ErrorClassifier(err error) (isRetryable bool, reason string) {
	if err == nil {
		return false, ""
	}

	// 根据错误类型判断是否可重试
	switch {
	case errors.Is(err, ErrPermanent):
		return false, "永久性错误，不应重试"
	case errors.Is(err, ErrTimeout):
		return true, "超时错误，可以重试"
	case errors.Is(err, ErrTemporary):
		return true, "临时性错误，可以重试"
	default:
		return false, "未知错误，默认不重试"
	}
}

// SmartRetryMiddleware 智能重试中间件（基于错误类型）
func SmartRetryMiddleware(maxRetries int, delay time.Duration) messaging.Middleware {
	return func(next messaging.Handler) messaging.Handler {
		return func(ctx context.Context, msg *messaging.Message) error {
			var lastErr error

			for attempt := 1; attempt <= maxRetries; attempt++ {
				lastErr = next(ctx, msg)

				if lastErr == nil {
					return nil
				}

				// 判断错误是否可重试
				retryable, reason := ErrorClassifier(lastErr)
				log.Printf("  → 第 %d 次尝试失败: %v (%s)", attempt, lastErr, reason)

				if !retryable {
					log.Println("  ❌ 不可重试的错误，停止重试")
					return lastErr
				}

				if attempt < maxRetries {
					log.Printf("  → 等待 %v 后重试...", delay)
					time.Sleep(delay)
				}
			}

			log.Printf("  ❌ 重试次数已用尽")
			return lastErr
		}
	}
}

func demonstrateErrorHandling(bus messaging.EventBus, logger *log.Logger) {
	log.Println("【演示 1】错误处理 - 区分可重试和不可重试错误")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	// 测试不同类型的错误
	errorTypes := []error{ErrTemporary, ErrPermanent, ErrTimeout}
	currentError := 0

	handler := func(ctx context.Context, msg *messaging.Message) error {
		err := errorTypes[currentError%len(errorTypes)]
		currentError++
		return err
	}

	router.AddHandlerWithMiddleware(
		"demo.error",
		"error-demo",
		handler,
		SmartRetryMiddleware(3, 500*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("测试不同错误类型的处理...")

	for i := 1; i <= 3; i++ {
		log.Printf("发送消息 #%d", i)
		bus.Publisher().Publish(context.Background(), "demo.error", []byte(fmt.Sprintf("测试-%d", i)))
		time.Sleep(2 * time.Second)
	}

	router.Stop()
}

// ========== 演示 2: 重试策略 ==========

// ExponentialBackoff 指数退避重试
func ExponentialBackoff(baseDelay time.Duration, maxDelay time.Duration, factor float64) func(int) time.Duration {
	return func(attempt int) time.Duration {
		delay := float64(baseDelay) * (factor * float64(attempt-1))
		if delay > float64(maxDelay) {
			delay = float64(maxDelay)
		}
		// 添加抖动（jitter）避免雷鸣群羊效应
		jitter := time.Duration(rand.Int63n(int64(delay / 10)))
		return time.Duration(delay) + jitter
	}
}

// ExponentialRetryMiddleware 指数退避重试中间件
func ExponentialRetryMiddleware(maxRetries int) messaging.Middleware {
	backoff := ExponentialBackoff(100*time.Millisecond, 5*time.Second, 2.0)

	return func(next messaging.Handler) messaging.Handler {
		return func(ctx context.Context, msg *messaging.Message) error {
			for attempt := 1; attempt <= maxRetries; attempt++ {
				err := next(ctx, msg)

				if err == nil {
					return nil
				}

				if attempt < maxRetries {
					delay := backoff(attempt)
					log.Printf("  → 第 %d 次重试，延迟 %v", attempt, delay)
					time.Sleep(delay)
				}
			}

			return errors.New("达到最大重试次数")
		}
	}
}

func demonstrateRetryStrategy(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 2】重试策略 - 指数退避")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	attemptCount := 0
	handler := func(ctx context.Context, msg *messaging.Message) error {
		attemptCount++
		log.Printf("  → 处理尝试 #%d", attemptCount)

		// 前 3 次失败
		if attemptCount < 4 {
			return errors.New("模拟失败")
		}

		log.Println("  ✅ 处理成功")
		return msg.Ack()
	}

	router.AddHandlerWithMiddleware(
		"demo.retry",
		"retry-demo",
		handler,
		ExponentialRetryMiddleware(5),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送消息（观察指数退避）...")
	log.Println("重试间隔: 100ms → 200ms → 400ms → 800ms → 1600ms")

	bus.Publisher().Publish(context.Background(), "demo.retry", []byte("测试"))

	time.Sleep(5 * time.Second)
	router.Stop()
}

// ========== 演示 3: 熔断降级 ==========

// AdvancedCircuitBreaker 高级熔断器
type AdvancedCircuitBreaker struct {
	mu           sync.Mutex
	state        string // "closed", "open", "half-open"
	failures     int
	successes    int
	threshold    int
	timeout      time.Duration
	openTime     time.Time
	fallbackFunc func(context.Context, *messaging.Message) error
}

func NewAdvancedCircuitBreaker(threshold int, timeout time.Duration, fallback func(context.Context, *messaging.Message) error) *AdvancedCircuitBreaker {
	return &AdvancedCircuitBreaker{
		state:        "closed",
		threshold:    threshold,
		timeout:      timeout,
		fallbackFunc: fallback,
	}
}

func (cb *AdvancedCircuitBreaker) Execute(ctx context.Context, msg *messaging.Message, handler messaging.Handler) error {
	cb.mu.Lock()
	state := cb.state

	// 检查是否可以尝试恢复
	if state == "open" && time.Since(cb.openTime) >= cb.timeout {
		log.Println("  🔄 熔断器进入半开状态，尝试恢复...")
		cb.state = "half-open"
		cb.successes = 0
		state = "half-open"
	}

	// 如果熔断器打开，执行降级逻辑
	if state == "open" {
		cb.mu.Unlock()
		log.Println("  ⚡ 熔断器已打开，执行降级逻辑")
		return cb.fallbackFunc(ctx, msg)
	}

	cb.mu.Unlock()

	// 执行正常处理
	err := handler(ctx, msg)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		log.Printf("  ❌ 失败次数: %d/%d", cb.failures, cb.threshold)

		if cb.failures >= cb.threshold {
			log.Println("  ⚡ 触发熔断！")
			cb.state = "open"
			cb.openTime = time.Now()
		}
	} else {
		if cb.state == "half-open" {
			cb.successes++
			log.Printf("  ✅ 半开状态成功次数: %d", cb.successes)

			if cb.successes >= 2 {
				log.Println("  ✅ 熔断器关闭，恢复正常")
				cb.state = "closed"
				cb.failures = 0
			}
		} else {
			cb.failures = 0
		}
	}

	return err
}

func demonstrateCircuitBreaker(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 3】熔断降级 - 三态熔断器")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	failureMode := true
	var processCount int32

	handler := func(ctx context.Context, msg *messaging.Message) error {
		count := atomic.AddInt32(&processCount, 1)

		// 前 3 次失败，触发熔断
		if failureMode && count <= 3 {
			return errors.New("服务故障")
		}

		// 后续恢复正常
		failureMode = false
		return msg.Ack()
	}

	// 降级处理函数
	fallback := func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → 执行降级逻辑（返回缓存数据）")
		return msg.Ack()
	}

	cb := NewAdvancedCircuitBreaker(3, 3*time.Second, fallback)

	wrappedHandler := func(ctx context.Context, msg *messaging.Message) error {
		return cb.Execute(ctx, msg, handler)
	}

	router.AddHandler("demo.breaker", "breaker-demo", wrappedHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送消息（观察熔断器状态变化）...")

	for i := 1; i <= 8; i++ {
		log.Printf("发送消息 #%d", i)
		bus.Publisher().Publish(context.Background(), "demo.breaker", []byte(fmt.Sprintf("消息-%d", i)))
		time.Sleep(800 * time.Millisecond)

		if i == 3 {
			log.Println("\n→ 熔断器应该已打开")
		}
		if i == 6 {
			log.Println("\n→ 等待熔断器超时，进入半开状态")
		}
	}

	router.Stop()
}

// ========== 演示 4: 消息幂等性 ==========

// IdempotencyStore 幂等性存储
type IdempotencyStore struct {
	mu        sync.RWMutex
	processed map[string]time.Time
	ttl       time.Duration
}

func NewIdempotencyStore(ttl time.Duration) *IdempotencyStore {
	store := &IdempotencyStore{
		processed: make(map[string]time.Time),
		ttl:       ttl,
	}

	// 定期清理过期记录
	go func() {
		ticker := time.NewTicker(ttl)
		defer ticker.Stop()

		for range ticker.C {
			store.cleanup()
		}
	}()

	return store
}

func (s *IdempotencyStore) IsProcessed(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	timestamp, exists := s.processed[id]
	if !exists {
		return false
	}

	// 检查是否过期
	return time.Since(timestamp) < s.ttl
}

func (s *IdempotencyStore) MarkProcessed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed[id] = time.Now()
}

func (s *IdempotencyStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, timestamp := range s.processed {
		if now.Sub(timestamp) >= s.ttl {
			delete(s.processed, id)
		}
	}
}

// IdempotencyMiddleware 幂等性中间件
func IdempotencyMiddleware(store *IdempotencyStore) messaging.Middleware {
	return func(next messaging.Handler) messaging.Handler {
		return func(ctx context.Context, msg *messaging.Message) error {
			// 检查是否已处理
			if store.IsProcessed(msg.UUID) {
				log.Printf("  ⏭️  消息已处理，跳过: %s", msg.UUID)
				return msg.Ack()
			}

			// 处理消息
			err := next(ctx, msg)

			// 标记为已处理
			if err == nil {
				store.MarkProcessed(msg.UUID)
			}

			return err
		}
	}
}

func demonstrateIdempotency(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 4】消息幂等性 - 防止重复处理")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	store := NewIdempotencyStore(5 * time.Second)

	var processCount int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		count := atomic.AddInt32(&processCount, 1)
		log.Printf("  ✅ 实际处理 #%d: %s", count, string(msg.Payload))
		return msg.Ack()
	}

	router.AddHandlerWithMiddleware(
		"demo.idempotency",
		"idempotency-demo",
		handler,
		IdempotencyMiddleware(store),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送重复的消息（相同 UUID）...")

	// 使用相同的 UUID 发送 3 次
	msg := messaging.NewMessage("", []byte("支付订单-12345"))
	log.Printf("消息 UUID: %s\n", msg.UUID)

	for i := 1; i <= 3; i++ {
		log.Printf("第 %d 次发送", i)
		bus.Publisher().PublishMessage(context.Background(), "demo.idempotency", msg)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)
	router.Stop()

	log.Printf("\n发送 3 次，实际处理 %d 次（其他被跳过）\n", atomic.LoadInt32(&processCount))
}

// 核心知识点：
//
// 1. 错误处理分类
//    • 临时错误（Temporary）: 网络抖动、服务繁忙 → 可重试
//    • 永久错误（Permanent）: 参数错误、权限不足 → 不可重试
//    • 超时错误（Timeout）: 请求超时 → 可重试
//
// 2. 重试策略
//    • 固定间隔: 适用于快速恢复的场景
//    • 指数退避: 适用于服务过载的场景
//    • 抖动（Jitter）: 避免雷鸣群羊效应
//
// 3. 熔断器状态机
//    • Closed（关闭）: 正常处理请求
//    • Open（打开）: 拒绝请求，执行降级
//    • Half-Open（半开）: 尝试恢复，部分放行
//
// 4. 消息幂等性
//    • 基于 UUID 去重
//    • 滑动时间窗口
//    • 防止重复处理（支付、扣库存等）
//
// 5. 可靠性保障策略
//    • 超时控制: 防止无限等待
//    • 重试机制: 处理临时故障
//    • 熔断降级: 防止级联故障
//    • 幂等保证: 防止重复处理
//    • 限流保护: 防止过载
//
// 生产环境实践：
//
// 1. 重试配置建议
//    • 最大重试次数: 3-5 次
//    • 基础延迟: 100ms-500ms
//    • 最大延迟: 5s-10s
//    • 添加抖动: 10%-20%
//
// 2. 熔断器配置建议
//    • 失败阈值: 5-10 次
//    • 超时时间: 10s-60s
//    • 半开成功次数: 2-3 次
//
// 3. 幂等性实现方式
//    • 消息 UUID + 时间窗口（内存）
//    • 业务唯一键 + 数据库（持久化）
//    • 分布式锁（Redis）
//
// 最佳实践：
// ✅ 明确区分临时错误和永久错误
// ✅ 重试要有最大次数限制
// ✅ 使用指数退避避免雪崩
// ✅ 熔断器要有降级方案
// ✅ 关键操作必须保证幂等性
// ✅ 记录所有错误和重试日志
//
// 注意事项：
// ⚠️ 不是所有错误都应该重试
// ⚠️ 重试间隔不要设置太短
// ⚠️ 熔断器要合理设置恢复时间
// ⚠️ 幂等性存储要定期清理
// ⚠️ 降级逻辑要提前测试
