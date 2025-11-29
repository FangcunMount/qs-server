package worker

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	"github.com/FangcunMount/qs-server/pkg/messaging"
	"github.com/FangcunMount/qs-server/pkg/messaging/nsq"
	"github.com/FangcunMount/qs-server/pkg/messaging/rabbitmq"
)

// Run 启动 Worker 服务
func Run(cfg *config.Config) error {
	// 初始化日志
	logger := initLogger(cfg.Log)
	logger.Info("starting qs-worker",
		slog.String("service", cfg.Worker.ServiceName),
		slog.Int("concurrency", cfg.Worker.Concurrency),
	)

	// 初始化 gRPC 客户端管理器
	grpcClientManager := grpcclient.NewClientManager(&grpcclient.ClientConfig{
		ApiserverAddr: cfg.GRPC.ApiserverAddr,
	})
	if err := grpcClientManager.Connect(context.Background()); err != nil {
		logger.Error("failed to connect to apiserver", slog.String("error", err.Error()))
		return err
	}
	defer grpcClientManager.Close()
	logger.Info("connected to apiserver via gRPC", slog.String("addr", cfg.GRPC.ApiserverAddr))

	// 创建 gRPC 客户端
	answerSheetClient := grpcclient.NewAnswerSheetClient(grpcClientManager)
	evaluationClient := grpcclient.NewEvaluationClient(grpcClientManager)

	// 初始化消息订阅者
	subscriber, err := createSubscriber(cfg.Messaging, logger)
	if err != nil {
		logger.Error("failed to create subscriber", slog.String("error", err.Error()))
		return err
	}
	defer subscriber.Close()

	// 初始化处理器注册表
	registry := handlers.NewRegistry(logger)
	handlers.RegisterDefaultHandlers(registry, &handlers.HandlerDeps{
		Logger:            logger,
		AnswerSheetClient: answerSheetClient,
		EvaluationClient:  evaluationClient,
	})

	// 为每个处理器创建订阅
	for _, handler := range registry.All() {
		h := handler // 避免闭包问题
		msgHandler := createMessageHandler(h, logger)
		if err := subscriber.Subscribe(h.Topic(), cfg.Worker.ServiceName, msgHandler); err != nil {
			logger.Error("failed to subscribe",
				slog.String("topic", h.Topic()),
				slog.String("error", err.Error()),
			)
			return err
		}
		logger.Info("subscribed to topic",
			slog.String("topic", h.Topic()),
			slog.String("handler", h.Name()),
			slog.String("channel", cfg.Worker.ServiceName),
		)
	}

	logger.Info("qs-worker started, waiting for events...")

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutdown signal received, stopping workers...")
	subscriber.Stop()

	logger.Info("qs-worker stopped gracefully")
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
func createSubscriber(cfg *config.MessagingConfig, logger *slog.Logger) (messaging.Subscriber, error) {
	switch cfg.Provider {
	case "nsq":
		return nsq.NewSubscriber([]string{cfg.NSQLookupdAddr}, nil)
	case "rabbitmq":
		return rabbitmq.NewSubscriber(cfg.RabbitMQURL)
	default:
		logger.Warn("unknown messaging provider, using NSQ as default",
			slog.String("provider", cfg.Provider),
		)
		return nsq.NewSubscriber([]string{cfg.NSQLookupdAddr}, nil)
	}
}

// createMessageHandler 创建消息处理函数
func createMessageHandler(handler handlers.Handler, logger *slog.Logger) messaging.Handler {
	return func(ctx context.Context, msg *messaging.Message) error {
		logger.Debug("received message",
			slog.String("topic", handler.Topic()),
			slog.String("handler", handler.Name()),
			slog.String("msg_id", msg.UUID),
		)

		if err := handler.Handle(ctx, msg.Payload); err != nil {
			logger.Error("failed to handle message",
				slog.String("topic", handler.Topic()),
				slog.String("handler", handler.Name()),
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
