package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v7"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// RedisPublisher Redis发布者
type RedisPublisher struct {
	client *redis.Client
	config *RedisConfig
}

// NewRedisPublisher 创建Redis发布者
func NewRedisPublisher(config *RedisConfig) *RedisPublisher {
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	return &RedisPublisher{
		client: rdb,
		config: config,
	}
}

// Connect 连接Redis
func (p *RedisPublisher) Connect(ctx context.Context) error {
	_, err := p.client.Ping().Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Infof("Redis publisher connected to %s", p.config.Addr)
	return nil
}

// Publish 发布消息
func (p *RedisPublisher) Publish(ctx context.Context, topic string, message interface{}) error {
	// 序列化消息
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 发布消息
	err = p.client.Publish(topic, data).Err()
	if err != nil {
		return fmt.Errorf("failed to publish message to topic %s: %w", topic, err)
	}

	log.Infof("Published message to topic %s: %s", topic, string(data))
	return nil
}

// Close 关闭连接
func (p *RedisPublisher) Close() error {
	return p.client.Close()
}

// RedisSubscriber Redis订阅者
type RedisSubscriber struct {
	client   *redis.Client
	config   *RedisConfig
	handlers map[string]MessageHandler
}

// NewRedisSubscriber 创建Redis订阅者
func NewRedisSubscriber(config *RedisConfig) *RedisSubscriber {
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	return &RedisSubscriber{
		client:   rdb,
		config:   config,
		handlers: make(map[string]MessageHandler),
	}
}

// Connect 连接Redis
func (s *RedisSubscriber) Connect(ctx context.Context) error {
	_, err := s.client.Ping().Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Infof("Redis subscriber connected to %s", s.config.Addr)
	return nil
}

// Subscribe 订阅主题
func (s *RedisSubscriber) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	s.handlers[topic] = handler

	// 创建订阅
	pubsub := s.client.Subscribe(topic)
	defer pubsub.Close()

	log.Infof("Subscribed to topic: %s", topic)

	// 接收消息
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Info("Subscription context cancelled")
			return ctx.Err()
		case msg := <-ch:
			if msg == nil {
				continue
			}

			log.Infof("Received message from topic %s: %s", msg.Channel, msg.Payload)

			// 处理消息 - 使用正确的函数签名
			if handler, exists := s.handlers[msg.Channel]; exists {
				if err := handler(msg.Channel, []byte(msg.Payload)); err != nil {
					log.Errorf("Failed to handle message from topic %s: %v", msg.Channel, err)
					// 这里可以添加重试逻辑或者死信队列
				}
			}
		}
	}
}

// SubscribeMultiple 订阅多个主题
func (s *RedisSubscriber) SubscribeMultiple(ctx context.Context, topics []string) error {
	if len(topics) == 0 {
		return fmt.Errorf("no topics to subscribe")
	}

	// 创建订阅
	pubsub := s.client.Subscribe(topics...)
	defer pubsub.Close()

	log.Infof("Subscribed to topics: %v", topics)

	// 接收消息
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Info("Subscription context cancelled")
			return ctx.Err()
		case msg := <-ch:
			if msg == nil {
				continue
			}

			log.Infof("Received message from topic %s: %s", msg.Channel, msg.Payload)

			// 处理消息 - 使用正确的函数签名
			if handler, exists := s.handlers[msg.Channel]; exists {
				if err := handler(msg.Channel, []byte(msg.Payload)); err != nil {
					log.Errorf("Failed to handle message from topic %s: %v", msg.Channel, err)
				}
			}
		}
	}
}

// RegisterHandler 注册消息处理器
func (s *RedisSubscriber) RegisterHandler(topic string, handler MessageHandler) {
	s.handlers[topic] = handler
	log.Infof("Registered handler for topic: %s", topic)
}

// Close 关闭连接
func (s *RedisSubscriber) Close() error {
	return s.client.Close()
}

// HealthCheck 健康检查
func (s *RedisSubscriber) HealthCheck(ctx context.Context) error {
	_, err := s.client.Ping().Result()
	return err
}
