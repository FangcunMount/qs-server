package messaging

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ========== 日志中间件 ==========

// LoggerMiddleware 日志中间件
// 记录消息处理的开始、结束、耗时和错误
func LoggerMiddleware(logger *log.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			start := time.Now()

			if logger != nil {
				logger.Printf("[Messaging] 开始处理消息: topic=%s, uuid=%s, attempts=%d",
					msg.Topic, msg.UUID, msg.Attempts)
			}

			err := next(ctx, msg)

			if logger != nil {
				if err != nil {
					logger.Printf("[Messaging] 消息处理失败: topic=%s, uuid=%s, 耗时=%v, error=%v",
						msg.Topic, msg.UUID, time.Since(start), err)
				} else {
					logger.Printf("[Messaging] 消息处理成功: topic=%s, uuid=%s, 耗时=%v",
						msg.Topic, msg.UUID, time.Since(start))
				}
			}

			return err
		}
	}
}

// ========== 重试中间件 ==========

// RetryMiddleware 重试中间件
// 在消息处理失败时自动重试
// maxRetries: 最大重试次数
// delay: 每次重试的延迟时间（指数退避）
func RetryMiddleware(maxRetries int, delay time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			var err error

			for i := 0; i <= maxRetries; i++ {
				err = next(ctx, msg)
				if err == nil {
					return nil
				}

				// 最后一次重试失败，直接返回错误
				if i == maxRetries {
					break
				}

				// 指数退避
				backoff := delay * time.Duration(1<<uint(i))
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
					// 继续下一次重试
				}
			}

			return fmt.Errorf("消息处理失败，已重试 %d 次: %w", maxRetries, err)
		}
	}
}

// ========== 超时中间件 ==========

// TimeoutMiddleware 超时中间件
// 限制消息处理的最大时间
// timeout: 超时时间
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			errChan := make(chan error, 1)

			go func() {
				errChan <- next(ctx, msg)
			}()

			select {
			case err := <-errChan:
				return err
			case <-ctx.Done():
				return fmt.Errorf("消息处理超时 (%v): topic=%s, uuid=%s", timeout, msg.Topic, msg.UUID)
			}
		}
	}
}

// ========== 恢复中间件 ==========

// RecoverMiddleware 恢复中间件
// 捕获 panic，防止单个消息处理失败导致整个程序崩溃
func RecoverMiddleware(logger *log.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("消息处理 panic: %v, topic=%s, uuid=%s", r, msg.Topic, msg.UUID)
					if logger != nil {
						logger.Printf("[Messaging] PANIC: %v", err)
					}
				}
			}()

			return next(ctx, msg)
		}
	}
}

// ========== 指标中间件 ==========

// MetricsMiddleware 指标中间件
// 收集消息处理的统计信息
type MetricsCollector interface {
	// IncrementProcessed 增加处理消息计数
	IncrementProcessed(topic string)

	// IncrementFailed 增加失败消息计数
	IncrementFailed(topic string)

	// RecordDuration 记录处理耗时
	RecordDuration(topic string, duration time.Duration)
}

// MetricsMiddleware 创建指标中间件
func MetricsMiddleware(collector MetricsCollector) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			start := time.Now()

			err := next(ctx, msg)

			duration := time.Since(start)
			collector.RecordDuration(msg.Topic, duration)

			if err != nil {
				collector.IncrementFailed(msg.Topic)
			} else {
				collector.IncrementProcessed(msg.Topic)
			}

			return err
		}
	}
}

// ========== 链路追踪中间件 ==========

// TracingMiddleware 链路追踪中间件
// 自动注入和提取 trace_id、span_id
func TracingMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			// 从 Metadata 中提取 trace_id
			traceID := msg.Metadata["trace_id"]
			if traceID == "" {
				// 如果没有 trace_id，生成一个新的
				traceID = msg.UUID
				msg.Metadata["trace_id"] = traceID
			}

			// 将 trace_id 注入到 context
			ctx = context.WithValue(ctx, "trace_id", traceID)

			return next(ctx, msg)
		}
	}
}

// ========== 去重中间件 ==========

// DeduplicationStore 去重存储接口
type DeduplicationStore interface {
	// Exists 检查消息是否已处理
	Exists(uuid string) bool

	// Mark 标记消息已处理
	Mark(uuid string, ttl time.Duration) error
}

// DeduplicationMiddleware 创建去重中间件
// 防止重复处理相同的消息
func DeduplicationMiddleware(store DeduplicationStore, ttl time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			// 检查消息是否已处理
			if store.Exists(msg.UUID) {
				// 已处理，直接跳过
				return nil
			}

			// 处理消息
			err := next(ctx, msg)

			// 只有成功处理后才标记
			if err == nil {
				store.Mark(msg.UUID, ttl)
			}

			return err
		}
	}
}

// ========== 限流中间件 ==========

// RateLimiter 限流器接口
type RateLimiter interface {
	// Allow 检查是否允许处理
	Allow() bool

	// Wait 等待直到允许处理
	Wait(ctx context.Context) error
}

// RateLimitMiddleware 限流中间件
// 限制消息处理速率，防止系统过载
// mode: "drop" 丢弃超限消息，"wait" 等待直到允许处理
func RateLimitMiddleware(limiter RateLimiter, mode string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			if mode == "wait" {
				// 等待模式：阻塞直到允许处理
				if err := limiter.Wait(ctx); err != nil {
					return fmt.Errorf("限流等待失败: %w", err)
				}
			} else {
				// 丢弃模式：直接拒绝超限消息
				if !limiter.Allow() {
					return fmt.Errorf("消息被限流丢弃: topic=%s, uuid=%s", msg.Topic, msg.UUID)
				}
			}

			return next(ctx, msg)
		}
	}
}

