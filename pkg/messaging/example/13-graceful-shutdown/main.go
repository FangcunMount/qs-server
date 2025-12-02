// Package main 演示优雅关闭
// 信号处理、资源清理、未完成任务处理
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
	log.Println("=== 优雅关闭演示 ===")

	// ========== 演示 1: 基础信号处理 ==========
	// demonstrateBasicShutdown()

	// ========== 演示 2: 完整的优雅关闭 ==========
	demonstrateGracefulShutdown()

	log.Println("\n程序已退出")
}

// ========== 演示 1: 基础信号处理 ==========

func demonstrateBasicShutdown() {
	log.Println("【演示 1】基础信号处理")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	logger := log.New(os.Stdout, "[Basic] ", log.LstdFlags)

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("处理消息: %s", string(msg.Payload))
		time.Sleep(2 * time.Second)
		return msg.Ack()
	}

	router.AddHandler("demo.shutdown", "shutdown-demo", handler)

	// 启动 Router
	ctx, cancel := context.WithCancel(context.Background())
	go router.Run(ctx)

	// 发送一些消息
	go func() {
		for i := 1; i <= 5; i++ {
			msg := fmt.Sprintf("消息-%d", i)
			bus.Publisher().Publish(context.Background(), "demo.shutdown", []byte(msg))
			time.Sleep(time.Second)
		}
	}()

	// 监听终止信号
	log.Println("按 Ctrl+C 触发优雅关闭...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("\n收到终止信号，开始优雅关闭...")

	// 1. 停止接收新消息
	cancel()

	// 2. 等待处理中的消息完成（简单等待）
	log.Println("等待处理中的消息完成...")
	time.Sleep(3 * time.Second)

	// 3. 关闭连接
	bus.Close()
	log.Println("关闭完成")
}

// ========== 演示 2: 完整的优雅关闭 ==========

// ShutdownManager 优雅关闭管理器
type ShutdownManager struct {
	mu             sync.Mutex
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	shutdownHooks  []func() error
	processingMsgs int32
	startTime      time.Time
}

func NewShutdownManager() *ShutdownManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ShutdownManager{
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
	}
}

// Context 返回全局上下文
func (sm *ShutdownManager) Context() context.Context {
	return sm.ctx
}

// RegisterHook 注册关闭钩子
func (sm *ShutdownManager) RegisterHook(hook func() error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.shutdownHooks = append(sm.shutdownHooks, hook)
}

// AddTask 添加任务（增加计数）
func (sm *ShutdownManager) AddTask() {
	sm.wg.Add(1)
	atomic.AddInt32(&sm.processingMsgs, 1)
}

// TaskDone 任务完成（减少计数）
func (sm *ShutdownManager) TaskDone() {
	atomic.AddInt32(&sm.processingMsgs, -1)
	sm.wg.Done()
}

// ProcessingCount 返回正在处理的任务数
func (sm *ShutdownManager) ProcessingCount() int32 {
	return atomic.LoadInt32(&sm.processingMsgs)
}

// Shutdown 执行优雅关闭
func (sm *ShutdownManager) Shutdown(timeout time.Duration) error {
	log.Println("\n========== 开始优雅关闭 ==========")
	shutdownStart := time.Now()

	// 1. 通知所有 goroutine 停止
	log.Println("步骤 1: 发送停止信号...")
	sm.cancel()

	// 2. 等待所有任务完成（带超时）
	log.Printf("步骤 2: 等待 %d 个任务完成...", sm.ProcessingCount())

	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("  ✅ 所有任务已完成")
	case <-time.After(timeout):
		log.Printf("  ⚠️  超时！仍有 %d 个任务未完成", sm.ProcessingCount())
	}

	// 3. 执行关闭钩子
	log.Println("步骤 3: 执行关闭钩子...")
	sm.mu.Lock()
	hooks := sm.shutdownHooks
	sm.mu.Unlock()

	for i, hook := range hooks {
		log.Printf("  → 执行钩子 %d/%d", i+1, len(hooks))
		if err := hook(); err != nil {
			log.Printf("  ⚠️  钩子执行失败: %v", err)
		}
	}

	// 4. 输出统计信息
	log.Println("\n========== 关闭统计 ==========")
	log.Printf("运行时长: %v", time.Since(sm.startTime))
	log.Printf("关闭耗时: %v", time.Since(shutdownStart))
	log.Printf("剩余任务: %d", sm.ProcessingCount())
	log.Println("============================")

	return nil
}

