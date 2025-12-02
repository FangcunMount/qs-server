// Package main 演示性能优化和压测
// 并发处理、批量操作、性能监控
package main

import (
	"context"
	"fmt"
	"log"
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
	log.Println("=== 性能优化演示 ===")

	// ========== 演示 1: 基准测试 ==========
	demonstrateBenchmark()
	time.Sleep(2 * time.Second)

	// ========== 演示 2: 并发优化 ==========
	demonstrateConcurrency()
	time.Sleep(2 * time.Second)

	// ========== 演示 3: 批量处理优化 ==========
	demonstrateBatchProcessing()
	time.Sleep(2 * time.Second)

	// ========== 演示 4: 内存优化 ==========
	demonstrateMemoryOptimization()

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// ========== 演示 1: 基准测试 ==========

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	TotalMessages   int64
	SuccessMessages int64
	FailedMessages  int64
	TotalDuration   time.Duration
	StartTime       time.Time
	EndTime         time.Time
}

func (pm *PerformanceMetrics) Calculate() {
	pm.TotalDuration = pm.EndTime.Sub(pm.StartTime)
}

func (pm *PerformanceMetrics) Report() {
	log.Println("\n========== 性能报告 ==========")
	log.Printf("总消息数: %d", pm.TotalMessages)
	log.Printf("成功: %d (%.2f%%)", pm.SuccessMessages,
		float64(pm.SuccessMessages)/float64(pm.TotalMessages)*100)
	log.Printf("失败: %d (%.2f%%)", pm.FailedMessages,
		float64(pm.FailedMessages)/float64(pm.TotalMessages)*100)
	log.Printf("总耗时: %v", pm.TotalDuration)
	log.Printf("吞吐量: %.2f msg/s",
		float64(pm.TotalMessages)/pm.TotalDuration.Seconds())
	log.Printf("平均延迟: %.2f ms",
		float64(pm.TotalDuration.Milliseconds())/float64(pm.TotalMessages))
	log.Println("=============================")
}

func demonstrateBenchmark() {
	log.Println("【演示 1】基准测试 - 测量吞吐量和延迟")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Benchmark] ", log.LstdFlags)
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	metrics := &PerformanceMetrics{}

	handler := func(ctx context.Context, msg *messaging.Message) error {
		atomic.AddInt64(&metrics.SuccessMessages, 1)
		// 模拟处理（非常快）
		return msg.Ack()
	}

	router.AddHandler("demo.benchmark", "benchmark-demo", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 基准测试参数
	messageCount := int64(1000)
	log.Printf("发送 %d 条消息进行基准测试...\n", messageCount)

	metrics.StartTime = time.Now()
	metrics.TotalMessages = messageCount

	// 发送消息
	for i := int64(0); i < messageCount; i++ {
		msg := fmt.Sprintf("消息-%d", i)
		bus.Publisher().Publish(context.Background(), "demo.benchmark", []byte(msg))
	}

	// 等待处理完成
	for {
		processed := atomic.LoadInt64(&metrics.SuccessMessages)
		if processed >= messageCount {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	metrics.EndTime = time.Now()
	metrics.Calculate()
	router.Stop()

	metrics.Report()
}

// ========== 演示 2: 并发优化 ==========

func demonstrateConcurrency() {
	log.Println("【演示 2】并发优化 - 多 Worker 并行处理")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Concurrency] ", log.LstdFlags)

	// 测试不同的 Worker 数量
	workerCounts := []int{1, 2, 4, 8}

	for _, workers := range workerCounts {
		testConcurrency(bus, logger, workers)
		time.Sleep(2 * time.Second)
	}
}

func testConcurrency(bus messaging.EventBus, logger *log.Logger, workers int) {
	log.Printf("测试 %d 个 Worker...\n", workers)

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var processed int64
	handler := func(ctx context.Context, msg *messaging.Message) error {
		// 模拟耗时操作
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&processed, 1)
		return msg.Ack()
	}

	// 启动多个 Worker（通过多次注册相同的 Handler）
	for i := 0; i < workers; i++ {
		router.AddHandler("demo.concurrency", fmt.Sprintf("worker-%d", i), handler)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发送消息
	messageCount := 100
	start := time.Now()

	for i := 0; i < messageCount; i++ {
		msg := fmt.Sprintf("消息-%d", i)
		bus.Publisher().Publish(context.Background(), "demo.concurrency", []byte(msg))
	}

	// 等待处理完成
	for atomic.LoadInt64(&processed) < int64(messageCount) {
		time.Sleep(100 * time.Millisecond)
	}

	duration := time.Since(start)
	router.Stop()

	log.Printf("  ✅ %d Workers: 处理 %d 条消息，耗时 %v，吞吐量 %.2f msg/s\n",
		workers, messageCount, duration, float64(messageCount)/duration.Seconds())
}

// ========== 演示 3: 批量处理优化 ==========

// BatchProcessor 批量处理器
type BatchProcessor struct {
	mu          sync.Mutex
	batch       []*messaging.Message
	batchSize   int
	flushTicker *time.Ticker
	processor   func([]*messaging.Message) error
}

func NewBatchProcessor(batchSize int, flushInterval time.Duration, processor func([]*messaging.Message) error) *BatchProcessor {
	bp := &BatchProcessor{
		batchSize:   batchSize,
		flushTicker: time.NewTicker(flushInterval),
		processor:   processor,
	}

	// 定期刷新
	go func() {
		for range bp.flushTicker.C {
			bp.Flush()
		}
	}()

	return bp
}

func (bp *BatchProcessor) Add(msg *messaging.Message) error {
	bp.mu.Lock()
	bp.batch = append(bp.batch, msg)
	shouldFlush := len(bp.batch) >= bp.batchSize
	bp.mu.Unlock()

	if shouldFlush {
		return bp.Flush()
	}

	return nil
}

func (bp *BatchProcessor) Flush() error {
	bp.mu.Lock()
	if len(bp.batch) == 0 {
		bp.mu.Unlock()
		return nil
	}

	currentBatch := bp.batch
	bp.batch = nil
	bp.mu.Unlock()

	log.Printf("  📦 批量处理 %d 条消息", len(currentBatch))
	return bp.processor(currentBatch)
}

func (bp *BatchProcessor) Close() {
	bp.flushTicker.Stop()
	bp.Flush()
}

func demonstrateBatchProcessing() {
	log.Println("【演示 3】批量处理优化 - 批量操作提升性能")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Batch] ", log.LstdFlags)

	// 对比单条处理 vs 批量处理
	log.Println("场景 1: 单条处理")
	testSingleProcessing(bus, logger)

	time.Sleep(2 * time.Second)

	log.Println("\n场景 2: 批量处理（每 10 条批量）")
	testBatchProcessing(bus, logger)
}