// TokenBucketLimiter 令牌桶限流器（简单实现）
type TokenBucketLimiter struct {
	tokens   chan struct{}
	refillCh chan struct{}
	rate     time.Duration
}

// NewTokenBucketLimiter 创建令牌桶限流器
// capacity: 桶容量
// rate: 令牌生成速率
func NewTokenBucketLimiter(capacity int, rate time.Duration) *TokenBucketLimiter {
	limiter := &TokenBucketLimiter{
		tokens:   make(chan struct{}, capacity),
		refillCh: make(chan struct{}),
		rate:     rate,
	}

	// 初始化令牌
	for i := 0; i < capacity; i++ {
		limiter.tokens <- struct{}{}
	}

	// 启动令牌补充协程
	go limiter.refill()

	return limiter
}

// Allow 检查是否允许处理
func (l *TokenBucketLimiter) Allow() bool {
	select {
	case <-l.tokens:
		return true
	default:
		return false
	}
}

// Wait 等待直到允许处理
func (l *TokenBucketLimiter) Wait(ctx context.Context) error {
	select {
	case <-l.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// refill 补充令牌
func (l *TokenBucketLimiter) refill() {
	ticker := time.NewTicker(l.rate)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case l.tokens <- struct{}{}:
			// 成功添加令牌
		default:
			// 桶已满，跳过
		}
	}
}

// ========== 熔断器中间件 ==========

// CircuitBreaker 熔断器接口
type CircuitBreaker interface {
	// Call 执行调用（带熔断保护）
	Call(fn func() error) error

	// State 获取当前状态（closed, open, half-open）
	State() string
}

// CircuitBreakerMiddleware 熔断器中间件
// 防止级联故障，当错误率超过阈值时自动熔断
func CircuitBreakerMiddleware(breaker CircuitBreaker) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			return breaker.Call(func() error {
				return next(ctx, msg)
			})
		}
	}
}

// SimpleCircuitBreaker 简单熔断器实现
type SimpleCircuitBreaker struct {
	maxFailures  int           // 最大失败次数
	timeout      time.Duration // 熔断超时时间
	failures     int           // 当前失败次数
	lastFailTime time.Time     // 上次失败时间
	state        string        // 状态：closed, open, half-open
}

// NewSimpleCircuitBreaker 创建简单熔断器
// maxFailures: 最大失败次数（超过则熔断）
// timeout: 熔断持续时间（之后尝试恢复）
func NewSimpleCircuitBreaker(maxFailures int, timeout time.Duration) *SimpleCircuitBreaker {
	return &SimpleCircuitBreaker{
		maxFailures: maxFailures,
		timeout:     timeout,
		state:       "closed",
	}
}

// Call 执行调用（带熔断保护）
func (cb *SimpleCircuitBreaker) Call(fn func() error) error {
	// 检查是否可以尝试恢复
	if cb.state == "open" {
		if time.Since(cb.lastFailTime) > cb.timeout {
			cb.state = "half-open"
		} else {
			return fmt.Errorf("熔断器开启中，拒绝处理")
		}
	}

	// 执行调用
	err := fn()

	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()

		if cb.failures >= cb.maxFailures {
			cb.state = "open"
			return fmt.Errorf("熔断器触发: %w", err)
		}

		return err
	}

	// 成功，重置失败计数
	if cb.state == "half-open" {
		cb.state = "closed"
	}
	cb.failures = 0

	return nil
}

// State 获取当前状态
func (cb *SimpleCircuitBreaker) State() string {
	return cb.state
}

// ========== 批处理中间件 ==========

// BatchProcessor 批处理器接口
type BatchProcessor interface {
	// Add 添加消息到批次
	Add(msg *Message) error

	// Flush 刷新批次（强制处理）
	Flush() error
}

// BatchMiddleware 批处理中间件
// 将多个消息合并处理，提高吞吐量
// batchSize: 批次大小
// flushInterval: 刷新间隔
func BatchMiddleware(batchSize int, flushInterval time.Duration, batchHandler func([]*Message) error) Middleware {
	batch := make([]*Message, 0, batchSize)
	var lastFlush time.Time

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}

		err := batchHandler(batch)
		batch = batch[:0]
		lastFlush = time.Now()
		return err
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			batch = append(batch, msg)

			// 检查是否达到批次大小或超时
			if len(batch) >= batchSize || time.Since(lastFlush) > flushInterval {
				if err := flush(); err != nil {
					return err
				}
			}

			return nil
		}
	}
}

// ========== 条件过滤中间件 ==========

// FilterMiddleware 条件过滤中间件
// 根据条件决定是否处理消息
func FilterMiddleware(predicate func(*Message) bool) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			if !predicate(msg) {
				// 不满足条件，跳过处理
				return nil
			}

			return next(ctx, msg)
		}
	}
}

// ========== 优先级中间件 ==========

// PriorityMiddleware 优先级中间件
// 根据消息优先级排序处理
func PriorityMiddleware(getPriority func(*Message) int) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			// 将优先级存入 Metadata
			priority := getPriority(msg)
			msg.Metadata["priority"] = fmt.Sprintf("%d", priority)

			return next(ctx, msg)
		}
	}
}

// ========== 审计中间件 ==========

// AuditLogger 审计日志接口
type AuditLogger interface {
	// Log 记录审计日志
	Log(event string, msg *Message, err error)
}

// AuditMiddleware 审计中间件
// 记录消息处理的完整审计日志
func AuditMiddleware(logger AuditLogger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, msg *Message) error {
			logger.Log("message.received", msg, nil)

			err := next(ctx, msg)

			if err != nil {
				logger.Log("message.failed", msg, err)
			} else {
				logger.Log("message.processed", msg, nil)
			}

			return err
		}
	}
}