// WrapHandler 包装处理器，自动管理任务计数
func (sm *ShutdownManager) WrapHandler(handler messaging.Handler) messaging.Handler {
	return func(ctx context.Context, msg *messaging.Message) error {
		// 检查是否正在关闭
		select {
		case <-sm.ctx.Done():
			log.Println("  ⚠️  系统正在关闭，拒绝新任务")
			return msg.Nack()
		default:
		}

		// 添加任务计数
		sm.AddTask()
		defer sm.TaskDone()

		// 使用全局上下文
		return handler(sm.ctx, msg)
	}
}

func demonstrateGracefulShutdown() {
	log.Println("【演示 2】完整的优雅关闭")

	// 创建关闭管理器
	shutdownMgr := NewShutdownManager()

	// 创建 EventBus
	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	logger := log.New(os.Stdout, "[Graceful] ", log.LstdFlags)

	// 注册关闭钩子
	shutdownMgr.RegisterHook(func() error {
		log.Println("关闭 EventBus...")
		return bus.Close()
	})

	// 创建 Router
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var processedCount int32
	var successCount int32
	var rejectedCount int32

	// 业务处理器
	businessHandler := func(ctx context.Context, msg *messaging.Message) error {
		processed := atomic.AddInt32(&processedCount, 1)
		log.Printf("  → [%d] 开始处理: %s", processed, string(msg.Payload))

		// 模拟耗时操作（检查上下文是否取消）
		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				log.Printf("  ⚠️  [%d] 任务被中断", processed)
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				// 继续处理
			}
		}

		atomic.AddInt32(&successCount, 1)
		log.Printf("  ✅ [%d] 处理完成: %s", processed, string(msg.Payload))
		return msg.Ack()
	}

	// 包装处理器（自动管理任务计数）
	wrappedHandler := shutdownMgr.WrapHandler(businessHandler)

	// 注册处理器
	router.AddHandler("demo.graceful", "graceful-demo", wrappedHandler)

	// 启动 Router
	go router.Run(shutdownMgr.Context())

	// 注册关闭钩子
	shutdownMgr.RegisterHook(func() error {
		log.Println("停止 Router...")
		router.Stop()
		return nil
	})

	// 生产者：持续发送消息
	go func() {
		for i := 1; ; i++ {
			select {
			case <-shutdownMgr.Context().Done():
				log.Println("\n生产者已停止")
				return
			default:
				msg := fmt.Sprintf("任务-%d", i)
				err := bus.Publisher().Publish(shutdownMgr.Context(), "demo.graceful", []byte(msg))
				if err != nil {
					atomic.AddInt32(&rejectedCount, 1)
				}
				time.Sleep(800 * time.Millisecond)
			}
		}
	}()

	// 监听终止信号
	log.Println("服务已启动")
	log.Println("提示: 按 Ctrl+C 触发优雅关闭")
	log.Println("观察: 正在处理的任务会完成，新任务会被拒绝")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan

	// 执行优雅关闭（15 秒超时）
	if err := shutdownMgr.Shutdown(15 * time.Second); err != nil {
		log.Printf("关闭失败: %v", err)
	}

	// 输出最终统计
	log.Println("========== 最终统计 ==========")
	log.Printf("已处理: %d", atomic.LoadInt32(&successCount))
	log.Printf("已拒绝: %d", atomic.LoadInt32(&rejectedCount))
	log.Println("============================")
}

