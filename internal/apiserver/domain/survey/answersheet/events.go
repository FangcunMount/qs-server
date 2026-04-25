package answersheet

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventconfig 包导入，保持事件类型的单一来源

const (
	// EventTypeSubmitted 答卷已提交
	EventTypeSubmitted = eventcatalog.AnswerSheetSubmitted
)

// AggregateType 聚合根类型
const AggregateType = "AnswerSheet"

// ==================== 事件 Payload 定义 ====================

// AnswerSheetSubmittedData 答卷已提交事件数据
type AnswerSheetSubmittedData struct {
	AnswerSheetID        string    `json:"answersheet_id"`
	QuestionnaireCode    string    `json:"questionnaire_code"`
	QuestionnaireVersion string    `json:"questionnaire_version"`
	TesteeID             uint64    `json:"testee_id"`   // 受试者ID（传递给测评层）
	OrgID                uint64    `json:"org_id"`      // 组织ID（传递给测评层）
	FillerID             uint64    `json:"filler_id"`   // 填写人ID
	FillerType           string    `json:"filler_type"` // 填写人类型
	TaskID               string    `json:"task_id,omitempty"`
	SubmittedAt          time.Time `json:"submitted_at"`
}

// ==================== 事件类型别名 ====================

// AnswerSheetSubmittedEvent 答卷已提交事件
type AnswerSheetSubmittedEvent = event.Event[AnswerSheetSubmittedData]

// ==================== 事件构造函数 ====================

// NewAnswerSheetSubmittedEvent 构造答卷提交事件
func NewAnswerSheetSubmittedEvent(sheet *AnswerSheet, testeeID, orgID uint64, taskID string) AnswerSheetSubmittedEvent {
	code, ver, _ := sheet.QuestionnaireInfo()
	filler := sheet.Filler()
	fillerID, err := safeconv.Int64ToUint64(filler.UserID())
	if err != nil {
		panic(fmt.Errorf("answersheet filler id: %w", err))
	}

	return event.New(EventTypeSubmitted, AggregateType, sheet.ID().String(),
		AnswerSheetSubmittedData{
			AnswerSheetID:        sheet.ID().String(),
			QuestionnaireCode:    code,
			QuestionnaireVersion: ver,
			TesteeID:             testeeID,
			OrgID:                orgID,
			FillerID:             fillerID,
			FillerType:           filler.FillerType().String(),
			TaskID:               taskID,
			SubmittedAt:          sheet.FilledAt(),
		},
	)
}
