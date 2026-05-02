package pipeline

import "context"

// WaiterNotifyHandler 只负责长轮询 waiter 的本地通知，不承担事件投递。
type WaiterNotifyHandler struct {
	*BaseHandler
	notifier CompletionNotifier
}

func NewWaiterNotifyHandler(notifier CompletionNotifier) *WaiterNotifyHandler {
	return &WaiterNotifyHandler{
		BaseHandler: NewBaseHandler("WaiterNotifyHandler"),
		notifier:    notifier,
	}
}

// Handle 在评估成功后通知等待队列。
func (h *WaiterNotifyHandler) Handle(ctx context.Context, evalCtx *Context) error {
	if h.notifier != nil {
		h.notifier.NotifyCompletion(ctx, evalCtx)
	}

	return h.Next(ctx, evalCtx)
}
