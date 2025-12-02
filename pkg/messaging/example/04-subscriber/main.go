// Package main 演示 Subscriber 的使用
// 订阅消息、多订阅者、订阅选项
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
	log.Println("=== Subscriber 演示 ===")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	// ========== 演示 1: 基础订阅 ==========
	demonstrateBasicSubscribe(bus)
	time.Sleep(3 * time.Second)

	// ========== 演示 2: 多订阅者（负载均衡）==========
	demonstrateMultiSubscriber(bus)
	time.Sleep(5 * time.Second)

	// ========== 演示 3: 订阅多个 Topic ==========
	demonstrateMultiTopicSubscribe(bus)
	time.Sleep(3 * time.Second)

	// ========== 演示 4: 订阅者生命周期 ==========
	demonstrateSubscriberLifecycle(bus)
	time.Sleep(3 * time.Second)

	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// ========== 演示 1: 基础订阅 ==========

func demonstrateBasicSubscribe(bus messaging.EventBus) {
	log.Println("【演示 1】基础订阅 - Subscribe() 方法")

	logger := log.New(os.Stdout, "[Basic] ", log.LstdFlags)
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var count int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		atomic.AddInt32(&count, 1)
		log.Printf("  ✅ 处理消息 #%d: %s", atomic.LoadInt32(&count), string(msg.Payload))
		return msg.Ack()
	}

	// 订阅 Topic
	router.AddHandler("demo.basic", "basic-subscriber", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发布消息
	log.Println("发布 3 条消息...")
	publisher := bus.Publisher()
	for i := 1; i <= 3; i++ {
		data := []byte(fmt.Sprintf("消息-%d", i))
		publisher.Publish(context.Background(), "demo.basic", data)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(time.Second)
	router.Stop()
	log.Printf("\n总共处理: %d 条消息\n", atomic.LoadInt32(&count))
}

// ========== 演示 2: 多订阅者（负载均衡）==========

func demonstrateMultiSubscriber(bus messaging.EventBus) {
	log.Println("\n【演示 2】多订阅者 - 负载均衡模式")

	logger := log.New(os.Stdout, "[Multi] ", log.LstdFlags)
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	// 创建 3 个订阅者（使用不同的 channel）
	for i := 1; i <= 3; i++ {
		subscriberID := i
		var count int32

		handler := func(ctx context.Context, msg *messaging.Message) error {
			atomic.AddInt32(&count, 1)
			log.Printf("  订阅者-%d 处理消息 #%d: %s",
				subscriberID,
				atomic.LoadInt32(&count),
				string(msg.Payload))
			time.Sleep(500 * time.Millisecond) // 模拟处理时间
			return msg.Ack()
		}

		router.AddHandler("demo.multi", fmt.Sprintf("subscriber-%d", i), handler)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发布 10 条消息
	log.Println("发布 10 条消息（观察负载均衡）...")
	publisher := bus.Publisher()
	for i := 1; i <= 10; i++ {
		data := []byte(fmt.Sprintf("任务-%d", i))
		publisher.Publish(context.Background(), "demo.multi", data)
		time.Sleep(200 * time.Millisecond)
	}

	time.Sleep(3 * time.Second)
	router.Stop()
	log.Println("\n每个订阅者处理了约 3-4 条消息（负载均衡）")
}

// ========== 演示 3: 订阅多个 Topic ==========

func demonstrateMultiTopicSubscribe(bus messaging.EventBus) {
	log.Println("\n【演示 3】订阅多个 Topic - 统一处理")

	logger := log.New(os.Stdout, "[MultiTopic] ", log.LstdFlags)
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var count int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		atomic.AddInt32(&count, 1)
		topic := msg.Metadata["topic"]
		log.Printf("  ✅ [%s] 处理消息 #%d: %s",
			topic,
			atomic.LoadInt32(&count),
			string(msg.Payload))
		return msg.Ack()
	}

	// 订阅多个 Topic
	topics := []string{"order.created", "order.paid", "order.shipped"}
	for _, topic := range topics {
		router.AddHandler(topic, "multi-topic-subscriber", handler)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发布到不同 Topic
	log.Println("发布到不同的 Topic...")
	publisher := bus.Publisher()

	events := []struct {
		topic string
		data  string
	}{
		{"order.created", "订单创建: ORDER-001"},
		{"order.paid", "订单支付: ORDER-001"},
		{"order.shipped", "订单发货: ORDER-001"},
	}

	for _, event := range events {
		msg := messaging.NewMessage("", []byte(event.data))
		msg.Metadata["topic"] = event.topic
		publisher.PublishMessage(context.Background(), event.topic, msg)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(time.Second)
	router.Stop()
	log.Printf("\n总共处理: %d 条消息\n", atomic.LoadInt32(&count))
}

// ========== 演示 4: 订阅者生命周期 ==========

func demonstrateSubscriberLifecycle(bus messaging.EventBus) {
	log.Println("\n【演示 4】订阅者生命周期 - 启动、暂停、恢复、停止")

	logger := log.New(os.Stdout, "[Lifecycle] ", log.LstdFlags)
	router := bus.Router()
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	var count int32
	handler := func(ctx context.Context, msg *messaging.Message) error {
		atomic.AddInt32(&count, 1)
		log.Printf("  ✅ 处理消息 #%d: %s", atomic.LoadInt32(&count), string(msg.Payload))
		return msg.Ack()
	}

	router.AddHandler("demo.lifecycle", "lifecycle-subscriber", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	publisher := bus.Publisher()

	// 阶段 1: 启动订阅者
	log.Println("阶段 1: 启动订阅者")
	go router.Run(ctx)
	time.Sleep(time.Second)

	// 发布消息
	log.Println("发布消息...")
	for i := 1; i <= 3; i++ {
		data := []byte(fmt.Sprintf("消息-%d", i))
		publisher.Publish(context.Background(), "demo.lifecycle", data)
		time.Sleep(300 * time.Millisecond)
	}

	time.Sleep(time.Second)

	// 阶段 2: 暂停订阅者（取消上下文）
	log.Println("\n阶段 2: 暂停订阅者")
	cancel()
	router.Stop()
	time.Sleep(time.Second)

	// 阶段 3: 恢复订阅者（重新启动）
	log.Println("\n阶段 3: 恢复订阅者")
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)
	time.Sleep(time.Second)

	// 再次发布消息
	log.Println("再次发布消息...")
	for i := 4; i <= 6; i++ {
		data := []byte(fmt.Sprintf("消息-%d", i))
		publisher.Publish(context.Background(), "demo.lifecycle", data)
		time.Sleep(300 * time.Millisecond)
	}

	time.Sleep(time.Second)

	// 阶段 4: 停止订阅者
	log.Println("\n阶段 4: 停止订阅者")
	cancel()
	router.Stop()

	log.Printf("\n总共处理: %d 条消息\n", atomic.LoadInt32(&count))
}

// 核心知识点：
//
// 1. Subscriber 接口
//    type Subscriber interface {
//        Subscribe(ctx context.Context, topic, channel string, handler Handler) error
//        SubscribeWithMiddleware(ctx context.Context, topic, channel string, handler Handler, middlewares ...Middleware) error
//    }
//
// 2. Channel 的概念
//    • Channel 是消费者组的标识
//    • 同一 Channel 的多个订阅者会负载均衡
//    • 不同 Channel 的订阅者会收到相同的消息（广播）
//
// 3. 订阅模式
//    • 负载均衡（Load Balancing）: 同一 Channel 的多个订阅者
//    • 广播（Broadcasting）: 不同 Channel 的订阅者
//    • 扇出（Fan-out）: 订阅多个 Topic
//
// 4. 订阅者生命周期
//    • 启动（Start）: 开始接收消息
//    • 运行（Running）: 处理消息中
//    • 暂停（Pause）: 停止接收新消息（Context 取消）
//    • 恢复（Resume）: 重新开始接收
//    • 停止（Stop）: 关闭连接
//
// 5. Router 的作用
//    • 统一管理多个订阅者
//    • 应用全局中间件
//    • 协调生命周期
//
// 应用场景：
//
// 1. 单订阅者
//    • 简单的消息处理
//    • 日志收集
//    • 事件监听
//
// 2. 多订阅者（同 Channel）
//    • 任务队列（负载均衡）
//    • 水平扩展
//    • 提升吞吐量
//
// 3. 多订阅者（不同 Channel）
//    • 多服务协同
//    • 广播通知
//    • 事件驱动架构
//
// 4. 订阅多个 Topic
//    • 统一事件处理
//    • 跨模块监听
//    • 聚合处理
//
// 负载均衡策略：
//
// NSQ 负载均衡:
// • Round-Robin（轮询）
// • 自动发现和重平衡
// • 每个消息只被一个订阅者处理
//
// RabbitMQ 负载均衡:
// • Fair Dispatch（公平分发）
// • Prefetch Count 控制
// • 基于 ACK 的流控
//
// 最佳实践：
// ✅ 使用有意义的 Channel 名称（如 "email-service"）
// ✅ 同一服务的多实例使用相同 Channel（负载均衡）
// ✅ 不同服务使用不同 Channel（广播）
// ✅ 处理失败要 Nack（让其他订阅者重试）
// ✅ 使用 Context 控制生命周期
// ✅ 添加中间件（日志、监控、重试）
// ✅ 订阅多个 Topic 要注意性能
//
// 注意事项：
// ⚠️ Channel 名称要唯一且有意义
// ⚠️ 多订阅者会增加资源消耗
// ⚠️ 订阅多个 Topic 要避免重复处理
// ⚠️ 暂停后的消息会积压
// ⚠️ 停止订阅者要处理未完成的消息
// ⚠️ Context 取消会中断处理中的消息
//
// 性能考虑：
// • 单订阅者: 顺序处理，吞吐量有限
// • 多订阅者: 并行处理，吞吐量提升
// • 订阅多 Topic: 增加网络和处理开销
// • Channel 数量: 不要过多（每个 Channel 一个连接）
//
// 错误处理：
// • Ack(): 消息处理成功
// • Nack(): 消息处理失败，重新入队
// • 超时: 自动 Requeue
// • Panic: 使用 RecoverMiddleware 捕获
//
// 监控指标：
// • 消息处理速率
// • 消息积压数量
// • 处理成功率
// • 处理延迟
// • 订阅者数量
