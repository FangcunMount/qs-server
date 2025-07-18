package pubsub

import (
	"context"
	"fmt"
	"log"
	"time"
)

// BusinessExampleMessage 业务示例消息
type BusinessExampleMessage struct {
	*BaseMessage
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
}

// NewBusinessExampleMessage 创建业务示例消息
func NewBusinessExampleMessage(orderID, customerID string, amount float64) *BusinessExampleMessage {
	data := map[string]interface{}{
		"order_id":    orderID,
		"customer_id": customerID,
		"amount":      amount,
	}

	return &BusinessExampleMessage{
		BaseMessage: NewBaseMessage("order.created", "order-service", data),
		OrderID:     orderID,
		CustomerID:  customerID,
		Amount:      amount,
	}
}

// RunBusinessExample 运行业务层消息示例
func RunBusinessExample() {
	ctx := context.Background()

	// 创建配置
	config := DefaultConfig()
	config.Addr = "localhost:6379"
	config.ConsumerGroup = "business-group"
	config.Consumer = "business-consumer"

	// 创建发布订阅实例
	ps, err := NewPubSub(config)
	if err != nil {
		log.Fatalf("Failed to create pubsub: %v", err)
	}
	defer ps.Close()

	// 定义业务消息处理器
	businessHandler := func(topic string, data []byte) error {
		// 解析基础消息
		baseMsg, err := UnmarshalMessage(data)
		if err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}

		fmt.Printf("Received business message:\n")
		fmt.Printf("  Type: %s\n", baseMsg.GetType())
		fmt.Printf("  Source: %s\n", baseMsg.GetSource())
		fmt.Printf("  Timestamp: %s\n", baseMsg.GetTimestamp().Format(time.RFC3339))
		fmt.Printf("  Data: %+v\n", baseMsg.GetData())

		// 根据消息类型进行不同的处理
		switch baseMsg.GetType() {
		case "order.created":
			fmt.Println("  Processing order creation...")
			// 这里可以添加订单创建的业务逻辑
		case "order.updated":
			fmt.Println("  Processing order update...")
			// 这里可以添加订单更新的业务逻辑
		default:
			fmt.Printf("  Unknown message type: %s\n", baseMsg.GetType())
		}

		return nil
	}

	// 订阅主题
	topic := "business-events"
	if err := ps.Subscriber().Subscribe(ctx, topic, businessHandler); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 启动订阅者
	go func() {
		if err := ps.Subscriber().Run(ctx); err != nil {
			log.Printf("Subscriber error: %v", err)
		}
	}()

	// 等待订阅者启动
	time.Sleep(time.Second * 2)

	// 发布业务消息
	for i := 0; i < 3; i++ {
		// 创建业务消息
		businessMsg := NewBusinessExampleMessage(
			fmt.Sprintf("order-%d", i+1),
			fmt.Sprintf("customer-%d", i+1),
			99.99+float64(i)*10,
		)

		// 发布消息
		if err := ps.Publisher().Publish(ctx, topic, businessMsg); err != nil {
			log.Printf("Failed to publish message: %v", err)
		}

		fmt.Printf("Published business message: %s\n", businessMsg.OrderID)
		time.Sleep(time.Second)
	}

	// 等待消息处理完成
	time.Sleep(time.Second * 5)
}

// 演示如何使用 Message 接口
func DemoMessageInterface() {
	// 创建不同类型的消息
	messages := []Message{
		NewBaseMessage("user.created", "user-service", map[string]interface{}{
			"user_id": "user123",
			"email":   "user@example.com",
		}),
		NewBusinessExampleMessage("order123", "customer456", 199.99),
	}

	// 统一处理不同类型的消息
	for _, msg := range messages {
		fmt.Printf("Message Type: %s\n", msg.GetType())
		fmt.Printf("Message Source: %s\n", msg.GetSource())
		fmt.Printf("Message Timestamp: %s\n", msg.GetTimestamp().Format(time.RFC3339))

		// 序列化消息
		data, err := msg.Marshal()
		if err != nil {
			fmt.Printf("Failed to marshal message: %v\n", err)
			continue
		}

		fmt.Printf("Serialized data: %s\n", string(data))
		fmt.Println("---")
	}
}
