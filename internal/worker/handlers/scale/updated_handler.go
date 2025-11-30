package scale

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

func init() {
	core.RegisterMessageHandler(core.TopicQuestionnaireLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
		return NewUpdatedHandler(deps.Logger)
	})
}

// UpdatedHandler 量表更新消息处理器
type UpdatedHandler struct {
	*core.TemplateMessageHandler
}

// NewUpdatedHandler 创建量表更新消息处理器
func NewUpdatedHandler(logger *slog.Logger) *UpdatedHandler {
	return &UpdatedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventScaleUpdated, logger),
	}
}

// Handle 处理量表更新消息（调用模板方法）
func (h *UpdatedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *UpdatedHandler) Parse(payload []byte) (interface{}, error) {
	var dto ScaleUpdatedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		return nil, err
	}
	return dto, nil
}

// Process 实现 MessageProcessor 接口
func (h *UpdatedHandler) Process(ctx context.Context, data interface{}) error {
	dto := data.(ScaleUpdatedEventDTO)
	_ = dto

	// TODO: 重新加载规则

	return nil
}
