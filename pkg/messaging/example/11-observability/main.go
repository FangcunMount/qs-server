// Package main 演示可观测性实践
// Metrics、Tracing、Health Check
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

// 全局指标收集器
var metrics = &MetricsCollector{
	counters: make(map[string]*int64),
	gauges:   make(map[string]*int64),
}

func main() {
	log.Println("=== 可观测性演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Observability] ", log.LstdFlags)

	// 启动 Metrics HTTP 服务
	go startMetricsServer()
	log.Println("Metrics 服务已启动: http://localhost:9090/metrics")

	// ========== 演示 1: Metrics 监控 ==========
	demonstrateMetrics(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 2: Tracing 追踪 ==========
	demonstrateTracing(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 3: Health Check 健康检查 ==========
	demonstrateHealthCheck(bus, logger)
	time.Sleep(3 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// ========== Metrics 监控 ==========

// MetricsCollector 简单的指标收集器
type MetricsCollector struct {
	counters map[string]*int64
	gauges   map[string]*int64
}

func (m *MetricsCollector) IncCounter(name string) {
	if _, exists := m.counters[name]; !exists {
		var v int64
		m.counters[name] = &v
	}
	atomic.AddInt64(m.counters[name], 1)
}

func (m *MetricsCollector) SetGauge(name string, value int64) {
	if _, exists := m.gauges[name]; !exists {
		var v int64
		m.gauges[name] = &v
	}
	atomic.StoreInt64(m.gauges[name], value)
}

func (m *MetricsCollector) Export() map[string]int64 {
	result := make(map[string]int64)
	for name, ptr := range m.counters {
		result[name] = atomic.LoadInt64(ptr)
	}
	for name, ptr := range m.gauges {
		result[name] = atomic.LoadInt64(ptr)
	}
	return result
}

// MetricsMiddleware 收集处理指标
func MetricsMiddleware(collector *MetricsCollector) messaging.Middleware {
	return func(next messaging.Handler) messaging.Handler {
		return func(ctx context.Context, msg *messaging.Message) error {
			start := time.Now()

			// 增加处理计数
			collector.IncCounter("messages_total")
			collector.IncCounter("messages_processing")

			err := next(ctx, msg)

			// 记录处理时长
			duration := time.Since(start).Milliseconds()
			collector.SetGauge("message_duration_ms", duration)

			// 记录成功/失败
			if err != nil {
				collector.IncCounter("messages_failed")
			} else {
				collector.IncCounter("messages_success")
			}

			collector.IncCounter("messages_processing_done")

			return err
		}
	}
}

func startMetricsServer() {
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		data := metrics.Export()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	http.ListenAndServe(":9090", nil)
}

func demonstrateMetrics(bus messaging.EventBus, logger *log.Logger) {
	log.Println("【演示 1】Metrics 监控 - 收集处理指标")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))
	router.AddMiddleware(MetricsMiddleware(metrics))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		// 随机处理时间
		time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)

		// 随机成功/失败
		if rand.Float32() < 0.2 {
			return fmt.Errorf("处理失败")
		}

		return msg.Ack()
	}

	router.AddHandler("demo.metrics", "metrics-demo", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送 10 条消息并收集指标...")

	for i := 1; i <= 10; i++ {
		msg := messaging.NewMessage("", []byte(fmt.Sprintf("消息-%d", i)))
		bus.Publisher().PublishMessage(context.Background(), "demo.metrics", msg)
		time.Sleep(200 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)
	router.Stop()

	log.Println("\n当前指标:")
	for name, value := range metrics.Export() {
		log.Printf("  %s: %d", name, value)
	}
	log.Println("\n访问 http://localhost:9090/metrics 查看完整指标")
}

// ========== Tracing 追踪 ==========

// TraceContext 追踪上下文
type TraceContext struct {
	TraceID  string
	SpanID   string
	ParentID string
}

// TracingMiddleware 分布式追踪中间件
func TracingMiddleware(logger *log.Logger) messaging.Middleware {
	return func(next messaging.Handler) messaging.Handler {
		return func(ctx context.Context, msg *messaging.Message) error {
			// 提取或生成 TraceID
			traceID := msg.Metadata["trace_id"]
			if traceID == "" {
				traceID = generateID()
			}

			// 生成新的 SpanID
			spanID := generateID()
			parentID := msg.Metadata["span_id"] // 创建追踪上下文
			trace := TraceContext{
				TraceID:  traceID,
				SpanID:   spanID,
				ParentID: parentID,
			}

			logger.Printf("🔍 [Trace] TraceID=%s, SpanID=%s, ParentID=%s",
				trace.TraceID, trace.SpanID, trace.ParentID)

			start := time.Now()
			err := next(ctx, msg)
			duration := time.Since(start)

			// 记录 Span
			logger.Printf("✅ [Span] Duration=%dms, Error=%v",
				duration.Milliseconds(), err != nil)

			return err
		}
	}
}

func generateID() string {
	return fmt.Sprintf("%016x", rand.Int63())
}

func demonstrateTracing(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 2】Tracing 追踪 - 分布式追踪")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))
	router.AddMiddleware(TracingMiddleware(logger))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → 执行业务逻辑...")
		time.Sleep(300 * time.Millisecond)

		// 模拟调用下游服务（传递 TraceID）
		traceID := msg.Metadata["trace_id"]
		log.Printf("  → 调用下游服务 (TraceID=%s)", traceID)

		return msg.Ack()
	}

	router.AddHandler("demo.tracing", "tracing-demo", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送带追踪信息的消息...")

	// 模拟请求链路
	msg := messaging.NewMessage("", []byte("用户请求"))
	msg.Metadata["trace_id"] = "00000000000001"
	msg.Metadata["span_id"] = "00000000000002"

	bus.Publisher().PublishMessage(context.Background(), "demo.tracing", msg)

	time.Sleep(2 * time.Second)
	router.Stop()
}

// ========== Health Check 健康检查 ==========

func demonstrateHealthCheck(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 3】Health Check - 健康检查")

	// 启动健康检查 HTTP 服务
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := bus.Health()

		health := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now().Format(time.RFC3339),
			"checks": map[string]string{
				"eventbus":  "ok",
				"publisher": "ok",
				"router":    "ok",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if status == nil {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(health)
	})

	go http.ListenAndServe(":9091", nil)
	log.Println("Health Check 服务已启动: http://localhost:9091/health")

	// 定期检查健康状态
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for i := 0; i < 3; i++ {
		<-ticker.C
		status := bus.Health()
		if status == nil {
			log.Println("✅ 健康检查: 系统正常")
		} else {
			log.Println("❌ 健康检查: 系统异常")
		}
	}

	log.Println("\n访问 http://localhost:9091/health 查看健康状态")
}

// 核心知识点：
//
// 1. Metrics 监控（度量）
//    • Counter: 累加计数（消息总数、错误次数）
//    • Gauge: 瞬时值（队列长度、处理时长）
//    • Histogram: 分布统计（响应时间分布）
//    • Summary: 汇总统计（百分位数）
//
// 2. Tracing 追踪（链路）
//    • TraceID: 全局唯一，标识一次完整请求
//    • SpanID: 标识一个处理阶段
//    • ParentID: 标识父 Span，形成调用链
//    • Span: 包含时间戳、持续时间、状态
//
// 3. Health Check 健康检查
//    • Liveness: 存活性检查（进程是否运行）
//    • Readiness: 就绪性检查（是否可以处理请求）
//    • Dependency: 依赖检查（数据库、消息队列等）
//
// 4. 可观测性三大支柱
//    • Metrics: What is happening?（发生了什么）
//    • Tracing: Where is it happening?（在哪发生）
//    • Logging: Why is it happening?（为什么发生）
//
// 5. 可观测性最佳实践
//    • 结构化日志: 使用 JSON 格式
//    • 统一标识: TraceID 贯穿全链路
//    • 采样策略: 生产环境要采样避免性能开销
//    • 告警规则: 基于 SLA 设置合理阈值
//
// 生产环境集成：
//
// 1. Metrics 集成（Prometheus）
//    • 使用 prometheus/client_golang
//    • 导出标准格式的 /metrics 端点
//    • 配置 Prometheus 抓取
//
// 2. Tracing 集成（Jaeger/Zipkin）
//    • 使用 OpenTelemetry SDK
//    • 配置 Trace Exporter
//    • 设置采样率（如 1%）
//
// 3. Logging 集成（ELK/Loki）
//    • 使用 zerolog/zap
//    • 输出 JSON 格式
//    • 集中化日志收集
//
// 4. Health Check 集成（Kubernetes）
//    • Liveness Probe: /health/live
//    • Readiness Probe: /health/ready
//    • Startup Probe: /health/startup
//
// 关键指标：
// • messages_total: 总消息数（Counter）
// • messages_success: 成功消息数（Counter）
// • messages_failed: 失败消息数（Counter）
// • message_duration_ms: 处理时长（Gauge/Histogram）
// • queue_depth: 队列深度（Gauge）
// • error_rate: 错误率（Derived）
// • throughput: 吞吐量（Derived）
//
// 告警规则示例：
// • 错误率 > 5%
// • P99 延迟 > 1000ms
// • 队列深度 > 10000
// • 处理速率 < 100 msg/s
//
// 最佳实践：
// ✅ 每个服务导出标准的 /metrics 端点
// ✅ 使用统一的 TraceID 格式（UUID/Hex）
// ✅ 健康检查要快速（< 100ms）
// ✅ 日志要包含上下文（TraceID、UserID 等）
// ✅ 告警要可操作（有明确的处理步骤）
//
// 注意事项：
// ⚠️ Metrics 收集有性能开销，不要过于细粒度
// ⚠️ Tracing 要设置采样率，避免大量数据
// ⚠️ 健康检查不要做耗时操作
// ⚠️ 日志级别要分级（Debug/Info/Warn/Error）
// ⚠️ 敏感信息不要记录到日志中
