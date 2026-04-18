package worker

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	cbdatabase "github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/messaging"
	cbnsq "github.com/FangcunMount/component-base/pkg/messaging/nsq"
	"github.com/FangcunMount/component-base/pkg/messaging/rabbitmq"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"github.com/FangcunMount/component-base/pkg/shutdown/shutdownmanagers/posixsignal"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	"github.com/nsqio/go-nsq"
	redis "github.com/redis/go-redis/v9"
)

// workerServer 定义了 Worker 服务器的基本结构
type workerServer struct {
	// 优雅关闭管理器
	gs *shutdown.GracefulShutdown
	// 配置
	config *config.Config
	// 日志器
	logger *slog.Logger
	// 数据库管理器
	dbManager *DatabaseManager
	// lock/lease 专用 Redis 客户端
	lockRedis redis.UniversalClient
	// family 状态注册表
	familyStatus *cacheobservability.FamilyStatusRegistry
	// Container 主容器
	container *container.Container
	// gRPC 客户端管理器
	grpcManager *grpcclient.Manager
	// 消息订阅者
	subscriber messaging.Subscriber
}

// preparedWorkerServer 定义了准备运行的 Worker 服务器
type preparedWorkerServer struct {
	*workerServer
}

// createWorkerServer 创建 Worker 服务器实例
func createWorkerServer(cfg *config.Config) (*workerServer, error) {
	// 创建一个 GracefulShutdown 实例
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())
	log.Info("🔔 Graceful shutdown manager registered (POSIX signals)")

	// 初始化日志
	logger := initLogger(cfg.Log)

	// 创建 Worker 服务器实例
	server := &workerServer{
		gs:           gs,
		config:       cfg,
		logger:       logger,
		familyStatus: cacheobservability.NewFamilyStatusRegistry("worker"),
	}

	log.Infof("✅ Worker server created (service: %s, concurrency: %d)",
		cfg.Worker.ServiceName, cfg.Worker.Concurrency)

	return server, nil
}

// PrepareRun 准备运行 Worker 服务器
func (s *workerServer) PrepareRun() preparedWorkerServer {
	var err error

	// 1. 初始化数据库管理器（Redis）
	s.dbManager = NewDatabaseManager(s.config)
	if err = s.dbManager.Initialize(); err != nil {
		log.Fatalf("Failed to initialize database manager: %v", err)
	}
	s.lockRedis = s.resolveLockRedisClient()

	// 2. 创建 gRPC 客户端管理器
	s.grpcManager, err = CreateGRPCClientManager(
		s.config.GRPC,
		30, // 默认超时 30 秒
	)
	if err != nil {
		log.Fatalf("Failed to create gRPC client manager: %v", err)
	}
	log.Infof("✅ gRPC client manager initialized (endpoint: %s)", s.config.GRPC.ApiserverAddr)

	// 3. 创建容器
	s.container = container.NewContainer(
		s.config.Options,
		s.logger,
		s.lockRedis,
	)

	// 4. 通过 GRPCClientRegistry 注入 gRPC 客户端到容器
	grpcRegistry := NewGRPCClientRegistry(s.grpcManager, s.container)
	if err = grpcRegistry.RegisterClients(); err != nil {
		log.Fatalf("Failed to register gRPC clients: %v", err)
	}

	// 5. 初始化容器中的所有组件
	if err = s.container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}

	// 6. 启动内建 plan scheduler（通过 gRPC 调用 apiserver 写侧命令）
	s.startPlanScheduler()

	// 7. 预创建 NSQ Topics（可选，避免 TOPIC_NOT_FOUND 日志）
	if s.config.Messaging.Provider == "nsq" {
		if err = s.createTopics(); err != nil {
			// Topic 创建失败不是致命错误，只记录警告
			log.Warnf("⚠️  Topic creation failed (non-fatal): %v", err)
		}
	}

	// 8. 创建消息订阅者
	maxInFlight := 1
	if s.config != nil && s.config.Worker != nil && s.config.Worker.Concurrency > 0 {
		maxInFlight = s.config.Worker.Concurrency
	}
	s.subscriber, err = createSubscriber(s.config.Messaging, s.logger, maxInFlight)
	if err != nil {
		log.Fatalf("Failed to create subscriber: %v", err)
	}
	log.Infof("✅ Message subscriber created (provider: %s)", s.config.Messaging.Provider)

	// 9. 订阅所有处理器
	if err = s.subscribeHandlers(); err != nil {
		log.Fatalf("Failed to subscribe handlers: %v", err)
	}

	log.Info("🏗️  Worker Server initialized successfully!")

	// 添加关闭回调
	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		if s.subscriber != nil {
			s.subscriber.Stop()
			_ = s.subscriber.Close()
		}
		if s.grpcManager != nil {
			_ = s.grpcManager.Close()
		}
		if s.dbManager != nil {
			_ = s.dbManager.Close()
		}

		// 清理容器资源
		if s.container != nil {
			s.container.Cleanup()
		}

		log.Info("🏗️  Worker Server shutdown complete")
		return nil
	}))

	return preparedWorkerServer{s}
}

