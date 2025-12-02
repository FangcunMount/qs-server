// Package main 演示如何在不同消息中间件之间切换
// 包括：NSQ、RabbitMQ 的切换和混合使用
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
_ "github.com/FangcunMount/qs-server/pkg/messaging/rabbitmq"
)

func main() {
	log.Println("=== 多 Provider 演示 ===")

	// ========== 演示 1: 切换 Provider ==========
	// demonstrateProviderSwitch()
	// time.Sleep(3 * time.Second)

	// ========== 演示 2: 混合使用多个 Provider ==========
	// demonstrateMultiProvider()

	log.Println("\n提示: 取消注释上面的演示代码并确保相应的消息中间件正在运行")
	log.Println("  - NSQ: 运行 `nsqlookupd` 和 `nsqd`")
	log.Println("  - RabbitMQ: 运行 `rabbitmq-server`")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// ========== 演示 1: 切换 Provider ==========

func demonstrateProviderSwitch() {
	log.Println("【演示 1】切换 Provider - NSQ vs RabbitMQ")

	// 配置 NSQ
	nsqConfig := &messaging.Config{
		Provider: "nsq",
		NSQ: messaging.NSQConfig{
			NSQdAddr:     "127.0.0.1:4150",
			LookupdAddrs: []string{"127.0.0.1:4161"},
		},
	}

	// 配置 RabbitMQ
	rabbitConfig := &messaging.Config{
		Provider: "rabbitmq",
		RabbitMQ: messaging.RabbitMQConfig{
			URL: "amqp://guest:guest@localhost:5672/",
		},
	}

	// 使用 NSQ
	log.Println("使用 NSQ Provider:")
	testProvider(nsqConfig, "nsq")

	time.Sleep(2 * time.Second)

	// 使用 RabbitMQ
	log.Println("\n使用 RabbitMQ Provider:")
	testProvider(rabbitConfig, "rabbitmq")
}

func testProvider(config *messaging.Config, name string) {
	bus, err := messaging.NewEventBus(config)
	if err != nil {
		log.Printf("  ❌ 创建 %s EventBus 失败: %v", name, err)
		return
	}
	defer bus.Close()

	// 创建订阅者
	router := bus.Router()
	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  [%s] → 收到消息: %s", name, string(msg.Payload))
		return msg.Ack()
	}

	router.AddHandler("demo.provider", "provider-test", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发送消息
	for i := 1; i <= 3; i++ {
		data := []byte(fmt.Sprintf("消息 #%d via %s", i, name))
		err := bus.Publisher().Publish(context.Background(), "demo.provider", data)
		if err != nil {
			log.Printf("  ❌ 发送失败: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(time.Second)
	router.Stop()
}

// ========== 演示 2: 混合使用多个 Provider ==========

// MessageBridge 消息桥接器（连接两个不同的 EventBus）
type MessageBridge struct {
	sourceBus  messaging.EventBus
	targetBus  messaging.EventBus
	sourceName string
	targetName string
}

// NewMessageBridge 创建消息桥接器
func NewMessageBridge(sourceConfig, targetConfig *messaging.Config, sourceName, targetName string) (*MessageBridge, error) {
	sourceBus, err := messaging.NewEventBus(sourceConfig)
	if err != nil {
		return nil, fmt.Errorf("创建源 EventBus 失败: %w", err)
	}

	targetBus, err := messaging.NewEventBus(targetConfig)
	if err != nil {
		sourceBus.Close()
		return nil, fmt.Errorf("创建目标 EventBus 失败: %w", err)
	}

	return &MessageBridge{
		sourceBus:  sourceBus,
		targetBus:  targetBus,
		sourceName: sourceName,
		targetName: targetName,
	}, nil
}

// Start 启动桥接器
func (b *MessageBridge) Start(sourceTopic, targetTopic string) error {
	router := b.sourceBus.Router()

	// 订阅源消息并转发到目标
	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  [桥接] %s → %s: %s",
b.sourceName, b.targetName, string(msg.Payload))

		// 转发到目标 Provider
		err := b.targetBus.Publisher().Publish(ctx, targetTopic, msg.Payload)
		if err != nil {
			log.Printf("  ❌ 转发失败: %v", err)
			return msg.Nack()
		}

		return msg.Ack()
	}

	router.AddHandler(sourceTopic, "bridge-worker", handler)

	ctx := context.Background()
	go router.Run(ctx)

	return nil
}

// Stop 停止桥接器
func (b *MessageBridge) Stop() {
	b.sourceBus.Router().Stop()
	b.sourceBus.Close()
	b.targetBus.Close()
}

func demonstrateMultiProvider() {
	log.Println("【演示 2】混合使用多个 Provider")
	log.Println("场景: NSQ 接收消息 → 桥接 → RabbitMQ 处理")

	// NSQ 配置（作为入口）
	nsqConfig := &messaging.Config{
		Provider: "nsq",
		NSQ: messaging.NSQConfig{
			NSQdAddr:     "127.0.0.1:4150",
			LookupdAddrs: []string{"127.0.0.1:4161"},
		},
	}

	// RabbitMQ 配置（作为处理）
	rabbitConfig := &messaging.Config{
		Provider: "rabbitmq",
		RabbitMQ: messaging.RabbitMQConfig{
			URL: "amqp://guest:guest@localhost:5672/",
		},
	}

	// 创建桥接器
	bridge, err := NewMessageBridge(nsqConfig, rabbitConfig, "NSQ", "RabbitMQ")
	if err != nil {
		log.Fatalf("创建桥接器失败: %v", err)
	}
	defer bridge.Stop()

	// 启动桥接
	bridge.Start("demo.source", "demo.target")

	// 在 RabbitMQ 端创建消费者
	rabbitBus, _ := messaging.NewEventBus(rabbitConfig)
	defer rabbitBus.Close()

	router := rabbitBus.Router()
	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  [RabbitMQ处理器] → 最终处理: %s", string(msg.Payload))
		return msg.Ack()
	}

	router.AddHandler("demo.target", "final-handler", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(2 * time.Second)

	// 从 NSQ 发送消息
	nsqBus, _ := messaging.NewEventBus(nsqConfig)
	defer nsqBus.Close()

	log.Println("从 NSQ 发送消息...")

	for i := 1; i <= 5; i++ {
		data := []byte(fmt.Sprintf("跨 Provider 消息 #%d", i))
		nsqBus.Publisher().Publish(context.Background(), "demo.source", data)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(3 * time.Second)
	router.Stop()

	log.Println("\n桥接演示完成！")
	log.Println("消息流: NSQ → 桥接器 → RabbitMQ → 处理器")
}
