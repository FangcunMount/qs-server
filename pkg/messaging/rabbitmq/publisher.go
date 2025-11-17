package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/FangcunMount/qs-server/pkg/messaging"
)

// publisher RabbitMQ 发布者实现
type publisher struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	exchanges map[string]bool // 已声明的 exchange
	mu        sync.RWMutex
}

// NewPublisher 创建 RabbitMQ 发布者
//
// url 格式：amqp://username:password@host:port/vhost
// 例如：amqp://guest:guest@localhost:5672/
func NewPublisher(url string) (messaging.Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("创建 channel 失败: %w", err)
	}

	return &publisher{
		conn:      conn,
		channel:   ch,
		exchanges: make(map[string]bool),
	}, nil
}

// Publish 实现 messaging.Publisher 接口
//
// 关键点：
//  1. 声明 exchange（topic 对应 RabbitMQ 的 exchange）
//  2. 发布消息到 exchange（使用 fanout 类型实现广播）
//  3. 处理错误和重试
func (p *publisher) Publish(ctx context.Context, topic string, body []byte) error {
	// 确保 exchange 已声明
	if err := p.ensureExchange(topic); err != nil {
		return err
	}

	// 发布消息
	return p.channel.PublishWithContext(
		ctx,
		topic, // exchange
		"",    // routing key (fanout 类型忽略)
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/octet-stream",
			Body:         body,
			DeliveryMode: amqp.Persistent, // 持久化消息
		},
	)
}

// PublishMessage 发布消息对象（支持 Metadata）
func (p *publisher) PublishMessage(ctx context.Context, topic string, msg *messaging.Message) error {
	// 确保 exchange 已声明
	if err := p.ensureExchange(topic); err != nil {
		return err
	}

	// 将 Metadata 转换为 Headers
	headers := make(amqp.Table)
	for k, v := range msg.Metadata {
		headers[k] = v
	}

	// 发布消息（支持 Headers）
	return p.channel.PublishWithContext(
		ctx,
		topic, // exchange
		"",    // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			MessageId:    msg.UUID,
			ContentType:  "application/octet-stream",
			Body:         msg.Payload,
			Headers:      headers,
			DeliveryMode: amqp.Persistent,
		},
	)
}

// ensureExchange 确保 exchange 已声明
func (p *publisher) ensureExchange(topic string) error {
	p.mu.RLock()
	if p.exchanges[topic] {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// 双重检查
	if p.exchanges[topic] {
		return nil
	}

	// 声明 fanout 类型的 exchange（实现广播）
	err := p.channel.ExchangeDeclare(
		topic,    // name
		"fanout", // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return fmt.Errorf("声明 exchange %s 失败: %w", topic, err)
	}

	p.exchanges[topic] = true
	return nil
}

// Close 关闭发布者
func (p *publisher) Close() error {
	if p.channel != nil {
		p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
