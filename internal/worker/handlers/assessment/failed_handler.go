package assessment

import (
	"context"
	"encoding/json"
	"log/slog"

	assessmentDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
)

// init 自动注册处理器
func init() {
	core.RegisterMessageHandler(core.TopicAssessmentLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
		return NewFailedHandler(deps.Logger)
	})
}

// FailedHandler 测评失败消息处理器
type FailedHandler struct {
	*core.TemplateMessageHandler
}

// NewFailedHandler 创建测评失败消息处理器
func NewFailedHandler(logger *slog.Logger) *FailedHandler {
	return &FailedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventAssessmentFailed, logger),
	}
}

// Handle 处理测评失败消息（调用模板方法）
func (h *FailedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *FailedHandler) Parse(payload []byte) (interface{}, error) {
	var event assessmentDomain.AssessmentFailedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	return event, nil
}

// Process 实现 MessageProcessor 接口
func (h *FailedHandler) Process(ctx context.Context, data interface{}) error {
	event := data.(assessmentDomain.AssessmentFailedEvent)
	_ = event

	// TODO: 监控告警

	return nil
}