// 核心知识点：
//
// 1. 信号处理
//    • SIGINT (Ctrl+C): 中断信号
//    • SIGTERM: 终止信号（docker stop, k8s 默认）
//    • SIGKILL: 强制终止（无法捕获）
//
// 2. 优雅关闭步骤
//    Step 1: 停止接收新请求（关闭监听）
//    Step 2: 等待正在处理的请求完成
//    Step 3: 关闭资源（数据库、连接等）
//    Step 4: 退出程序
//
// 3. 上下文传播
//    • 全局 Context: 传递关闭信号
//    • Context.Done(): 检查是否取消
//    • Context.Err(): 获取取消原因
//
// 4. 任务计数
//    • sync.WaitGroup: 等待所有任务完成
//    • atomic: 安全的计数器
//    • AddTask()/TaskDone(): 任务生命周期管理
//
// 5. 关闭钩子
//    • 注册需要清理的资源
//    • 按顺序执行（LIFO 或 FIFO）
//    • 错误处理和日志记录
//
// 优雅关闭的重要性：
// • 避免数据丢失（正在处理的消息）
// • 避免资源泄漏（连接、文件等）
// • 避免脏数据（未完成的事务）
// • 提升用户体验（请求不会被中断）
//
// Kubernetes 环境：
//
// 1. Pod 终止流程
//    Step 1: PreStop Hook（可选）
//    Step 2: SIGTERM 信号
//    Step 3: 等待 terminationGracePeriodSeconds（默认 30s）
//    Step 4: SIGKILL 强制终止
//
// 2. 配置示例
//    ```yaml
//    spec:
//      terminationGracePeriodSeconds: 30
//      containers:
//      - name: app
//        lifecycle:
//          preStop:
//            exec:
//              command: ["/bin/sh", "-c", "sleep 5"]
//    ```
//
// 3. Readiness Probe
//    • 关闭时立即设置为 Not Ready
//    • 防止新流量进入
//
// 超时处理策略：
//
// 1. 渐进式超时
//    • 软超时（警告）: 80% 的时间
//    • 硬超时（强制）: 100% 的时间
//
// 2. 示例
//    ```go
//    timeout := 30 * time.Second
//    softTimeout := timeout * 80 / 100  // 24s
//
//    select {
//    case <-done:
//        // 正常完成
//    case <-time.After(softTimeout):
//        log.Warn("接近超时")
//    case <-time.After(timeout):
//        log.Error("强制退出")
//    }
//    ```
//
// 最佳实践：
// ✅ 监听 SIGINT 和 SIGTERM 信号
// ✅ 停止接收新任务
// ✅ 等待正在处理的任务（带超时）
// ✅ 按顺序清理资源
// ✅ 记录关闭日志和统计
// ✅ 设置合理的超时时间（30-60s）
// ✅ 关闭过程中定期输出进度
//
// 注意事项：
// ⚠️ 不要阻塞信号处理
// ⚠️ 超时后要强制退出
// ⚠️ 清理顺序很重要（先停止生产，再清理消费）
// ⚠️ 关闭钩子要处理错误
// ⚠️ 长时间运行的任务要支持取消
// ⚠️ 测试不同信号的行为
//
// 测试方法：
// 1. 正常关闭：kill -SIGTERM <pid>
// 2. 中断关闭：Ctrl+C
// 3. 强制关闭：kill -SIGKILL <pid>
// 4. 超时测试：减少等待时间
//
// Docker 环境：
// • docker stop: 发送 SIGTERM，等待 10 秒，然后 SIGKILL
// • docker stop -t 30: 自定义等待时间
//
// 完整的生产环境关闭流程：
//
// 1. 收到信号
// 2. 设置健康检查为 Not Ready
// 3. 等待负载均衡器移除实例（5-10s）
// 4. 停止接收新请求
// 5. 等待正在处理的请求（20-30s）
// 6. 刷新缓冲区（日志、指标）
// 7. 关闭数据库连接
// 8. 关闭消息队列连接
// 9. 关闭文件句柄
// 10. 退出程序
