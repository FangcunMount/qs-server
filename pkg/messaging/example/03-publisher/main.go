// Package main 演示 Publisher 的使用
// 发布消息、批量发布、发布选项
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
	log.Println("=== Publisher 演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	// 启动消费者统计
	setupConsumer(bus)

	// ========== 演示 1: 基础发布 ==========
	demonstrateBasicPublish(bus)
	time.Sleep(2 * time.Second)

	// ========== 演示 2: 发布带 Metadata 的消息 ==========
	demonstratePublishWithMetadata(bus)
	time.Sleep(2 * time.Second)

	// ========== 演示 3: 批量发布 ==========
	demonstrateBatchPublish(bus)
	time.Sleep(3 * time.Second)

	// ========== 演示 4: 发布到多个 Topic ==========
	demonstrateMultiTopicPublish(bus)
	time.Sleep(2 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

func setupConsumer(bus messaging.EventBus) {
	logger := log.New(os.Stdout, "[Consumer] ", log.LstdFlags)
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var count int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		atomic.AddInt32(&count, 1)
		log.Printf("  ✅ 接收消息 #%d: %s", atomic.LoadInt32(&count), string(msg.Payload))
		if len(msg.Metadata) > 0 {
			log.Printf("     Metadata: %+v", msg.Metadata)
		}
		return msg.Ack()
	}

	// 注册多个 Topic 的处理器
	router.AddHandler("demo.publisher", "publisher-demo", handler)
	router.AddHandler("demo.batch", "batch-demo", handler)
	router.AddHandler("topic.1", "topic1-demo", handler)
	router.AddHandler("topic.2", "topic2-demo", handler)
	router.AddHandler("topic.3", "topic3-demo", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)
}

// ========== 演示 1: 基础发布 ==========

func demonstrateBasicPublish(bus messaging.EventBus) {
	log.Println("【演示 1】基础发布 - Publish() 方法")

	publisher := bus.Publisher()

	// 发布简单消息
	log.Println("发布 3 条简单消息...")
	for i := 1; i <= 3; i++ {
		data := []byte(fmt.Sprintf("简单消息-%d", i))
		err := publisher.Publish(context.Background(), "demo.publisher", data)
		if err != nil {
			log.Printf("  ❌ 发布失败: %v", err)
		} else {
			log.Printf("  → 发送: 简单消息-%d", i)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ========== 演示 2: 发布带 Metadata 的消息 ==========

func demonstratePublishWithMetadata(bus messaging.EventBus) {
	log.Println("\n【演示 2】发布带 Metadata 的消息 - PublishMessage() 方法")

	publisher := bus.Publisher()

	// 发布带元数据的消息
	log.Println("发布带 Metadata 的消息...")

	messages := []struct {
		content  string
		priority int
		source   string
	}{
		{"紧急通知", 10, "system"},
		{"普通消息", 5, "user"},
		{"低优先级任务", 1, "cron"},
	}

	for _, m := range messages {
		msg := messaging.NewMessage("", []byte(m.content))
		msg.Metadata["priority"] = fmt.Sprintf("%d", m.priority)
		msg.Metadata["source"] = m.source
		msg.Metadata["timestamp"] = fmt.Sprintf("%d", time.Now().Unix())

		err := publisher.PublishMessage(context.Background(), "demo.publisher", msg)
		if err != nil {
			log.Printf("  ❌ 发布失败: %v", err)
		} else {
			log.Printf("  → 发送: %s (priority=%d, source=%s)", m.content, m.priority, m.source)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ========== 演示 3: 批量发布 ==========

func demonstrateBatchPublish(bus messaging.EventBus) {
	log.Println("\n【演示 3】批量发布 - 提升性能")

	publisher := bus.Publisher()

	// 批量发布
	batchSize := 10
	log.Printf("批量发布 %d 条消息...\n", batchSize)

	start := time.Now()
	for i := 1; i <= batchSize; i++ {
		data := []byte(fmt.Sprintf("批量消息-%d", i))
		publisher.Publish(context.Background(), "demo.batch", data)
	}
	duration := time.Since(start)

	log.Printf("  ✅ 批量发布完成，耗时: %v\n", duration)
	log.Printf("  平均每条: %.2f ms\n", float64(duration.Milliseconds())/float64(batchSize))
}

// ========== 演示 4: 发布到多个 Topic ==========

func demonstrateMultiTopicPublish(bus messaging.EventBus) {
	log.Println("\n【演示 4】发布到多个 Topic - 扇出模式")

	publisher := bus.Publisher()

	// 同一消息发送到多个 Topic
	topics := []string{"topic.1", "topic.2", "topic.3"}
	data := []byte("广播消息：系统维护通知")

	log.Println("发送广播消息到多个 Topic...")
	for _, topic := range topics {
		err := publisher.Publish(context.Background(), topic, data)
		if err != nil {
			log.Printf("  ❌ 发送到 %s 失败: %v", topic, err)
		} else {
			log.Printf("  → 发送到 %s", topic)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// 核心知识点：
//
// 1. Publisher 接口
//    type Publisher interface {
//        Publish(ctx context.Context, topic string, data []byte) error
//        PublishMessage(ctx context.Context, topic string, msg *Message) error
//    }
//
// 2. Publish() vs PublishMessage()
//    • Publish(): 简单快捷，直接发送字节数组
//    • PublishMessage(): 灵活强大，可以携带 Metadata
//
// 3. Metadata 的用途
//    • 优先级（priority）
//    • 来源（source）
//    • 时间戳（timestamp）
//    • 追踪 ID（trace_id）
//    • 自定义字段
//
// 4. 批量发布的优势
//    • 减少网络往返
//    • 提升吞吐量
//    • 降低延迟
//
// 5. 发布模式
//    • 点对点（Point-to-Point）: 单个 Topic
//    • 扇出（Fan-out）: 多个 Topic
//    • 广播（Broadcast）: 所有订阅者
//
// 应用场景：
//
// 1. Publish() - 简单场景
//    • 日志收集
//    • 事件通知
//    • 简单消息传递
//
// 2. PublishMessage() - 复杂场景
//    • 需要携带元数据
//    • 需要追踪和监控
//    • 需要优先级控制
//
// 3. 批量发布 - 高吞吐场景
//    • 数据导入
//    • 批量任务分发
//    • 大量事件上报
//
// 4. 多 Topic 发布 - 扇出场景
//    • 系统广播通知
//    • 多服务协调
//    • 事件驱动架构
//
// 最佳实践：
// ✅ 小消息用 Publish()，复杂消息用 PublishMessage()
// ✅ 批量发布要控制批次大小（避免内存占用）
// ✅ 使用 Context 控制超时
// ✅ 处理发布失败（重试或记录日志）
// ✅ 添加 Metadata 便于追踪和监控
// ✅ 大消息考虑压缩或分片
//
// 注意事项：
// ⚠️ 发布是异步的，不保证立即到达
// ⚠️ 批量发布要注意内存占用
// ⚠️ 多 Topic 发布会增加网络开销
// ⚠️ 发布失败要有容错机制
// ⚠️ Metadata 不要过大（影响性能）
//
// 性能建议：
// • 单条发布: < 1ms
// • 批量发布（100条）: < 10ms
// • Payload 大小: < 1MB（建议）
// • Metadata 大小: < 1KB（建议）
