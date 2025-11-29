package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
)

// AssessmentSubmittedHandler 处理答卷提交事件
type AssessmentSubmittedHandler struct {
	*BaseHandler
	logger            *slog.Logger
	answerSheetClient *grpcclient.AnswerSheetClient
}

// NewAssessmentSubmittedHandler 创建答卷提交事件处理器
func NewAssessmentSubmittedHandler(
	logger *slog.Logger,
	answerSheetClient *grpcclient.AnswerSheetClient,
) *AssessmentSubmittedHandler {
	return &AssessmentSubmittedHandler{
		BaseHandler:       NewBaseHandler("assessment.submitted", "assessment_submitted_handler"),
		logger:            logger,
		answerSheetClient: answerSheetClient,
	}
}

// Handle 处理答卷提交事件
func (h *AssessmentSubmittedHandler) Handle(ctx context.Context, payload []byte) error {
	var event assessment.AssessmentSubmittedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		h.logger.Error("failed to unmarshal assessment submitted event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing assessment submitted event",
		slog.String("handler", h.Name()),
		slog.String("event_id", event.EventID()),
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
		slog.String("testee_id", fmt.Sprintf("%d", event.TesteeID())),
	)

	// 1. 通过 gRPC 获取答卷详情
	answerSheetID := uint64(event.AnswerSheetRef().ID())
	answerSheetResp, err := h.answerSheetClient.GetAnswerSheet(ctx, answerSheetID)
	if err != nil {
		h.logger.Error("failed to get answer sheet",
			slog.String("handler", h.Name()),
			slog.Uint64("answer_sheet_id", answerSheetID),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Debug("answer sheet loaded",
		slog.String("handler", h.Name()),
		slog.Uint64("answer_sheet_id", answerSheetID),
		slog.Int("answers_count", len(answerSheetResp.AnswerSheet.Answers)),
	)

	// TODO: 后续步骤
	// 2. 加载量表计算规则
	// 3. 执行分数计算
	// 4. 生成解读结果
	// 5. 发布 AssessmentInterpretedEvent

	h.logger.Info("assessment submitted event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("assessment_id", fmt.Sprintf("%d", event.AssessmentID())),
	)

	return nil
}
