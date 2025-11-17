package nsq

import (
	"context"
	"fmt"
	"sync"

	"github.com/FangcunMount/qs-server/pkg/messaging"
	"github.com/nsqio/go-nsq"
)

// subscriber NSQ 订阅者实现
type subscriber struct {
	consumers []*nsq.Consumer
	config    *nsq.Config
	lookupd   []string
	mu        sync.RWMutex
	stopped   bool
}

// NewSubscriber 创建 NSQ 订阅者
// lookupdAddrs: NSQLookupd 地址列表
// cfg: NSQ 配置
func NewSubscriber(lookupdAddrs []string, cfg *nsq.Config) (messaging.Subscriber, error) {
	if cfg == nil {
		cfg = nsq.NewConfig()
	}

	if len(lookupdAddrs) == 0 {
		return nil, fmt.Errorf("lookupd addresses cannot be empty")
	}

	return &subscriber{
		consumers: make([]*nsq.Consumer, 0),
		config:    cfg,
		lookupd:   lookupdAddrs,
		stopped:   false,
	}, nil
}

// Subscribe 订阅主题
func (s *subscriber) Subscribe(topic, channel string, handler messaging.Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return fmt.Errorf("subscriber is stopped")
	}

	// 创建 consumer
	consumer, err := nsq.NewConsumer(topic, channel, s.config)
	if err != nil {
		return fmt.Errorf("failed to create NSQ consumer: %w", err)
	}

	// 添加消息处理器
	consumer.AddHandler(nsq.HandlerFunc(func(message *nsq.Message) error {
		// 将 NSQ 的 Message 转换为领域层的 Message
		domainMsg := &messaging.Message{
			UUID:      string(message.ID[:]),
			Payload:   message.Body,
			Metadata:  make(map[string]string),
			Attempts:  message.Attempts,
			Timestamp: message.Timestamp,
			Topic:     topic,
			Channel:   channel,
		}

		// 注入 Ack/Nack 函数
		domainMsg.SetAckFunc(func() error {
			message.Finish()
			return nil
		})
		domainMsg.SetNackFunc(func() error {
			message.Requeue(-1)
			return nil
		})

		// 创建 context（可以注入 trace、timeout 等）
		ctx := context.Background()

		// 调用业务层的 handler
		if err := handler(ctx, domainMsg); err != nil {
			// 如果处理失败，自动 Nack（重新入队）
			domainMsg.Nack()
			return err
		}

		// 处理成功，自动 Ack
		domainMsg.Ack()
		return nil
	}))

	// 连接到 NSQLookupd
	if err := consumer.ConnectToNSQLookupds(s.lookupd); err != nil {
		consumer.Stop()
		return fmt.Errorf("failed to connect to lookupd: %w", err)
	}

	// 保存 consumer 引用
	s.consumers = append(s.consumers, consumer)

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

// Stop 停止所有订阅
func (s *subscriber) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}

	s.stopped = true

	for _, consumer := range s.consumers {
		consumer.Stop()
	}
}

// Close 关闭订阅者
func (s *subscriber) Close() error {
	s.Stop()

	// 等待所有 consumer 停止
	for _, consumer := range s.consumers {
		<-consumer.StopChan
	}

	return nil
}

// Stats 获取订阅者统计信息
func (s *subscriber) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	for i, consumer := range s.consumers {
		consumerStats := consumer.Stats()
		stats[fmt.Sprintf("consumer_%d", i)] = map[string]interface{}{
			"messages": consumerStats.MessagesReceived,
			"finished": consumerStats.MessagesFinished,
			"requeued": consumerStats.MessagesRequeued,
		}
	}
	return stats
}
