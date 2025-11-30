package scale

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

// UnpublishedHandler 量表下架消息处理器
type UnpublishedHandler struct {
	*core.TemplateMessageHandler
}

// NewUnpublishedHandler 创建量表下架消息处理器
func NewUnpublishedHandler(logger *slog.Logger) *UnpublishedHandler {
	return &UnpublishedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventScaleUnpublished, logger),
	}
}

// Handle 处理量表下架消息（调用模板方法）
func (h *UnpublishedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *UnpublishedHandler) Parse(payload []byte) (interface{}, error) {
	var dto ScaleUnpublishedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		return nil, err
	}
	return dto, nil
}

// Process 实现 MessageProcessor 接口
func (h *UnpublishedHandler) Process(ctx context.Context, data interface{}) error {
	dto := data.(ScaleUnpublishedEventDTO)
	_ = dto

	// TODO: 清除缓存和规则

	return nil
}
