// Package main 演示如何编写自定义中间件
// 包括：认证中间件、审计中间件、批处理中间件
package main

import (
"context"
"crypto/md5"
"encoding/json"
"fmt"
"log"
"os"
"os/signal"
"sync"
"syscall"
"time"

"github.com/FangcunMount/qs-server/pkg/messaging"
_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

func main() {
	log.Println("=== 自定义中间件演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	logger := log.New(os.Stdout, "[Custom] ", log.LstdFlags)

	// ========== 演示 1: 认证中间件 ==========
	demonstrateAuth(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 2: 审计中间件 ==========
	demonstrateAudit(bus, logger)
	time.Sleep(3 * time.Second)

	// ========== 演示 3: 批处理中间件 ==========
	demonstrateBatch(bus, logger)
	time.Sleep(5 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// ========== 自定义中间件 1: 认证中间件 ==========

// AuthMiddleware 验证消息的签名
// 使用场景：需要验证消息来源的合法性
func AuthMiddleware(secretKey string) messaging.Middleware {
	return func(next messaging.Handler) messaging.Handler {
		return func(ctx context.Context, msg *messaging.Message) error {
			// 1. 从 Metadata 中获取签名
			signature := msg.Metadata["signature"]
			if signature == "" {
				log.Println("  ❌ 认证失败: 缺少签名")
				return msg.Nack()
			}

			// 2. 计算期望的签名
			expected := calculateSignature(msg.Payload, secretKey)

			// 3. 验证签名
			if signature != expected {
				log.Println("  ❌ 认证失败: 签名不匹配")
				return msg.Nack()
			}

			log.Println("  ✅ 认证通过")
			return next(ctx, msg)
		}
	}
}

// calculateSignature 计算消息签名（简化版）
func calculateSignature(payload []byte, key string) string {
	h := md5.New()
	h.Write(payload)
	h.Write([]byte(key))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func demonstrateAuth(bus messaging.EventBus, logger *log.Logger) {
	log.Println("【演示 1】认证中间件 - 验证消息签名")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Println("  → 处理已认证的消息")
		return msg.Ack()
	}

	secretKey := "my-secret-key-123"

	// 使用认证中间件
	router.AddHandlerWithMiddleware(
"demo.auth",
"auth-demo",
handler,
AuthMiddleware(secretKey),
)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 测试 1: 发送带有效签名的消息
	log.Println("测试 1: 发送带有效签名的消息")
	payload := []byte("敏感数据")
	validMsg := messaging.NewMessage("valid-msg-1", payload)
	validMsg.Metadata["signature"] = calculateSignature(payload, secretKey)
	bus.Publisher().PublishMessage(context.Background(), "demo.auth", validMsg)

	time.Sleep(time.Second)

	// 测试 2: 发送无签名的消息
	log.Println("\n测试 2: 发送无签名的消息")
	invalidMsg := messaging.NewMessage("invalid-msg-1", []byte("无签名数据"))
	bus.Publisher().PublishMessage(context.Background(), "demo.auth", invalidMsg)

	time.Sleep(time.Second)

	// 测试 3: 发送错误签名的消息
	log.Println("\n测试 3: 发送错误签名的消息")
	wrongMsg := messaging.NewMessage("wrong-msg-1", []byte("错误签名"))
	wrongMsg.Metadata["signature"] = "wrong-signature"
	bus.Publisher().PublishMessage(context.Background(), "demo.auth", wrongMsg)

	time.Sleep(time.Second)
	router.Stop()
}

// ========== 自定义中间件 2: 审计中间件 ==========

// AuditRecord 审计记录
type AuditRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Topic     string    `json:"topic"`
	MessageID string    `json:"message_id"`
	User      string    `json:"user"`
	Action    string    `json:"action"`
	Duration  int64     `json:"duration_ms"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// AuditMiddleware 记录消息处理的审计日志
// 使用场景：合规审计、操作追踪
func AuditMiddleware(auditLog *os.File) messaging.Middleware {
	encoder := json.NewEncoder(auditLog)

	return func(next messaging.Handler) messaging.Handler {
		return func(ctx context.Context, msg *messaging.Message) error {
			start := time.Now()

			// 提取审计信息
			topic := msg.Metadata["topic"]
			user := msg.Metadata["user"]
			action := msg.Metadata["action"]

			// 调用下一个处理器
			err := next(ctx, msg)

			// 记录审计日志
			record := AuditRecord{
				Timestamp: start,
				Topic:     topic,
				MessageID: msg.UUID,
				User:      user,
				Action:    action,
				Duration:  time.Since(start).Milliseconds(),
				Success:   err == nil,
			}

			if err != nil {
				record.Error = err.Error()
			}

			encoder.Encode(record)
			return err
		}
	}
}

func demonstrateAudit(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 2】审计中间件 - 记录操作日志")

	// 创建审计日志文件
	auditLog, _ := os.Create("audit.log")
	defer auditLog.Close()

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  → 执行操作: %s", string(msg.Payload))
		time.Sleep(200 * time.Millisecond)
		return msg.Ack()
	}

	// 使用审计中间件
	router.AddHandlerWithMiddleware(
"demo.audit",
"audit-demo",
handler,
AuditMiddleware(auditLog),
)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送需要审计的操作...")

	operations := []struct {
		user   string
		action string
		data   string
	}{
		{"admin", "删除用户", "用户ID=123"},
		{"user1", "修改密码", "用户ID=456"},
		{"admin", "导出数据", "数据范围=全部"},
	}

	for _, op := range operations {
		msg := messaging.NewMessage("", []byte(op.data))
		msg.Metadata["topic"] = "demo.audit"
		msg.Metadata["user"] = op.user
		msg.Metadata["action"] = op.action

		log.Printf("操作: user=%s, action=%s", op.user, op.action)
		bus.Publisher().PublishMessage(context.Background(), "demo.audit", msg)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(time.Second)
	router.Stop()

	log.Println("\n审计日志已写入 audit.log:")
	content, _ := os.ReadFile("audit.log")
	log.Println(string(content))
}

// ========== 自定义中间件 3: 批处理中间件 ==========

// BatchMiddleware 将多个消息合并批量处理
// 使用场景：数据库批量写入、批量 API 调用
func BatchMiddleware(batchSize int, batchTimeout time.Duration) messaging.Middleware {
	return func(next messaging.Handler) messaging.Handler {
		var (
mu       sync.Mutex
batch    []*messaging.Message
timer    *time.Timer
timerSet bool
)

		// 处理批次
		processBatch := func() {
			mu.Lock()
			if len(batch) == 0 {
				mu.Unlock()
				return
			}

			currentBatch := batch
			batch = nil
			timerSet = false
			mu.Unlock()

			log.Printf("  📦 批量处理 %d 条消息", len(currentBatch))

			// 合并 Payload
			var combined []byte
			for _, msg := range currentBatch {
				combined = append(combined, msg.Payload...)
				combined = append(combined, '\n')
			}

			// 创建批量消息
			batchMsg := messaging.NewMessage("", combined)
			batchMsg.Metadata["batch_size"] = fmt.Sprintf("%d", len(currentBatch))

			// 调用下一个处理器
			if err := next(context.Background(), batchMsg); err != nil {
				log.Printf("  ❌ 批处理失败: %v", err)
				for _, msg := range currentBatch {
					msg.Nack()
				}
			} else {
				for _, msg := range currentBatch {
					msg.Ack()
				}
			}
		}

		return func(ctx context.Context, msg *messaging.Message) error {
			mu.Lock()
			batch = append(batch, msg)

			// 启动定时器
			if !timerSet {
				timer = time.AfterFunc(batchTimeout, processBatch)
				timerSet = true
			}

			// 达到批次大小，立即处理
			if len(batch) >= batchSize {
				mu.Unlock()
				if timer != nil {
					timer.Stop()
				}
				processBatch()
				return nil
			}

			mu.Unlock()
			return nil
		}
	}
}

func demonstrateBatch(bus messaging.EventBus, logger *log.Logger) {
	log.Println("\n【演示 3】批处理中间件 - 合并处理消息")

	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	handler := func(ctx context.Context, msg *messaging.Message) error {
		batchSize := msg.Metadata["batch_size"]
		log.Printf("  → 处理批次（包含 %s 条消息）", batchSize)
		log.Printf("  → 数据: %s", string(msg.Payload))
		time.Sleep(500 * time.Millisecond)
		return msg.Ack()
	}

	// 使用批处理中间件：每 5 条或每 2 秒触发一次
	router.AddHandlerWithMiddleware(
"demo.batch",
"batch-demo",
handler,
BatchMiddleware(5, 2*time.Second),
)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	log.Println("发送 12 条消息（观察批处理）...")

	for i := 1; i <= 12; i++ {
		msg := messaging.NewMessage("", []byte(fmt.Sprintf("数据-%d", i)))
		bus.Publisher().PublishMessage(context.Background(), "demo.batch", msg)
		time.Sleep(300 * time.Millisecond)

		if i == 5 {
			log.Println("\n→ 达到批次大小（5 条），触发处理")
		}
		if i == 10 {
			log.Println("\n→ 达到批次大小（5 条），再次触发处理")
		}
	}

	log.Println("\n等待最后一批（超时触发）...")
	time.Sleep(3 * time.Second)
	router.Stop()
}
