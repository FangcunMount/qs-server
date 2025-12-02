// Package main 演示高级中间件的使用
// RateLimit、CircuitBreaker、Filter、Priority、Deduplication
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

func main() {
	log.Println("=== 高级中间件演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Advanced] ", log.LstdFlags)

	// ========== 演示 1: RateLimit 中间件 ==========
	demonstrateRateLimit(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 2: CircuitBreaker 中间件 ==========
	demonstrateCircuitBreaker(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 3: Filter 中间件 ==========
	demonstrateFilter(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 4: Priority 中间件 ==========
	demonstratePriority(bus, logger)
	time.Sleep(5 * time.Second)

	// ========== 演示 5: Deduplication 中间件 ==========
	demonstrateDeduplication(bus, logger)
	time.Sleep(3 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// SimpleRateLimiter 简单的限流器实现
type SimpleRateLimiter struct {
	mu         sync.Mutex
	tokens     int
	rate       int
	period     time.Duration
	lastRefill time.Time
}

func (r *SimpleRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.lastRefill) >= r.period {
		r.tokens = r.rate
		r.lastRefill = now
	}

	if r.tokens > 0 {
		r.tokens--
		return true
	}
	return false
}

func (r *SimpleRateLimiter) Wait(ctx context.Context) error {
	for !r.Allow() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil
}

// demonstrateRateLimit 演示限流中间件
func demonstrateRateLimit(bus messaging.EventBus, logger *log.Logger) {
	log.Println("【演示 1】RateLimit 中间件 - 流量控制")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var processedCount int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		atomic.AddInt32(&processedCount, 1)
		log.Printf("  ✅ 处理消息 #%d", atomic.LoadInt32(&processedCount))
		return msg.Ack()
	}

	// 添加限流中间件：每秒最多 5 条消息
	// 创建简单的限流器
	limiter := &SimpleRateLimiter{rate: 5, period: time.Second}
	router.AddHandlerWithMiddleware(
		"demo.ratelimit",
		"ratelimit-demo",
		handler,
		messaging.RateLimitMiddleware(limiter, "block"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("快速发送 10 条消息（观察限流效果）...")
	log.Println("限流配置：5 条/秒")

	for i := 1; i <= 10; i++ {
		msg := fmt.Sprintf("消息 #%d", i)
		bus.Publisher().Publish(context.Background(), "demo.ratelimit", []byte(msg))
		time.Sleep(50 * time.Millisecond) // 快速发送
	}

	time.Sleep(3 * time.Second)
	router.Stop()
	log.Printf("\n总共处理: %d 条（预期 5-6 条）\n", atomic.LoadInt32(&processedCount))
}

// SimpleCircuitBreaker 简单的熔断器实现
type SimpleCircuitBreaker struct {
	mu        sync.Mutex
	state     string
	failures  int
	threshold int
	timeout   time.Duration
	openTime  time.Time
}

func (cb *SimpleCircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	if cb.state == "open" && time.Since(cb.openTime) >= cb.timeout {
		cb.state = "half-open"
		cb.failures = 0
	}
	state := cb.state
	cb.mu.Unlock()

	if state == "open" {
		return fmt.Errorf("circuit breaker is open")
	}

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		if cb.failures >= cb.threshold {
			cb.state = "open"
			cb.openTime = time.Now()
		}
	} else {
		if cb.state == "half-open" {
			cb.state = "closed"
		}
		cb.failures = 0
	}

	return err
}

func (cb *SimpleCircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// SimpleDeduplicationStore 简单的去重存储实现
type SimpleDeduplicationStore struct {
	mu   sync.RWMutex
	seen map[string]time.Time
}

func NewSimpleDeduplicationStore() *SimpleDeduplicationStore {
	store := &SimpleDeduplicationStore{
		seen: make(map[string]time.Time),
	}
	// 启动定期清理
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			store.cleanup()
		}
	}()
	return store
}

func (s *SimpleDeduplicationStore) Exists(uuid string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.seen[uuid]
	return exists
}

func (s *SimpleDeduplicationStore) Mark(uuid string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[uuid] = time.Now().Add(ttl)
	return nil
}

func (s *SimpleDeduplicationStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for uuid, expiry := range s.seen {
		if now.After(expiry) {
			delete(s.seen, uuid)
		}
	}
}

// demonstrateCircuitBreaker 演示熔断器中间件
func demonstrateCircuitBreaker(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 2】CircuitBreaker 中间件 - 熔断保护")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var attemptCount int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		count := atomic.AddInt32(&attemptCount, 1)
		log.Printf("  → 尝试处理 #%d", count)

		// 前 3 次失败，触发熔断
		if count <= 3 {
			return errors.New("模拟服务故障")
		}

		// 后续成功（但因为熔断可能无法到达）
		log.Println("  ✅ 处理成功")
		return msg.Ack()
	}

	// 创建简单的熔断器
	breaker := &SimpleCircuitBreaker{threshold: 3, timeout: 5 * time.Second}
	router.AddHandlerWithMiddleware(
		"demo.breaker",
		"breaker-demo",
		handler,
		messaging.CircuitBreakerMiddleware(breaker),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("连续发送消息（观察熔断过程）...")

	for i := 1; i <= 6; i++ {
		msg := fmt.Sprintf("消息 #%d", i)
		bus.Publisher().Publish(context.Background(), "demo.breaker", []byte(msg))
		time.Sleep(500 * time.Millisecond)

		if i == 3 {
			log.Println("\n⚡ 熔断器打开！后续请求将被拒绝...")
		}
	}

	time.Sleep(time.Second)
	router.Stop()
}

// demonstrateFilter 演示过滤器中间件
func demonstrateFilter(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 3】Filter 中间件 - 消息过滤")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  ✅ 处理消息: %s", string(msg.Payload))
		return msg.Ack()
	}

	// 添加过滤器：只处理优先级 >= 5 的消息
	filterFunc := func(msg *messaging.Message) bool {
		priorityStr, ok := msg.Metadata["priority"]
		if !ok {
			return false
		}
		priority, err := strconv.Atoi(priorityStr)
		if err != nil {
			return false
		}
		return priority >= 5
	}

	router.AddHandlerWithMiddleware(
		"demo.filter",
		"filter-demo",
		handler,
		messaging.FilterMiddleware(filterFunc),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送不同优先级的消息（过滤规则：priority >= 5）...")

	priorities := []int{2, 4, 5, 7, 9, 3}
	for _, p := range priorities {
		msg := messaging.NewMessage("", []byte(fmt.Sprintf("优先级=%d", p)))
		msg.Metadata["priority"] = fmt.Sprintf("%d", p)

		log.Printf("发送: priority=%d", p)
		bus.Publisher().PublishMessage(context.Background(), "demo.filter", msg)
		time.Sleep(300 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)
	router.Stop()
	log.Println("\n只有 priority >= 5 的消息被处理（5, 7, 9）")
}

// demonstratePriority 演示优先级中间件
func demonstratePriority(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 4】Priority 中间件 - 优先级队列")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		priority := msg.Metadata["priority"]
		log.Printf("  ✅ 处理消息: priority=%v, data=%s", priority, string(msg.Payload))
		time.Sleep(500 * time.Millisecond) // 模拟处理时间
		return msg.Ack()
	}

	// 优先级提取函数
	getPriority := func(msg *messaging.Message) int {
		priorityStr, ok := msg.Metadata["priority"]
		if !ok {
			return 0
		}
		priority, _ := strconv.Atoi(priorityStr)
		return priority
	}

	// 添加优先级中间件
	router.AddHandlerWithMiddleware(
		"demo.priority",
		"priority-demo",
		handler,
		messaging.PriorityMiddleware(getPriority),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("快速发送不同优先级的消息...")

	tasks := []struct {
		priority int
		data     string
	}{
		{priority: 5, data: "普通任务A"},
		{priority: 10, data: "重要任务B"},
		{priority: 3, data: "低优先级C"},
		{priority: 9, data: "重要任务D"},
		{priority: 1, data: "低优先级E"},
	}

	for _, task := range tasks {
		msg := messaging.NewMessage("", []byte(task.data))
		msg.Metadata["priority"] = fmt.Sprintf("%d", task.priority)

		log.Printf("发送: priority=%d, data=%s", task.priority, task.data)
		bus.Publisher().PublishMessage(context.Background(), "demo.priority", msg)
		time.Sleep(100 * time.Millisecond)
	}

	log.Println("\n观察处理顺序（高优先级优先处理）...")
	time.Sleep(4 * time.Second)
	router.Stop()
}

// demonstrateDeduplication 演示去重中间件
func demonstrateDeduplication(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 5】Deduplication 中间件 - 消息去重")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var processedCount int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		count := atomic.AddInt32(&processedCount, 1)
		log.Printf("  ✅ 处理消息 #%d: %s", count, string(msg.Payload))
		return msg.Ack()
	}

	// 创建去重存储
	dedupStore := NewSimpleDeduplicationStore()

	// 添加去重中间件：5 秒内的重复消息会被过滤
	router.AddHandlerWithMiddleware(
		"demo.dedup",
		"dedup-demo",
		handler,
		messaging.DeduplicationMiddleware(dedupStore, 5*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送重复的消息（观察去重效果）...")

	// 使用固定UUID发送相同的消息 3 次
	uuid := "fixed-uuid-12345"
	for i := 1; i <= 3; i++ {
		msg := messaging.NewMessage(uuid, []byte("订单-12345"))
		log.Printf("第 %d 次发送: 订单-12345", i)
		bus.Publisher().PublishMessage(context.Background(), "demo.dedup", msg)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)
	router.Stop()
	log.Printf("\n总共处理: %d 条（预期 1 条，其他被去重）\n", atomic.LoadInt32(&processedCount))
}

// 核心知识点：
//
// 1. RateLimit 中间件（流量控制）
//    - 使用令牌桶算法
//    - 防止下游服务过载
//    - 平滑流量尖峰
//
// 2. CircuitBreaker 中间件（熔断保护）
//    - 三种状态：关闭、打开、半开
//    - 失败次数达到阈值时打开
//    - 超时后进入半开状态尝试恢复
//    - 防止级联故障
//
// 3. Filter 中间件（消息过滤）
//    - 自定义过滤规则
//    - 基于 Metadata 过滤
//    - 减少无效处理
//
// 4. Priority 中间件（优先级队列）
//    - 高优先级消息优先处理
//    - 使用堆数据结构
//    - 适用于任务调度
//
// 5. Deduplication 中间件（消息去重）
//    - 基于消息 UUID 去重
//    - 滑动时间窗口
//    - 防止重复处理
//
// 应用场景：
// • RateLimit: API 网关、数据库写入、第三方调用
// • CircuitBreaker: 微服务调用、外部 API、数据库连接
// • Filter: 日志采样、A/B 测试、灰度发布
// • Priority: 任务调度、紧急通知、VIP 用户
// • Deduplication: 支付订单、库存扣减、消息推送
//
// 最佳实践：
// ✅ RateLimit 配置要根据下游服务能力设置
// ✅ CircuitBreaker 要设置合理的失败阈值和恢复时间
// ✅ Filter 规则要简单高效，避免复杂计算
// ✅ Priority 要平衡高优先级和低优先级的处理
// ✅ Deduplication 时间窗口不要设置太长，避免内存占用
//
// 注意事项：
// ⚠️ 限流会影响吞吐量，要合理配置
// ⚠️ 熔断可能导致服务降级，要有降级方案
// ⚠️ 过滤会丢弃消息，要记录日志
// ⚠️ 优先级队列可能导致低优先级消息饥饿
// ⚠️ 去重需要额外的内存和计算开销
