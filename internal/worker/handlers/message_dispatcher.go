package handlers

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

// MessageDispatcher 消息分发器（注册表模式）
// 维护事件类型到处理器的映射，根据事件类型路由消息
type MessageDispatcher struct {
	handlers map[string]core.MessageHandler
}

// NewMessageDispatcher 创建消息分发器
func NewMessageDispatcher() *MessageDispatcher {
	return &MessageDispatcher{
		handlers: make(map[string]core.MessageHandler),
	}
}

// Register 注册消息处理器
func (d *MessageDispatcher) Register(handler core.MessageHandler) {
	d.handlers[handler.EventType()] = handler
}

// Count 返回已注册的处理器数量
func (d *MessageDispatcher) Count() int {
	return len(d.handlers)
}

// Dispatch 分发消息到对应的处理器
func (d *MessageDispatcher) Dispatch(ctx context.Context, eventType string, payload []byte) error {
	handler, ok := d.handlers[eventType]
	if !ok {
		// 返回 nil 表示忽略未知事件，或者返回错误
		return nil
	}
	return handler.Handle(ctx, payload)
}
