// Package main 演示 Message 消息模型的完整功能
// 学习 UUID、Metadata、Payload、Ack/Nack
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

func main() {
	log.Println("=== Message 消息模型详解 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	// ========== 演示 1: UUID 全局唯一标识 ==========
	demonstrateUUID(bus)
	time.Sleep(2 * time.Second)

	// ========== 演示 2: Metadata 元数据 ==========
	demonstrateMetadata(bus)
	time.Sleep(2 * time.Second)

	// ========== 演示 3: Ack/Nack 确认机制 ==========
	demonstrateAckNack(bus)
	time.Sleep(2 * time.Second)

	// 等待退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// demonstrateUUID 演示 UUID 的使用
func demonstrateUUID(bus messaging.EventBus) {
	log.Println("【演示 1】UUID - 全局唯一标识")
	log.Println("用途：消息追踪、去重、日志关联")

	// 订阅
	bus.Subscriber().Subscribe("demo.uuid", "uuid-demo", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  收到消息:")
		log.Printf("    UUID    : %s", msg.UUID)
		log.Printf("    Payload : %s", string(msg.Payload))
		log.Printf("    Attempts: %d\n", msg.Attempts)
		return msg.Ack()
	})

	time.Sleep(time.Second)

	// 发布消息（自动生成 UUID）
	publisher := bus.Publisher()
	for i := 1; i <= 2; i++ {
		msg := messaging.NewMessage("", []byte(fmt.Sprintf("消息 #%d", i)))
		// msg.UUID 会被 Adapter 自动填充
		publisher.PublishMessage(context.Background(), "demo.uuid", msg)
		log.Printf("✓ 发布消息 #%d\n", i)
		time.Sleep(500 * time.Millisecond)
	}
}

// demonstrateMetadata 演示 Metadata 的使用
func demonstrateMetadata(bus messaging.EventBus) {
	log.Println("\n【演示 2】Metadata - 元数据")
	log.Println("用途：链路追踪、业务标识、消息路由")

	// 订阅
	bus.Subscriber().Subscribe("demo.metadata", "metadata-demo", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  收到消息:")
		log.Printf("    Payload  : %s", string(msg.Payload))
		log.Printf("    Metadata :")
		for k, v := range msg.Metadata {
			log.Printf("      %s = %s", k, v)
		}
		log.Println()
		return msg.Ack()
	})

	time.Sleep(time.Second)

	// 发布带 Metadata 的消息
	publisher := bus.Publisher()
	msg := messaging.NewMessage("", []byte("订单创建成功"))

	// 添加元数据
	msg.Metadata["trace_id"] = "trace-abc-123"                  // 链路追踪
	msg.Metadata["user_id"] = "1001"                            // 业务标识
	msg.Metadata["source"] = "web"                              // 来源
	msg.Metadata["priority"] = "high"                           // 优先级
	msg.Metadata["timestamp"] = time.Now().Format(time.RFC3339) // 时间戳

	publisher.PublishMessage(context.Background(), "demo.metadata", msg)
	log.Println("✓ 发布带 Metadata 的消息")
}

// demonstrateAckNack 演示 Ack/Nack 确认机制
func demonstrateAckNack(bus messaging.EventBus) {
	log.Println("\n【演示 3】Ack/Nack - 消息确认机制")
	log.Println("Ack  : 确认消息处理成功，不会重新投递")
	log.Println("Nack : 拒绝消息，触发重试（NSQ 会自动重新投递）")

	processCount := 0

	// 订阅
	bus.Subscriber().Subscribe("demo.ack-nack", "ack-demo", func(ctx context.Context, msg *messaging.Message) error {
		processCount++
		log.Printf("  处理消息 (第 %d 次): %s", processCount, string(msg.Payload))

		// 模拟：前 2 次处理失败，第 3 次成功
		if processCount < 3 {
			log.Println("  ❌ 处理失败，触发 Nack（将重试）")
			return msg.Nack()
		}

		log.Println("  ✅ 处理成功，触发 Ack")
		return msg.Ack()
	})

	time.Sleep(time.Second)

	// 发布消息
	publisher := bus.Publisher()
	publisher.Publish(context.Background(), "demo.ack-nack", []byte("需要重试的消息"))
	log.Println("✓ 发布消息（观察重试机制）")

	time.Sleep(3 * time.Second)
}

// 核心知识点：
//
// 1. UUID（全局唯一标识）
//    - 自动生成或手动指定
//    - 用于消息追踪、去重
//    - 贯穿整个消息生命周期
//
// 2. Metadata（元数据）
//    - 键值对形式存储额外信息
//    - 不影响消息体（Payload）
//    - 常见用途：
//      • trace_id - 分布式追踪
//      • user_id - 用户标识
//      • tenant_id - 租户隔离
//      • priority - 优先级
//      • source - 消息来源
//
// 3. Payload（消息体）
//    - 实际的业务数据
//    - []byte 类型，可以是 JSON、Protobuf 等
//    - 建议使用结构化格式（JSON）
//
// 4. Ack/Nack（确认机制）
//    - Ack : 处理成功，消息不会重新投递
//    - Nack: 处理失败，消息会重新投递
//    - 需要手动调用（否则消息会超时重试）
//
// 最佳实践：
// ✅ 总是调用 Ack() 或 Nack()
// ✅ 使用 Metadata 传递追踪信息
// ✅ 保持 Payload 结构化（JSON）
// ✅ 记录 UUID 用于调试
