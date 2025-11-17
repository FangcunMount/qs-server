package rabbitmq

import (
	"fmt"

	"github.com/FangcunMount/qs-server/pkg/messaging"
)

// 在包初始化时注册 RabbitMQ 提供者
func init() {
	messaging.RegisterProvider(messaging.ProviderRabbitMQ, NewEventBusFromConfig)
}

// eventBus RabbitMQ 事件总线实现
type eventBus struct {
	publisher  messaging.Publisher
	subscriber messaging.Subscriber
	router     *messaging.Router
	conn       interface{} // 保存连接用于健康检查
}

// NewEventBus 创建 RabbitMQ 事件总线
//
// url 格式：amqp://username:password@host:port/vhost
// 例如：amqp://guest:guest@localhost:5672/
//
// 使用示例：
//
//	bus, err := rabbitmq.NewEventBus("amqp://guest:guest@localhost:5672/")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer bus.Close()
//
//	// 使用发布者
//	publisher := bus.Publisher()
//	publisher.Publish(ctx, "user.created", data)
//
//	// 使用订阅者
//	subscriber := bus.Subscriber()
//	subscriber.Subscribe("user.created", "email-service", handler)
func NewEventBus(url string) (messaging.EventBus, error) {
	// 创建发布者
	pub, err := NewPublisher(url)
	if err != nil {
		return nil, fmt.Errorf("创建发布者失败: %w", err)
	}

	// 创建订阅者
	sub, err := NewSubscriber(url)
	if err != nil {
		pub.Close()
		return nil, fmt.Errorf("创建订阅者失败: %w", err)
	}

	bus := &eventBus{
		publisher:  pub,
		subscriber: sub,
	}
	bus.router = messaging.NewRouter(sub)

	return bus, nil
}

// NewEventBusFromConfig 从配置创建事件总线
func NewEventBusFromConfig(config *messaging.Config) (messaging.EventBus, error) {
	if config == nil {
		config = messaging.DefaultConfig()
	}

	// 构建 RabbitMQ URL
	url := config.RabbitMQ.URL
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	return NewEventBus(url)
}

// Publisher 返回发布者
func (b *eventBus) Publisher() messaging.Publisher {
	return b.publisher
}

// Subscriber 返回订阅者
func (b *eventBus) Subscriber() messaging.Subscriber {
	return b.subscriber
}

// Router 返回路由器
func (b *eventBus) Router() *messaging.Router {
	return b.router
}

// Health 健康检查
func (b *eventBus) Health() error {
	// 尝试创建一个临时 channel 来测试连接
	if p, ok := b.publisher.(*publisher); ok {
		if p.conn == nil || p.conn.IsClosed() {
			return fmt.Errorf("RabbitMQ connection is closed")
		}
	}
	return nil
}

// Close 关闭事件总线
func (b *eventBus) Close() error {
	// 先关闭订阅者，再关闭发布者
	if b.subscriber != nil {
		b.subscriber.Close()
	}
	if b.publisher != nil {
		return b.publisher.Close()
	}
	return nil
}
