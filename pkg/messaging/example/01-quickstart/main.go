// Package main 演示 messaging 包的最简单用法
// 5 分钟快速入门：发布和订阅消息
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq" // 自动注册 NSQ Provider
)

func main() {
	log.Println("=== Messaging 快速入门示例 ===")

	// ========== 步骤 1: 创建配置 ==========
	config := messaging.DefaultConfig()
	log.Println("✓ 使用默认配置（NSQ）")

	// ========== 步骤 2: 创建事件总线 ==========
	bus, err := messaging.NewEventBus(config)
	if err != nil {
		log.Fatalf("创建事件总线失败: %v", err)
	}
	defer bus.Close()
	log.Println("✓ 事件总线创建成功")

	// ========== 步骤 3: 订阅消息 ==========
	subscriber := bus.Subscriber()
	err = subscriber.Subscribe("hello", "quickstart", func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("📨 收到消息: %s", string(msg.Payload))
		return msg.Ack() // 确认消息
	})
	if err != nil {
		log.Fatalf("订阅失败: %v", err)
	}
	log.Println("✓ 订阅成功: topic=hello, channel=quickstart")

	// 等待订阅准备好
	time.Sleep(2 * time.Second)

	// ========== 步骤 4: 发布消息 ==========
	publisher := bus.Publisher()
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		message := []byte("Hello, Messaging! #" + string(rune('0'+i)))
		err := publisher.Publish(ctx, "hello", message)
		if err != nil {
			log.Printf("发布失败: %v", err)
		} else {
			log.Printf("✓ 发布消息 #%d", i)
		}
		time.Sleep(time.Second)
	}

	// ========== 步骤 5: 等待退出 ==========
	log.Println("\n按 Ctrl+C 退出...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n正在关闭...")
}

// 运行前准备：
// 1. 启动 NSQ：
//    docker run -d --name nsqlookupd -p 4160:4160 -p 4161:4161 nsqio/nsq /nsqlookupd
//    docker run -d --name nsqd -p 4150:4150 -p 4151:4151 \
//      nsqio/nsq /nsqd --lookupd-tcp-address=host.docker.internal:4160
//
// 2. 运行示例：
//    go run main.go
//
// 预期输出：
// === Messaging 快速入门示例 ===
// ✓ 使用默认配置（NSQ）
// ✓ 事件总线创建成功
// ✓ 订阅成功: topic=hello, channel=quickstart
// ✓ 发布消息 #1
// 📨 收到消息: Hello, Messaging! #1
// ✓ 发布消息 #2
// 📨 收到消息: Hello, Messaging! #2
// ...
