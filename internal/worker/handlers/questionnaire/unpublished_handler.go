package questionnaire

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

func init() {
	core.RegisterMessageHandler(core.TopicQuestionnaireLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
		return NewUnpublishedHandler(deps.Logger)
	})
}

// UnpublishedHandler 问卷下架消息处理器
type UnpublishedHandler struct {
	*core.TemplateMessageHandler
}

// NewUnpublishedHandler 创建问卷下架消息处理器
func NewUnpublishedHandler(logger *slog.Logger) *UnpublishedHandler {
	return &UnpublishedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventQuestionnaireUnpublished, logger),
	}
}

// Handle 处理问卷下架消息（调用模板方法）
func (h *UnpublishedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *UnpublishedHandler) Parse(payload []byte) (interface{}, error) {
	var dto UnpublishedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		return nil, err
	}
	return dto, nil
}

// Process 实现 MessageProcessor 接口
func (h *UnpublishedHandler) Process(ctx context.Context, data interface{}) error {
	dto := data.(UnpublishedEventDTO)
	_ = dto

	// TODO: 实现缓存清除逻辑

	return nil
}