func (s *workerServer) lockKeyBuilder() *rediskey.Builder {
	if s == nil || s.config == nil || s.config.Options == nil || s.config.Options.Cache == nil {
		return rediskey.NewBuilder()
	}
	suffix := ""
	if s.config.Options.Cache.Lock != nil {
		suffix = s.config.Options.Cache.Lock.NamespaceSuffix
	}
	return rediskey.NewBuilderWithNamespace(
		rediskey.ComposeNamespace(s.config.Options.Cache.Namespace, suffix),
	)
}

func (s *workerServer) resolveLockRedisClient() redis.UniversalClient {
	if s == nil || s.dbManager == nil {
		return nil
	}

	profile := ""
	if s.config != nil && s.config.Options != nil && s.config.Options.Cache != nil && s.config.Options.Cache.Lock != nil {
		profile = s.config.Options.Cache.Lock.RedisProfile
	}
	namespace := ""
	if s.config != nil && s.config.Options != nil && s.config.Options.Cache != nil && s.config.Options.Cache.Lock != nil {
		namespace = rediskey.ComposeNamespace(s.config.Options.Cache.Namespace, s.config.Options.Cache.Lock.NamespaceSuffix)
	}
	updateStatus := func(mode string, configured, available, degraded bool, err error) {
		if s.familyStatus == nil {
			return
		}
		s.familyStatus.Update(cacheobservability.FamilyStatus{
			Component:   "worker",
			Family:      "lock_lease",
			Profile:     profile,
			Namespace:   namespace,
			AllowWarmup: false,
			Configured:  configured,
			Available:   available,
			Degraded:    degraded,
			Mode:        mode,
			LastError:   errorString(err),
		})
	}

	if profile == "" {
		client, err := s.dbManager.GetRedisClient()
		if err != nil {
			log.Warnf("worker lock Redis not available: %v", err)
			updateStatus(cacheobservability.FamilyModeDegraded, true, false, true, err)
			return nil
		}
		log.Infof("worker lock Redis using default profile")
		updateStatus(cacheobservability.FamilyModeDefault, true, true, false, nil)
		return client
	}

	status := s.dbManager.GetRedisProfileStatus(profile)
	switch status.State {
	case cbdatabase.RedisProfileStateMissing:
		client, err := s.dbManager.GetRedisClient()
		if err != nil {
			log.Warnf("worker lock Redis default fallback unavailable (profile=%s): %v", profile, err)
			updateStatus(cacheobservability.FamilyModeDegraded, false, false, true, err)
			return nil
		}
		log.Infof("worker lock Redis profile missing, falling back to default (profile=%s)", profile)
		updateStatus(cacheobservability.FamilyModeFallbackDefault, false, true, false, nil)
		return client
	case cbdatabase.RedisProfileStateUnavailable:
		log.Warnf("worker lock Redis profile unavailable, running degraded without HA lock (profile=%s, error=%v)", profile, status.Err)
		updateStatus(cacheobservability.FamilyModeDegraded, true, false, true, status.Err)
		return nil
	default:
		client, err := s.dbManager.GetRedisClientByProfile(profile)
		if err != nil {
			log.Warnf("worker lock Redis profile unavailable, running degraded without HA lock (profile=%s, error=%v)", profile, err)
			updateStatus(cacheobservability.FamilyModeDegraded, true, false, true, err)
			return nil
		}
		log.Infof("worker lock Redis using named profile (profile=%s)", profile)
		updateStatus(cacheobservability.FamilyModeNamedProfile, true, true, false, nil)
		return client
	}
}

