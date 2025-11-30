package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

// BaseTopicHandler Topic 处理器基础实现
type BaseTopicHandler struct {
	topic string
	name  string
}

// NewBaseTopicHandler 创建 Topic 处理器基础实例
func NewBaseTopicHandler(topic, name string) *BaseTopicHandler {
	return &BaseTopicHandler{
		topic: topic,
		name:  name,
	}
}

// Topic 返回处理器订阅的主题
func (h *BaseTopicHandler) Topic() string {
	return h.topic
}

// Name 返回处理器名称
func (h *BaseTopicHandler) Name() string {
	return h.name
}

// ==================== 通用 Topic 处理器 ====================

// GenericTopicHandler 通用 Topic 处理器
// 自动根据事件类型路由到对应的 MessageHandler
type GenericTopicHandler struct {
	*BaseTopicHandler
	logger     *slog.Logger
	dispatcher *MessageDispatcher
}

// NewGenericTopicHandler 创建通用 Topic 处理器
func NewGenericTopicHandler(topic, name string, logger *slog.Logger) *GenericTopicHandler {
	return &GenericTopicHandler{
		BaseTopicHandler: NewBaseTopicHandler(topic, name),
		logger:           logger,
		dispatcher:       NewMessageDispatcher(),
	}
}

// RegisterHandler 注册消息处理器
func (h *GenericTopicHandler) RegisterHandler(handler core.MessageHandler) {
	h.dispatcher.Register(handler)
}

// RegisterHandlers 批量注册消息处理器
func (h *GenericTopicHandler) RegisterHandlers(handlers ...core.MessageHandler) {
	for _, handler := range handlers {
		h.dispatcher.Register(handler)
	}
}

// HandlerCount 返回已注册的消息处理器数量
func (h *GenericTopicHandler) HandlerCount() int {
	return h.dispatcher.Count()
}

// Handle 分发处理消息
func (h *GenericTopicHandler) Handle(ctx context.Context, payload []byte) error {
	// 提取事件类型
	eventType, err := ExtractEventType(payload)
	if err != nil {
		h.logger.Error("failed to extract event type",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Debug("received event",
		slog.String("topic", h.Topic()),
		slog.String("handler", h.Name()),
		slog.String("event_type", eventType),
	)

	// 使用分发器路由事件
	return h.dispatcher.Dispatch(ctx, eventType, payload)
}

// EventTypeExtractor 用于提取事件类型
type EventTypeExtractor struct {
	EventType string `json:"event_type"`
}

// ExtractEventType 从 JSON payload 中提取事件类型
func ExtractEventType(payload []byte) (string, error) {
	var extractor EventTypeExtractor
	if err := json.Unmarshal(payload, &extractor); err != nil {
		return "", fmt.Errorf("failed to extract event type: %w", err)
	}
	return extractor.EventType, nil
}
