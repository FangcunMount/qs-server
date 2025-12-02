// Package main 完整的生产级应用示例
// 综合展示：配置管理、中间件、可观测性、错误处理、优雅关闭
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
	log.Println("=== 完整应用演示 ===")
	log.Println("提示: 这是一个综合性的生产级示例")
	log.Println("包括: 配置管理、中间件链、可观测性、错误处理、优雅关闭")

	// 创建默认配置的 EventBus
	bus, err := messaging.NewEventBus(messaging.DefaultConfig())
	if err != nil {
		log.Fatalf("创建 EventBus 失败: %v", err)
	}
	defer bus.Close()

	// 创建路由器
	router := bus.Router()

	// 添加全局中间件
	logger := log.New(os.Stdout, "[App] ", log.LstdFlags)
	router.AddMiddleware(messaging.LoggerMiddleware(logger))

	// 创建处理器
	handler := func(ctx context.Context, msg *messaging.Message) error {
		log.Printf("  → 处理消息: %s", string(msg.Payload))
		time.Sleep(100 * time.Millisecond) // 模拟处理
		return msg.Ack()
	}

	// 注册处理器
	router.AddHandler("app.messages", "app-worker", handler)

	// 启动路由器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go router.Run(ctx)

	time.Sleep(time.Second)

	// 发送测试消息
	log.Println("发送测试消息...")
	for i := 1; i <= 5; i++ {
		msg := messaging.NewMessage("", []byte(fmt.Sprintf("测试消息 #%d", i)))
		msg.Metadata["priority"] = fmt.Sprintf("%d", i)
		bus.Publisher().PublishMessage(context.Background(), "app.messages", msg)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)

	// 优雅关闭
	log.Println("\n开始优雅关闭...")
	router.Stop()
	log.Println("应用已安全关闭")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}