// subscribeHandlers 订阅所有 Topic 处理器
func (s *workerServer) subscribeHandlers() error {
	subscriptions := s.container.GetTopicSubscriptions()
	for _, sub := range subscriptions {
		topicName := sub.TopicName
		msgHandler := s.createDispatchHandler(topicName)
		if err := s.subscriber.Subscribe(topicName, s.config.Worker.ServiceName, msgHandler); err != nil {
			s.logger.Error("failed to subscribe",
				slog.String("topic", topicName),
				slog.String("error", err.Error()),
			)
			return err
		}
		s.logger.Info("subscribed to topic",
			slog.String("topic", topicName),
			slog.Int("event_count", len(sub.EventTypes)),
			slog.String("channel", s.config.Worker.ServiceName),
		)
	}
	return nil
}

// createDispatchHandler 创建分发处理函数
func (s *workerServer) createDispatchHandler(topicName string) messaging.Handler {
	return func(ctx context.Context, msg *messaging.Message) error {
		// 从消息元数据中提取事件类型
		eventType, ok := msg.Metadata["event_type"]
		if !ok {
			// 尝试从 payload 解析事件信封获取 eventType（兼容未传 metadata 的发布端）
			env, err := handlers.ParseEventEnvelope(msg.Payload)
			if err != nil {
				s.logger.Warn("message missing event_type and payload parse failed",
					slog.String("topic", topicName),
					slog.String("msg_id", msg.UUID),
					slog.String("error", err.Error()),
				)
				msg.Ack() // 无法处理，直接确认避免堆积
				return nil
			}
			eventType = env.EventType
			// 填充 metadata，后续处理链可复用
			msg.Metadata["event_type"] = eventType
		}

		s.logger.Debug("received message",
			slog.String("topic", topicName),
			slog.String("event_type", eventType),
			slog.String("msg_id", msg.UUID),
		)

		// 分发到对应的处理器
		if err := s.container.DispatchEvent(ctx, eventType, msg.Payload); err != nil {
			s.logger.Error("failed to dispatch event",
				slog.String("topic", topicName),
				slog.String("event_type", eventType),
				slog.String("msg_id", msg.UUID),
				slog.String("error", err.Error()),
			)
			msg.Nack()
			return err
		}

		msg.Ack()
		return nil
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// createTopics 在 NSQ 中预创建 Topics
// 在 subscriber 启动前预先创建所有 topics，避免 TOPIC_NOT_FOUND 日志
func (s *workerServer) createTopics() error {
	// 获取所有需要订阅的 topics
	subscriptions := s.container.GetTopicSubscriptions()
	topics := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		topics = append(topics, sub.TopicName)
	}

	if len(topics) == 0 {
		s.logger.Debug("No topics to create")
		return nil
	}

	// 创建 Topic 创建器
	creator := cbnsq.NewTopicCreator(s.config.Messaging.NSQAddr, s.logger)

	// 创建所有 topics
	return creator.EnsureTopics(topics)
}

// Run 运行 Worker 服务器
func (s preparedWorkerServer) Run() error {
	// 启动关闭管理器
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}
	log.Info("🚦 Shutdown manager started, worker coming online")

	log.Info("🚀 Worker started, waiting for events...")

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutdown signal received, stopping workers...")
	return nil
}

// initLogger 初始化日志
func initLogger(cfg *config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// createSubscriber 创建消息订阅者
func createSubscriber(cfg *config.MessagingConfig, logger *slog.Logger, maxInFlight int) (messaging.Subscriber, error) {
	switch cfg.Provider {
	case "nsq":
		nsqCfg := nsq.NewConfig()
		if maxInFlight > 0 {
			nsqCfg.MaxInFlight = maxInFlight
		}
		return cbnsq.NewSubscriber([]string{cfg.NSQLookupdAddr}, nsqCfg)
	case "rabbitmq":
		return rabbitmq.NewSubscriber(cfg.RabbitMQURL)
	default:
		logger.Warn("unknown messaging provider, using NSQ as default",
			slog.String("provider", cfg.Provider),
		)
		return cbnsq.NewSubscriber([]string{cfg.NSQLookupdAddr}, nil)
	}
}
