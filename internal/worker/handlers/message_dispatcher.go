package handlers

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
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
	l := logger.L(context.Background())
	eventType := handler.EventType()
	d.handlers[eventType] = handler

	l.Debugw("注册消息处理器",
		"action", "register_handler",
		"event_type", eventType,
	)
}

// Count 返回已注册的处理器数量
func (d *MessageDispatcher) Count() int {
	return len(d.handlers)
}

// Dispatch 分发消息到对应的处理器
func (d *MessageDispatcher) Dispatch(ctx context.Context, eventType string, payload []byte) error {
	l := logger.L(ctx)

	l.Debugw("开始分发消息",
		"action", "dispatch_message",
		"event_type", eventType,
		"payload_size", len(payload),
	)

	handler, ok := d.handlers[eventType]
	if !ok {
		l.Warnw("未找到对应的消息处理器，忽略此消息",
			"action", "dispatch_message",
			"event_type", eventType,
			"result", "no_handler",
		)
		// 返回 nil 表示忽略未知事件，或者返回错误
		return nil
	}

	l.Debugw("找到处理器，开始处理消息",
		"event_type", eventType,
	)

	err := handler.Handle(ctx, payload)
	if err != nil {
		l.Errorw("消息处理失败",
			"action", "dispatch_message",
			"event_type", eventType,
			"result", "failed",
			"error", err.Error(),
		)
		return err
	}

	l.Debugw("消息处理成功",
		"action", "dispatch_message",
		"event_type", eventType,
		"result", "success",
	)

	return nil
}
