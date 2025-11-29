package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== AnswerSheetSavedEvent 定义 ====================

// AnswerSheetSavedEvent 答卷已保存事件
// 来源：collection-server
// 语义：用户填写的答卷已持久化保存
type AnswerSheetSavedEvent struct {
	event.BaseEvent

	answerSheetID     uint64
	questionnaireCode string
	questionnaireVer  string
	testeeID          uint64
	savedAt           time.Time
}

// AnswerSheetSavedEventDTO 用于 JSON 反序列化的 DTO
type AnswerSheetSavedEventDTO struct {
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type"`
	AggregateType     string `json:"aggregate_type"`
	AggregateID       string `json:"aggregate_id"`
	OccurredAt        string `json:"occurred_at"`
	AnswerSheetID     uint64 `json:"answer_sheet_id"`
	QuestionnaireCode string `json:"questionnaire_code"`
	QuestionnaireVer  string `json:"questionnaire_ver"`
	TesteeID          uint64 `json:"testee_id"`
	SavedAt           string `json:"saved_at"`
}

// ==================== AnswerSheetSavedHandler ====================

// AnswerSheetSavedHandler 处理答卷保存事件
// 职责：判断答卷是否关联量表，如果是则创建 Assessment
type AnswerSheetSavedHandler struct {
	*BaseHandler
	logger *slog.Logger
	// TODO: 注入 apiserver gRPC 客户端用于创建 Assessment
}

// NewAnswerSheetSavedHandler 创建答卷保存事件处理器
func NewAnswerSheetSavedHandler(logger *slog.Logger) *AnswerSheetSavedHandler {
	return &AnswerSheetSavedHandler{
		BaseHandler: NewBaseHandler("answersheet.saved", "answersheet_saved_handler"),
		logger:      logger,
	}
}

// Handle 处理答卷保存事件
func (h *AnswerSheetSavedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto AnswerSheetSavedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal answersheet saved event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing answersheet saved event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.Uint64("answer_sheet_id", dto.AnswerSheetID),
		slog.String("questionnaire_code", dto.QuestionnaireCode),
		slog.Uint64("testee_id", dto.TesteeID),
	)

	// 业务逻辑：
	// 1. 查询问卷是否关联量表
	// 2. 如果关联量表，调用 apiserver 创建 Assessment
	// 3. Assessment 创建后会自动发布 AssessmentSubmittedEvent

	// TODO: 实现业务逻辑
	// scaleRef, err := h.questionnaireClient.GetScaleRef(ctx, dto.QuestionnaireCode)
	// if err != nil { return err }
	//
	// if scaleRef != nil {
	//     // 有量表，创建 Assessment
	//     _, err := h.assessmentClient.CreateAssessment(ctx, &CreateAssessmentRequest{
	//         TesteeID:          dto.TesteeID,
	//         AnswerSheetID:     dto.AnswerSheetID,
	//         QuestionnaireCode: dto.QuestionnaireCode,
	//         ScaleCode:         scaleRef.Code,
	//     })
	//     if err != nil { return err }
	// }

	h.logger.Info("answersheet saved event processed successfully",
		slog.String("handler", h.Name()),
		slog.Uint64("answer_sheet_id", dto.AnswerSheetID),
	)

	return nil
}
