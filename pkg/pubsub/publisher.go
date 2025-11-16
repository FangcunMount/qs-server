package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/pkg/log"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// watermillPublisher Watermill 发布者实现
type watermillPublisher struct {
	publisher message.Publisher
	config    *Config
	logger    watermill.LoggerAdapter
}

// newWatermillPublisher 创建 Watermill 发布者
func newWatermillPublisher(config *Config) (*watermillPublisher, error) {
	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	// 创建 Watermill logger
	logger := &watermillLogger{}

	// 创建发布者配置
	publisherConfig := redisstream.PublisherConfig{
		Client:     rdb,
		Marshaller: redisstream.DefaultMarshallerUnmarshaller{},
	}

	// 创建发布者
	publisher, err := redisstream.NewPublisher(publisherConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create publisher: %w", err)
	}

	return &watermillPublisher{
		publisher: publisher,
		config:    config,
		logger:    logger,
	}, nil
}

// Publish 发布消息
func (p *watermillPublisher) Publish(ctx context.Context, topic string, payload interface{}) error {
	// 序列化消息
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 创建消息
	msg := message.NewMessage(watermill.NewUUID(), data)

	// 添加元数据
	msg.Metadata.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixNano()))
	msg.Metadata.Set("source", "pubsub-publisher")

	// 发布消息
	if err := p.publisher.Publish(topic, msg); err != nil {
		return fmt.Errorf("failed to publish message to topic %s: %w", topic, err)
	}

	log.Infof("Published message to topic %s: %s", topic, string(data))
	return nil
}

// Close 关闭发布者
func (p *watermillPublisher) Close() error {
	return p.publisher.Close()
}

// noOpPublisher 空发布者实现
type noOpPublisher struct{}

func (p *noOpPublisher) Publish(topic string, messages ...*message.Message) error {
	return nil
}

func (p *noOpPublisher) Close() error {
	return nil
}
