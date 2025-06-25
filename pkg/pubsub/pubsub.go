package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	redis "github.com/go-redis/redis/v7"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// PubSub 发布订阅接口
type PubSub interface {
	// 发布消息
	Publish(ctx context.Context, channel string, message interface{}) error

	// 订阅消息（阻塞式）
	Subscribe(ctx context.Context, channel string, handler MessageHandler) error

	// 批量订阅多个频道
	SubscribeMultiple(ctx context.Context, channels []string, handler MessageHandler) error

	// 关闭连接
	Close() error
}

// MessageHandler 消息处理函数类型
type MessageHandler func(channel string, message []byte) error

// Message 标准消息结构
type Message struct {
	Type      string      `json:"type"`      // 消息类型
	Source    string      `json:"source"`    // 来源服务
	Data      interface{} `json:"data"`      // 消息数据
	Timestamp int64       `json:"timestamp"` // 时间戳
}

// ResponseSavedMessage 答卷已保存消息
type ResponseSavedMessage struct {
	ResponseID      string `json:"response_id"`
	QuestionnaireID string `json:"questionnaire_id"`
	UserID          string `json:"user_id"`
	SubmittedAt     int64  `json:"submitted_at"`
}

// redisPubSub Redis 发布订阅实现
type redisPubSub struct {
	client redis.UniversalClient
}

// NewRedisPubSub 创建 Redis 发布订阅实例
func NewRedisPubSub(client redis.UniversalClient) PubSub {
	return &redisPubSub{
		client: client,
	}
}

// Publish 发布消息
func (r *redisPubSub) Publish(ctx context.Context, channel string, message interface{}) error {
	// 将消息序列化为 JSON
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 发布到 Redis
	result := r.client.Publish(channel, data)
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to publish message to channel %s: %w", channel, err)
	}

	log.Infof("Published message to channel %s, subscribers: %d", channel, result.Val())
	return nil
}

// Subscribe 订阅单个频道
func (r *redisPubSub) Subscribe(ctx context.Context, channel string, handler MessageHandler) error {
	return r.SubscribeMultiple(ctx, []string{channel}, handler)
}

// SubscribeMultiple 订阅多个频道
func (r *redisPubSub) SubscribeMultiple(ctx context.Context, channels []string, handler MessageHandler) error {
	// 创建订阅
	pubsub := r.client.Subscribe(channels...)
	defer pubsub.Close()

	log.Infof("Subscribed to channels: %v", channels)

	// 获取消息通道
	msgChan := pubsub.Channel()

	// 处理消息
	for {
		select {
		case <-ctx.Done():
			log.Info("Subscription context cancelled")
			return ctx.Err()

		case msg := <-msgChan:
			if msg == nil {
				log.Warn("Received nil message, continuing...")
				continue
			}

			// 处理消息
			if err := handler(msg.Channel, []byte(msg.Payload)); err != nil {
				log.Errorf("Error handling message from channel %s: %v", msg.Channel, err)
				// 继续处理其他消息，不中断订阅
			}
		}
	}
}

// Close 关闭连接
func (r *redisPubSub) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
