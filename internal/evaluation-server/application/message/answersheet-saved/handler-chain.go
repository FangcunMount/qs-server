package answersheet_saved

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
)

// Handler 答卷已保存消息处理器
type Handler interface {
	Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error
}

// HandlerChain 答卷已保存消息处理器链
type HandlerChain struct {
	handlers []Handler
}

// AddHandler 向处理器链中增加处理器
func (chain *HandlerChain) AddHandler(handler Handler) {
	chain.handlers = append(chain.handlers, handler)
}

// Handle 处理答卷已保存消息
func (chain *HandlerChain) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	for _, handler := range chain.handlers {
		if err := handler.Handle(ctx, data); err != nil {
			return err
		}
	}
	return nil
}
