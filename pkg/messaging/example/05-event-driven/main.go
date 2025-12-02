// Package main 演示事件驱动架构（Event-Driven Architecture）
// 一个事件，多个服务订阅（广播模式）
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	_ "github.com/FangcunMount/qs-server/pkg/messaging/nsq"
)

// UserCreatedEvent 用户创建事件
type UserCreatedEvent struct {
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	log.Println("=== 事件驱动架构演示 ===")
	log.Println("场景：用户注册后，通知多个服务（邮件、统计、审计）")

	bus, _ := messaging.NewEventBus(messaging.DefaultConfig())
	defer bus.Close()

	// ========== 关键点：每个服务使用不同的 channel ==========
	// 这样每条消息都会被所有服务接收（广播）

	// 服务 1: 邮件服务
	log.Println("启动服务：邮件服务（email-service）")
	bus.Subscriber().Subscribe("user.created", "email-service", emailService)

	// 服务 2: 统计服务
	log.Println("启动服务：统计服务（stat-service）")
	bus.Subscriber().Subscribe("user.created", "stat-service", statService)

	// 服务 3: 审计服务
	log.Println("启动服务：审计服务（audit-service）")
	bus.Subscriber().Subscribe("user.created", "audit-service", auditService)

	log.Println("\n所有服务已就绪，开始发布事件...")
	time.Sleep(2 * time.Second)

	// ========== 发布事件 ==========
	publisher := bus.Publisher()
	users := []UserCreatedEvent{
		{UserID: 1001, Username: "alice", Email: "alice@example.com", CreatedAt: time.Now()},
		{UserID: 1002, Username: "bob", Email: "bob@example.com", CreatedAt: time.Now()},
		{UserID: 1003, Username: "charlie", Email: "charlie@example.com", CreatedAt: time.Now()},
	}

	for i, user := range users {
		// 序列化事件
		payload, _ := json.Marshal(user)

		// 创建消息（带 Metadata）
		msg := messaging.NewMessage("", payload)
		msg.Metadata["event_type"] = "user.created"
		msg.Metadata["version"] = "v1"
		msg.Metadata["source"] = "user-service"

		// 发布事件
		publisher.PublishMessage(context.Background(), "user.created", msg)

		log.Printf("📤 [发布事件 #%d] 用户创建: user_id=%d, username=%s\n",
			i+1, user.UserID, user.Username)

		time.Sleep(2 * time.Second)
	}

	log.Println("\n所有事件发布完成，按 Ctrl+C 退出...")

	// 等待退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}

// emailService 邮件服务：发送欢迎邮件
func emailService(ctx context.Context, msg *messaging.Message) error {
	var event UserCreatedEvent
	json.Unmarshal(msg.Payload, &event)

	// 模拟发送邮件
	time.Sleep(100 * time.Millisecond)

	log.Printf("  📧 [邮件服务] 发送欢迎邮件: to=%s, user_id=%d",
		event.Email, event.UserID)

	return msg.Ack()
}

// statService 统计服务：更新用户统计
func statService(ctx context.Context, msg *messaging.Message) error {
	var event UserCreatedEvent
	json.Unmarshal(msg.Payload, &event)

	// 模拟更新统计
	time.Sleep(50 * time.Millisecond)

	log.Printf("  📊 [统计服务] 更新用户统计: user_id=%d, total_users++",
		event.UserID)

	return msg.Ack()
}

// auditService 审计服务：记录审计日志
func auditService(ctx context.Context, msg *messaging.Message) error {
	var event UserCreatedEvent
	json.Unmarshal(msg.Payload, &event)

	// 模拟记录审计日志
	time.Sleep(30 * time.Millisecond)

	log.Printf("  📝 [审计服务] 记录审计日志: user_id=%d, action=created, time=%s",
		event.UserID, event.CreatedAt.Format("15:04:05"))

	return msg.Ack()
}

// 预期输出：
//
// === 事件驱动架构演示 ===
// 场景：用户注册后，通知多个服务（邮件、统计、审计）
//
// 启动服务：邮件服务（email-service）
// 启动服务：统计服务（stat-service）
// 启动服务：审计服务（audit-service）
//
// 所有服务已就绪，开始发布事件...
//
// 📤 [发布事件 #1] 用户创建: user_id=1001, username=alice
//   📧 [邮件服务] 发送欢迎邮件: to=alice@example.com, user_id=1001
//   📊 [统计服务] 更新用户统计: user_id=1001, total_users++
//   📝 [审计服务] 记录审计日志: user_id=1001, action=created, time=14:23:45
//
// 📤 [发布事件 #2] 用户创建: user_id=1002, username=bob
//   📧 [邮件服务] 发送欢迎邮件: to=bob@example.com, user_id=1002
//   📊 [统计服务] 更新用户统计: user_id=1002, total_users++
//   📝 [审计服务] 记录审计日志: user_id=1002, action=created, time=14:23:47
//
// ...
//
// 核心知识点：
//
// 1. 事件驱动架构特点
//    - 发布者不知道有哪些订阅者
//    - 订阅者相互独立，互不影响
//    - 一个事件可以触发多个操作
//    - 松耦合，易于扩展
//
// 2. Channel 的作用
//    - 每个服务使用不同的 channel
//    - email-service、stat-service、audit-service
//    - 保证每条消息都被所有服务接收
//
// 3. 适用场景
//    ✅ 用户注册（发送邮件、更新统计、记录日志）
//    ✅ 订单创建（扣减库存、发送通知、生成发票）
//    ✅ 文件上传（生成缩略图、病毒扫描、CDN 同步）
//    ✅ 支付完成（更新订单、发送通知、积分增加）
//
// 4. 与任务队列的区别
//    事件驱动：多个服务，每个都收到消息（广播）
//    任务队列：多个 Worker，只有一个收到消息（负载均衡）
//
// 最佳实践：
// ✅ 事件名称使用过去式（user.created, order.paid）
// ✅ 事件包含完整的业务数据
// ✅ 使用 Metadata 传递版本和类型
// ✅ 每个服务独立处理，互不依赖
// ✅ 失败时记录日志，不影响其他服务
