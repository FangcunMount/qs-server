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
		return NewInterpretedHandler(deps.Logger)
	})
}

// InterpretedHandler 测评解读完成消息处理器
type InterpretedHandler struct {
	*core.TemplateMessageHandler
}

// NewInterpretedHandler 创建测评解读完成消息处理器
func NewInterpretedHandler(logger *slog.Logger) *InterpretedHandler {
	return &InterpretedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventAssessmentInterpreted, logger),
	}
}

// Handle 处理测评解读完成消息（调用模板方法）
func (h *InterpretedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *InterpretedHandler) Parse(payload []byte) (interface{}, error) {
	var event assessmentDomain.AssessmentInterpretedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	return event, nil
}

// Process 实现 MessageProcessor 接口
func (h *InterpretedHandler) Process(ctx context.Context, data interface{}) error {
	event := data.(assessmentDomain.AssessmentInterpretedEvent)
	_ = event

	// TODO: 发送通知、预警、统计

	return nil
}
