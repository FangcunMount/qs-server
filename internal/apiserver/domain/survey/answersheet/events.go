package answersheet

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventconfig 包导入，保持事件类型的单一来源

const (
	// EventTypeSubmitted 答卷已提交
	EventTypeSubmitted = eventconfig.AnswerSheetSubmitted
)

// AggregateType 聚合根类型
const AggregateType = "AnswerSheet"

// ==================== 事件 Payload 定义 ====================

// AnswerSheetSubmittedData 答卷已提交事件数据
type AnswerSheetSubmittedData struct {
	AnswerSheetID        string    `json:"answersheet_id"`
	QuestionnaireCode    string    `json:"questionnaire_code"`
	QuestionnaireVersion string    `json:"questionnaire_version"`
	FillerID             uint64    `json:"filler_id"`
	FillerType           string    `json:"filler_type"`
	SubmittedAt          time.Time `json:"submitted_at"`
}

// ==================== 事件类型别名 ====================

// AnswerSheetSubmittedEvent 答卷已提交事件
type AnswerSheetSubmittedEvent = event.Event[AnswerSheetSubmittedData]

// ==================== 事件构造函数 ====================

// NewAnswerSheetSubmittedEvent 构造答卷提交事件
func NewAnswerSheetSubmittedEvent(sheet *AnswerSheet) AnswerSheetSubmittedEvent {
	code, ver, _ := sheet.QuestionnaireInfo()
	filler := sheet.Filler()

	return event.New(EventTypeSubmitted, AggregateType, sheet.ID().String(),
		AnswerSheetSubmittedData{
			AnswerSheetID:        sheet.ID().String(),
			QuestionnaireCode:    code,
			QuestionnaireVersion: ver,
			FillerID:             uint64(filler.UserID()),
			FillerType:           filler.FillerType().String(),
			SubmittedAt:          sheet.FilledAt(),
		},
	)
}
