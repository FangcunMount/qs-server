package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/pkg/log"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// watermillSubscriber Watermill 订阅者实现
type watermillSubscriber struct {
	subscriber message.Subscriber
	config     *Config
	logger     watermill.LoggerAdapter
	router     *message.Router
}

// newWatermillSubscriber 创建 Watermill 订阅者
func newWatermillSubscriber(config *Config) (*watermillSubscriber, error) {
	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	// 创建 Watermill logger
	logger := &watermillLogger{}

	// 创建订阅者配置
	subscriberConfig := redisstream.SubscriberConfig{
		Client:        rdb,
		Unmarshaller:  redisstream.DefaultMarshallerUnmarshaller{},
		ConsumerGroup: config.ConsumerGroup,
		Consumer:      config.Consumer,
		ClaimInterval: config.ClaimInterval,
		BlockTime:     config.BlockTime,
	}

	// 创建订阅者
	subscriber, err := redisstream.NewSubscriber(subscriberConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscriber: %w", err)
	}

	// 创建路由器
	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create router: %w", err)
	}

	return &watermillSubscriber{
		subscriber: subscriber,
		config:     config,
		logger:     logger,
		router:     router,
	}, nil
}

// Subscribe 订阅主题
func (s *watermillSubscriber) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	// 创建处理函数
	handlerFunc := func(msg *message.Message) ([]*message.Message, error) {
		// 记录接收到的消息
		log.Infof("Received message from topic %s: %s", topic, string(msg.Payload))

		// 调用用户定义的处理器
		if err := handler(topic, msg.Payload); err != nil {
			log.Errorf("Failed to handle message from topic %s: %v", topic, err)
			return nil, err
		}

		return nil, nil
	}

	// 添加处理器到路由器
	s.router.AddHandler(
		fmt.Sprintf("handler_%s", topic),
		topic,
		s.subscriber,
		topic,
		&noOpPublisher{}, // 不需要发布消息
		handlerFunc,
	)

	log.Infof("Subscribed to topic: %s", topic)
	return nil
}

// SubscribeWithRetry 带重试机制的订阅
func (s *watermillSubscriber) SubscribeWithRetry(ctx context.Context, topic string, handler MessageHandler) error {
	// 创建带重试的处理函数
	handlerFunc := func(msg *message.Message) ([]*message.Message, error) {
		var lastErr error

		for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
			// 记录重试信息
			if attempt > 0 {
				log.Warnf("Retrying message processing (attempt %d/%d) for topic %s",
					attempt+1, s.config.MaxRetries, topic)
			}

			// 调用处理器
			if err := handler(topic, msg.Payload); err != nil {
				lastErr = err

				// 计算退避时间
				backoffDuration := s.calculateBackoff(attempt)
				log.Errorf("Failed to handle message (attempt %d): %v, retrying in %v",
					attempt+1, err, backoffDuration)

				// 等待退避时间
				time.Sleep(backoffDuration)
				continue
			}

			// 成功处理
			if attempt > 0 {
				log.Infof("Successfully processed message after %d retries", attempt)
			}
			return nil, nil
		}

		// 所有重试都失败了
		log.Errorf("Failed to process message after %d retries: %v", s.config.MaxRetries, lastErr)
		return nil, lastErr
	}

	// 添加处理器到路由器
	s.router.AddHandler(
		fmt.Sprintf("retry_handler_%s", topic),
		topic,
		s.subscriber,
		topic,
		&noOpPublisher{},
		handlerFunc,
	)

	log.Infof("Subscribed to topic with retry: %s", topic)
	return nil
}

// calculateBackoff 计算退避时间
func (s *watermillSubscriber) calculateBackoff(attempt int) time.Duration {
	// 指数退避算法
	duration := s.config.InitialInterval * time.Duration(1<<uint(attempt))
	if duration > s.config.MaxInterval {
		duration = s.config.MaxInterval
	}
	return duration
}

// Run 启动订阅者
func (s *watermillSubscriber) Run(ctx context.Context) error {
	log.Info("Starting message subscriber...")
	return s.router.Run(ctx)
}

// Close 关闭订阅者
func (s *watermillSubscriber) Close() error {
	if err := s.router.Close(); err != nil {
		log.Errorf("Failed to close router: %v", err)
	}
	return s.subscriber.Close()
}

// HealthCheck 健康检查
func (s *watermillSubscriber) HealthCheck(ctx context.Context) error {
	// 这里可以添加具体的健康检查逻辑
	return nil
}
