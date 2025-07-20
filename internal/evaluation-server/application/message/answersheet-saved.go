package message

import (
	"context"

	grpcclient "github.com/yshujie/questionnaire-scale/internal/evaluation-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
)

// AnswersheetSavedHandler 答卷已保存消息处理器
type AnswersheetSavedHandler interface {
	Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error
}

// AnswersheetSavedHandlerChain 答卷已保存消息处理器链
type AnswersheetSavedHandlerChain struct {
	handlers []AnswersheetSavedHandler
}

// AddHandler 向处理器链中增加处理器
func (chain *AnswersheetSavedHandlerChain) AddHandler(handler AnswersheetSavedHandler) {
	chain.handlers = append(chain.handlers, handler)
}

// Handle 处理答卷已保存消息
func (chain *AnswersheetSavedHandlerChain) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	for _, handler := range chain.handlers {
		if err := handler.Handle(ctx, data); err != nil {
			return err
		}
	}
	return nil
}

// HandlerGenerateInterpretReportContent 生成解读报告内容处理器
type HandlerGenerateInterpretReportContent struct {
	medicalScaleClient    *grpcclient.MedicalScaleClient
	interpretReportClient *grpcclient.InterpretReportClient
}

// Handle 生成解读报告内容，并保存解读报告
func (h *HandlerGenerateInterpretReportContent) Handle(ctx context.Context, data pubsub.AnswersheetSavedData) error {
	return nil
}
