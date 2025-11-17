package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/FangcunMount/qs-server/pkg/messaging"
)

// subscriber RabbitMQ 订阅者实现
type subscriber struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	consumers map[string]*consumer
	stopCh    chan struct{}
	mu        sync.Mutex
}

type consumer struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// NewSubscriber 创建 RabbitMQ 订阅者
//
// url 格式：amqp://username:password@host:port/vhost
// 例如：amqp://guest:guest@localhost:5672/
func NewSubscriber(url string) (messaging.Subscriber, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("创建 channel 失败: %w", err)
	}

	// 设置 QoS
	err = ch.Qos(
		200,   // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("设置 QoS 失败: %w", err)
	}

	return &subscriber{
		conn:      conn,
		channel:   ch,
		consumers: make(map[string]*consumer),
		stopCh:    make(chan struct{}),
	}, nil
}

// Subscribe 实现 messaging.Subscriber 接口
//
// RabbitMQ 中的映射：
//   - topic → exchange (使用 fanout 类型)
//   - channel → queue (队列名称)
//
// 关键步骤：
//  1. 声明 exchange
//  2. 声明 queue（channel 参数对应队列名）
//  3. 绑定 queue 到 exchange
//  4. 开始消费消息
//  5. 将 RabbitMQ 消息转换为 messaging.Message
//  6. 调用 handler 处理
//  7. 根据处理结果 Ack/Nack
func (s *subscriber) Subscribe(topic, channel string, handler messaging.Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已订阅
	key := topic + ":" + channel
	if _, exists := s.consumers[key]; exists {
		return fmt.Errorf("已经订阅了 %s:%s", topic, channel)
	}

	// 1. 声明 exchange
	err := s.channel.ExchangeDeclare(
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

	// 2. 声明 queue
	q, err := s.channel.QueueDeclare(
		channel, // name
		true,    // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return fmt.Errorf("声明 queue %s 失败: %w", channel, err)
	}

	// 3. 绑定 queue 到 exchange
	err = s.channel.QueueBind(
		q.Name, // queue name
		"",     // routing key (fanout 类型忽略)
		topic,  // exchange
		false,  // no-wait
		nil,    // arguments
	)
	if err != nil {
		return fmt.Errorf("绑定 queue %s 到 exchange %s 失败: %w", channel, topic, err)
	}

	// 4. 开始消费
	msgs, err := s.channel.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack (使用手动确认)
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return fmt.Errorf("开始消费 queue %s 失败: %w", channel, err)
	}

	// 5. 创建 context 用于取消
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	// 记录 consumer
	s.consumers[key] = &consumer{
		cancel: cancel,
		done:   done,
	}

	// 6. 处理消息
	go func() {
		defer close(done)

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case d, ok := <-msgs:
				if !ok {
					return
				}

				// 7. 转换为领域消息
				domainMsg := &messaging.Message{
					UUID:      d.MessageId,
					Payload:   d.Body,
					Metadata:  make(map[string]string),
					Timestamp: d.Timestamp.UnixNano(),
					Topic:     topic,
					Channel:   channel,
				}

				// 提取 Headers 到 Metadata
				for k, v := range d.Headers {
					if str, ok := v.(string); ok {
						domainMsg.Metadata[k] = str
					}
				}

				// 如果没有 UUID，使用 DeliveryTag
				if domainMsg.UUID == "" {
					domainMsg.UUID = fmt.Sprintf("%d", d.DeliveryTag)
				}

				// 注入 Ack/Nack 函数
				domainMsg.SetAckFunc(func() error {
					return d.Ack(false)
				})
				domainMsg.SetNackFunc(func() error {
					return d.Nack(false, true)
				})

				// 8. 调用 handler
				if err := handler(ctx, domainMsg); err != nil {
					// 处理失败，Nack（重新入队）
					domainMsg.Nack()
				} else {
					// 处理成功，Ack
					domainMsg.Ack()
				}
			}
		}
	}()

	return nil
}

// SubscribeWithMiddleware 订阅消息（支持中间件）
func (s *subscriber) SubscribeWithMiddleware(topic, channel string, handler messaging.Handler, middlewares ...messaging.Middleware) error {
	// 应用中间件
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	// 调用标准 Subscribe
	return s.Subscribe(topic, channel, handler)
}

// Stop 停止订阅（不关闭连接）
func (s *subscriber) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭 stopCh（如果还未关闭）
	select {
	case <-s.stopCh:
		// 已关闭
	default:
		close(s.stopCh)
	}

	// 取消所有 consumer
	for _, c := range s.consumers {
		c.cancel()
	}

	// 等待所有 consumer 退出
	for _, c := range s.consumers {
		<-c.done
	}
}

// Close 关闭订阅者
func (s *subscriber) Close() error {
	s.Stop()

	if s.channel != nil {
		s.channel.Close()
	}
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
