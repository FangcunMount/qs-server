package handlers

import (
	"context"
)

// Handler 事件处理器接口
type Handler interface {
	// Handle 处理事件
	Handle(ctx context.Context, payload []byte) error
	// Topic 返回处理器订阅的主题
	Topic() string
	// Name 返回处理器名称
	Name() string
}

// BaseHandler 基础处理器实现
type BaseHandler struct {
	topic string
	name  string
}

// NewBaseHandler 创建基础处理器
func NewBaseHandler(topic, name string) *BaseHandler {
	return &BaseHandler{
		topic: topic,
		name:  name,
	}
}

// Topic 返回处理器订阅的主题
func (h *BaseHandler) Topic() string {
	return h.topic
}

// Name 返回处理器名称
func (h *BaseHandler) Name() string {
	return h.name
}