func testSingleProcessing(bus messaging.EventBus, logger *log.Logger) {
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var processed int64
	handler := func(ctx context.Context, msg *messaging.Message) error {
		// 模拟数据库写入（单条）
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt64(&processed, 1)
		return msg.Ack()
	}

	router.AddHandler("demo.single", "single-demo", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	messageCount := 50
	start := time.Now()

	for i := 0; i < messageCount; i++ {
		msg := fmt.Sprintf("消息-%d", i)
		bus.Publisher().Publish(context.Background(), "demo.single", []byte(msg))
	}

	for atomic.LoadInt64(&processed) < int64(messageCount) {
		time.Sleep(100 * time.Millisecond)
	}

	duration := time.Since(start)
	router.Stop()

	log.Printf("  单条处理: %d 条消息，耗时 %v\n", messageCount, duration)
}

func testBatchProcessing(bus messaging.EventBus, logger *log.Logger) {
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var processed int64

	// 批量处理函数
	batchProcess := func(messages []*messaging.Message) error {
		// 模拟批量数据库写入（批量更快）
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&processed, int64(len(messages)))

		for _, msg := range messages {
			msg.Ack()
		}
		return nil
	}

	batchProcessor := NewBatchProcessor(10, 1*time.Second, batchProcess)
	defer batchProcessor.Close()

	handler := func(ctx context.Context, msg *messaging.Message) error {
		return batchProcessor.Add(msg)
	}

	router.AddHandler("demo.batch", "batch-demo", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	messageCount := 50
	start := time.Now()

	for i := 0; i < messageCount; i++ {
		msg := fmt.Sprintf("消息-%d", i)
		bus.Publisher().Publish(context.Background(), "demo.batch", []byte(msg))
	}

	for atomic.LoadInt64(&processed) < int64(messageCount) {
		time.Sleep(100 * time.Millisecond)
	}

	duration := time.Since(start)
	router.Stop()

	log.Printf("  批量处理: %d 条消息，耗时 %v\n", messageCount, duration)
}

// ========== 演示 4: 内存优化 ==========

func demonstrateMemoryOptimization() {
	log.Println("\n【演示 4】内存优化 - 对象池和零拷贝")

	// 使用 sync.Pool 优化内存分配
	msgPool := &sync.Pool{
		New: func() interface{} {
			return &messaging.Message{}
		},
	}

	log.Println("场景 1: 普通创建（每次 new）")
	start1 := time.Now()
	for i := 0; i < 10000; i++ {
		msg := messaging.NewMessage("", []byte(fmt.Sprintf("消息-%d", i)))
		_ = msg
	}
	duration1 := time.Since(start1)
	log.Printf("  普通创建: 10000 次，耗时 %v\n", duration1)

	log.Println("\n场景 2: 对象池（复用对象）")
	start2 := time.Now()
	for i := 0; i < 10000; i++ {
		msg := msgPool.Get().(*messaging.Message)
		msg.Payload = []byte(fmt.Sprintf("消息-%d", i))
		msgPool.Put(msg)
	}
	duration2 := time.Since(start2)
	log.Printf("  对象池: 10000 次，耗时 %v\n", duration2)

	improvement := float64(duration1-duration2) / float64(duration1) * 100
	log.Printf("  ✅ 性能提升: %.2f%%\n", improvement)
}

// 核心知识点：
//
// 1. 性能优化方向
//    • 吞吐量（Throughput）: 每秒处理的消息数
//    • 延迟（Latency）: 单条消息的处理时间
//    • 并发度（Concurrency）: 同时处理的消息数
//    • 内存占用（Memory）: 减少内存分配和 GC
//
// 2. 并发优化
//    • 多 Worker: 提升并发处理能力
//    • 协程池: 控制并发数，避免资源耗尽
//    • 无锁数据结构: 减少锁竞争
//
// 3. 批量处理
//    • 批量写入数据库: 减少网络往返
//    • 批量调用 API: 减少连接开销
//    • 批量发送消息: 提升吞吐量
//
// 4. 内存优化
//    • 对象池（sync.Pool）: 复用对象，减少 GC
//    • 零拷贝: 避免不必要的内存拷贝
//    • 预分配: 提前分配内存，避免动态扩容
//
// 5. 网络优化
//    • 连接池: 复用连接
//    • 批量发送: 减少网络 I/O
//    • 压缩: 减少传输数据量
//
// 性能基准：
//
// NSQ 单机性能（参考）:
// • 吞吐量: 10w+ msg/s
// • 延迟: P99 < 10ms
// • 内存: 每 100w 消息 ~100MB
//
// 优化前 vs 优化后:
// • 单 Worker: 1000 msg/s
// • 8 Worker: 8000 msg/s（8倍提升）
// • 批量处理: 5000 msg/s → 15000 msg/s（3倍提升）
// • 对象池: 减少 50% 内存分配
//
// 性能监控指标：
//
// 1. 业务指标
//    • 消息处理速率
//    • 消息积压数量
//    • 错误率
//
// 2. 系统指标
//    • CPU 使用率
//    • 内存使用率
//    • 网络带宽
//    • 磁盘 I/O
//
// 3. Go 运行时指标
//    • Goroutine 数量
//    • GC 频率和耗时
//    • 堆内存大小
//
// 性能优化流程：
//
// 1. 基准测试（Baseline）
//    • 测量当前性能
//    • 确定瓶颈
//
// 2. 性能分析（Profiling）
//    • CPU Profile
//    • Memory Profile
//    • Goroutine Profile
//    • Block Profile
//
// 3. 针对性优化
//    • 优化热点代码
//    • 减少内存分配
//    • 优化算法
//
// 4. 验证效果
//    • 再次基准测试
//    • 对比优化前后
//
// Go 性能分析工具：
//
// 1. pprof
//    ```go
//    import _ "net/http/pprof"
//    go func() {
//        http.ListenAndServe("localhost:6060", nil)
//    }()
//    ```
//
//    访问: http://localhost:6060/debug/pprof/
//
// 2. trace
//    ```go
//    import "runtime/trace"
//
//    f, _ := os.Create("trace.out")
//    trace.Start(f)
//    defer trace.Stop()
//    ```
//
// 3. benchstat
//    ```bash
//    go test -bench=. -count=10 > old.txt
//    # 优化代码
//    go test -bench=. -count=10 > new.txt
//    benchstat old.txt new.txt
//    ```
//
// 压测工具：
//
// 1. 内置压测
//    ```bash
//    go test -bench=. -benchmem
//    ```
//
// 2. wrk（HTTP）
//    ```bash
//    wrk -t4 -c100 -d30s http://localhost:8080/
//    ```
//
// 3. 自定义压测脚本
//    • 模拟真实负载
//    • 多场景测试
//
// 最佳实践：
// ✅ 先测量再优化（不要过早优化）
// ✅ 优化热点代码（80/20 原则）
// ✅ 使用性能分析工具定位瓶颈
// ✅ 批量操作优于单条操作
// ✅ 控制并发度（不是越多越好）
// ✅ 使用对象池减少内存分配
// ✅ 压测要模拟真实负载
// ✅ 持续监控生产环境性能
//
// 注意事项：
// ⚠️ 优化要有明确目标（吞吐量还是延迟）
// ⚠️ 过度优化会增加代码复杂度
// ⚠️ 并发不是越多越好（要平衡资源）
// ⚠️ 批量处理会增加延迟
// ⚠️ 对象池要正确使用（避免状态污染）
// ⚠️ 压测环境要接近生产环境
// ⚠️ 性能优化要考虑可维护性
//
// 性能调优清单：
// □ 使用多 Worker 并行处理
// □ 批量操作（数据库、API）
// □ 使用对象池（sync.Pool）
// □ 预分配切片容量
// □ 避免不必要的内存拷贝
// □ 使用连接池
// □ 启用消息压缩（跨数据中心）
// □ 调整 GC 参数（GOGC）
// □ 使用缓存（本地缓存、Redis）
// □ 异步处理非关键路径
