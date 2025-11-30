package handlers

import (
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"

	// 导入处理器包以触发 init() 自动注册
	_ "github.com/FangcunMount/qs-server/internal/worker/handlers/assessment"
	_ "github.com/FangcunMount/qs-server/internal/worker/handlers/questionnaire"
	_ "github.com/FangcunMount/qs-server/internal/worker/handlers/scale"
)

// ==================== 重新导出 core 包类型（供外部使用）====================
// 外部使用者应直接使用 core 包的常量（core.TopicXxx, core.EventXxx）
// 这里只导出必要的类型别名

type (
	MessageHandler      = core.MessageHandler
	TopicHandler        = core.TopicHandler
	HandlerDependencies = core.HandlerDependencies
)

// TopicRegistry Topic 处理器注册表
type TopicRegistry struct {
	handlers map[string]TopicHandler
	logger   *slog.Logger
}

// NewTopicRegistry 创建 Topic 处理器注册表
func NewTopicRegistry(logger *slog.Logger) *TopicRegistry {
	return &TopicRegistry{
		handlers: make(map[string]TopicHandler),
		logger:   logger,
	}
}

// Register 注册 Topic 处理器
func (r *TopicRegistry) Register(handler TopicHandler) {
	r.handlers[handler.Topic()] = handler

	// 统计 MessageHandler 数量
	messageHandlerCount := 0
	if genericHandler, ok := handler.(*GenericTopicHandler); ok {
		messageHandlerCount = genericHandler.HandlerCount()
	}

	r.logger.Info("topic handler registered",
		slog.String("topic", handler.Topic()),
		slog.String("handler", handler.Name()),
		slog.Int("message_handlers", messageHandlerCount),
	)
}

// Get 获取 Topic 处理器
func (r *TopicRegistry) Get(topic string) (TopicHandler, bool) {
	handler, ok := r.handlers[topic]
	return handler, ok
}

// Topics 获取所有已注册的主题
func (r *TopicRegistry) Topics() []string {
	topics := make([]string, 0, len(r.handlers))
	for topic := range r.handlers {
		topics = append(topics, topic)
	}
	return topics
}

// All 获取所有 Topic 处理器
func (r *TopicRegistry) All() []TopicHandler {
	handlers := make([]TopicHandler, 0, len(r.handlers))
	for _, handler := range r.handlers {
		handlers = append(handlers, handler)
	}
	return handlers
}

// TopicHandlerDeps Topic 处理器依赖
type TopicHandlerDeps struct {
	Logger            *slog.Logger
	AnswerSheetClient *grpcclient.AnswerSheetClient
	EvaluationClient  *grpcclient.EvaluationClient
}

// RegisterDefaultTopicHandlers 注册默认 Topic 处理器
// 使用自动注册机制：所有 MessageHandler 通过 init() 注册，
// CreateTopicHandlers() 自动为每个 Topic 创建 GenericTopicHandler
func RegisterDefaultTopicHandlers(registry *TopicRegistry, deps *TopicHandlerDeps) {
	// 构建处理器依赖
	handlerDeps := &HandlerDependencies{
		Logger: deps.Logger,
		Extra: map[string]interface{}{
			"answerSheetClient": deps.AnswerSheetClient,
			"evaluationClient":  deps.EvaluationClient,
		},
	}

	// 自动创建并注册所有 TopicHandler
	topicHandlers := CreateTopicHandlers(handlerDeps)
	for _, handler := range topicHandlers {
		registry.Register(handler)
	}
}

// ==================== 自动创建 Topic 处理器 ====================

// CreateTopicHandlers 根据配置自动创建所有 Topic 处理器
// 自动将注册的 MessageHandler 关联到对应的 Topic
func CreateTopicHandlers(deps *core.HandlerDependencies) []core.TopicHandler {
	handlers := make([]core.TopicHandler, 0, len(core.AllTopicConfigs))

	for _, config := range core.AllTopicConfigs {
		handler := NewGenericTopicHandler(config.Topic, config.HandlerName, deps.Logger)

		// 获取该 Topic 下注册的所有 MessageHandler 工厂
		factories := core.GetMessageHandlerFactories(config.Topic)
		for _, factory := range factories {
			msgHandler := factory(deps)
			handler.RegisterHandler(msgHandler)
		}

		handlers = append(handlers, handler)
	}

	return handlers
}
