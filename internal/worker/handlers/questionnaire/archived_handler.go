package questionnaire

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

func init() {
	core.RegisterMessageHandler(core.TopicQuestionnaireLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
		return NewArchivedHandler(deps.Logger)
	})
}

// ArchivedHandler 问卷归档消息处理器
type ArchivedHandler struct {
	*core.TemplateMessageHandler
}

// NewArchivedHandler 创建问卷归档消息处理器
func NewArchivedHandler(logger *slog.Logger) *ArchivedHandler {
	return &ArchivedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventQuestionnaireArchived, logger),
	}
}

// Handle 处理问卷归档消息（调用模板方法）
func (h *ArchivedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *ArchivedHandler) Parse(payload []byte) (interface{}, error) {
	var dto ArchivedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		return nil, err
	}
	return dto, nil
}

// Process 实现 MessageProcessor 接口
func (h *ArchivedHandler) Process(ctx context.Context, data interface{}) error {
	dto := data.(ArchivedEventDTO)
	_ = dto

	// TODO: 实现缓存清除逻辑

	return nil
}
