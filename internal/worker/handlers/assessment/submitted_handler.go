package assessment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	assessmentDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/worker/handlers/core"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
)

// init 自动注册处理器
func init() {
	core.RegisterMessageHandler(core.TopicAssessmentLifecycle, func(deps *core.HandlerDependencies) core.MessageHandler {
		var answerSheetClient *grpcclient.AnswerSheetClient
		if client, ok := deps.Extra["answerSheetClient"]; ok {
			answerSheetClient = client.(*grpcclient.AnswerSheetClient)
		}
		return NewSubmittedHandler(deps.Logger, answerSheetClient)
	})
}

// SubmittedHandler 测评提交消息处理器
type SubmittedHandler struct {
	*core.TemplateMessageHandler
	answerSheetClient *grpcclient.AnswerSheetClient
}

// NewSubmittedHandler 创建测评提交消息处理器
func NewSubmittedHandler(
	logger *slog.Logger,
	answerSheetClient *grpcclient.AnswerSheetClient,
) *SubmittedHandler {
	return &SubmittedHandler{
		TemplateMessageHandler: core.NewTemplateMessageHandler(core.EventAssessmentSubmitted, logger),
		answerSheetClient:      answerSheetClient,
	}
}

// Handle 处理测评提交消息（调用模板方法）
func (h *SubmittedHandler) Handle(ctx context.Context, payload []byte) error {
	return h.Execute(ctx, payload, h)
}

// ==================== 实现钩子接口 ====================

// Parse 实现 MessageParser 接口
func (h *SubmittedHandler) Parse(payload []byte) (interface{}, error) {
	var event assessmentDomain.AssessmentSubmittedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	return event, nil
}

// Process 实现 MessageProcessor 接口
func (h *SubmittedHandler) Process(ctx context.Context, data interface{}) error {
	event := data.(assessmentDomain.AssessmentSubmittedEvent)

	// 调用 gRPC 获取答卷
	answerSheetID := uint64(event.AnswerSheetRef().ID())
	_, err := h.answerSheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		return fmt.Errorf("failed to get answer sheet: %w", err)
	}

	// TODO: 执行计算逻辑

	return nil
}
