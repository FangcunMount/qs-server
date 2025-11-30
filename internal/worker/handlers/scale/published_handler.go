package scale

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

func init() {
	core.RegisterMessageHandler(core.TopicQuestionnaireLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
		return NewPublishedHandler(deps.Logger)
	})
}

// PublishedHandler 量表发布消息处理器
type PublishedHandler struct {
	*core.TemplateMessageHandler
}

// NewPublishedHandler 创建量表发布消息处理器
func NewPublishedHandler(logger *slog.Logger) *PublishedHandler {
	return &PublishedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventScalePublished, logger),
	}
}

// Handle 处理量表发布消息（调用模板方法）
func (h *PublishedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *PublishedHandler) Parse(payload []byte) (interface{}, error) {
	var dto ScalePublishedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		return nil, err
	}
	return dto, nil
}

// Process 实现 MessageProcessor 接口
func (h *PublishedHandler) Process(ctx context.Context, data interface{}) error {
	dto := data.(ScalePublishedEventDTO)
	_ = dto

	// TODO: 预热缓存、预加载规则

	return nil
}
