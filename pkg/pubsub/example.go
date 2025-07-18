package pubsub

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ExampleMessage 示例消息结构
type ExampleMessage struct {
	ID      string    `json:"id"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// RunExample 运行发布订阅示例
func RunExample() {
	ctx := context.Background()

	// 创建配置
	config := DefaultConfig()
	config.Addr = "localhost:6379"
	config.ConsumerGroup = "example-group"
	config.Consumer = "example-consumer"

	// 创建发布订阅实例
	ps, err := NewPubSub(config)
	if err != nil {
		log.Fatalf("Failed to create pubsub: %v", err)
	}
	defer ps.Close()

	// 定义消息处理器
	messageHandler := func(topic string, data []byte) error {
		fmt.Printf("Received message on topic %s: %s\n", topic, string(data))
		return nil
	}

	// 订阅主题
	topic := "example-topic"
	if err := ps.Subscriber().Subscribe(ctx, topic, messageHandler); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 启动订阅者（在 goroutine 中运行）
	go func() {
		if err := ps.Subscriber().Run(ctx); err != nil {
			log.Printf("Subscriber error: %v", err)
		}
	}()

	// 等待一段时间让订阅者启动
	time.Sleep(time.Second * 2)

	// 发布消息
	for i := 0; i < 5; i++ {
		message := ExampleMessage{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: fmt.Sprintf("Hello PubSub %d", i),
			Time:    time.Now(),
		}

		if err := ps.Publisher().Publish(ctx, topic, message); err != nil {
			log.Printf("Failed to publish message: %v", err)
		}

		time.Sleep(time.Second)
	}

	// 等待消息处理完成
	time.Sleep(time.Second * 5)
}

// RunRetryExample 运行带重试的发布订阅示例
func RunRetryExample() {
	ctx := context.Background()

	// 创建配置
	config := DefaultConfig()
	config.Addr = "localhost:6379"
	config.ConsumerGroup = "retry-group"
	config.Consumer = "retry-consumer"
	config.MaxRetries = 3
	config.InitialInterval = time.Millisecond * 100
	config.MaxInterval = time.Second * 2

	// 创建发布订阅实例
	ps, err := NewPubSub(config)
	if err != nil {
		log.Fatalf("Failed to create pubsub: %v", err)
	}
	defer ps.Close()

	// 定义会失败的消息处理器（前两次失败，第三次成功）
	attemptCount := 0
	retryHandler := func(topic string, data []byte) error {
		attemptCount++
		if attemptCount < 3 {
			return fmt.Errorf("simulated failure (attempt %d)", attemptCount)
		}
		fmt.Printf("Successfully processed message on attempt %d: %s\n", attemptCount, string(data))
		return nil
	}

	// 订阅主题（带重试）
	topic := "retry-topic"
	if err := ps.Subscriber().SubscribeWithRetry(ctx, topic, retryHandler); err != nil {
		log.Fatalf("Failed to subscribe with retry: %v", err)
	}

	// 启动订阅者
	go func() {
		if err := ps.Subscriber().Run(ctx); err != nil {
			log.Printf("Subscriber error: %v", err)
		}
	}()

	// 等待订阅者启动
	time.Sleep(time.Second * 2)

	// 发布一条消息
	message := ExampleMessage{
		ID:      "retry-msg-1",
		Content: "This message will fail twice before succeeding",
		Time:    time.Now(),
	}

	if err := ps.Publisher().Publish(ctx, topic, message); err != nil {
		log.Printf("Failed to publish message: %v", err)
	}

	// 等待消息处理完成
	time.Sleep(time.Second * 10)
}
