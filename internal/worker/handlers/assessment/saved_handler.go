package assessment

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

// init 自动注册处理器
func init() {
	core.RegisterMessageHandler(core.TopicAssessmentLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
		return NewSavedHandler(deps.Logger)
	})
}

// SavedHandler 答卷保存消息处理器
type SavedHandler struct {
	*core.TemplateMessageHandler
}

// NewSavedHandler 创建答卷保存消息处理器
func NewSavedHandler(logger *slog.Logger) *SavedHandler {
	return &SavedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventAnswerSheetSaved, logger),
	}
}

// Handle 处理答卷保存消息（调用模板方法）
func (h *SavedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *SavedHandler) Parse(payload []byte) (interface{}, error) {
	var dto SavedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		return nil, err
	}
	return dto, nil
}

// Process 实现 MessageProcessor 接口
func (h *SavedHandler) Process(ctx context.Context, data interface{}) error {
	dto := data.(SavedEventDTO)
	_ = dto

	// TODO: 判断是否需要创建 Assessment

	return nil
}
