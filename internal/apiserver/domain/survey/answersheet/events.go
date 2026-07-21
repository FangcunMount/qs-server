package answersheet

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// ==================== 事件类型常量 ====================
// 从 eventcatalog 包导入，保持事件类型的单一来源

const (
	// EventTypeSubmitted 答卷已提交
	EventTypeSubmitted = eventcatalog.AnswerSheetSubmitted
)

// AggregateType 聚合根类型
const AggregateType = "AnswerSheet"

// ==================== 事件 Payload 定义 ====================

// AnswerSheetSubmittedData 答卷已提交事件数据
type AnswerSheetSubmittedData = eventpayload.AnswerSheetSubmittedData

// ==================== 事件类型别名 ====================

// AnswerSheetSubmittedEvent 答卷已提交事件
type AnswerSheetSubmittedEvent = event.Event[AnswerSheetSubmittedData]

// ==================== 事件构造函数 ====================

// NewAnswerSheetSubmittedEvent 构造答卷提交事件。
func NewAnswerSheetSubmittedEvent(sheet *AnswerSheet) AnswerSheetSubmittedEvent {
	code, ver, _ := sheet.QuestionnaireInfo()
	filler := sheet.Filler()
	fillerID, err := safeconv.Int64ToUint64(filler.UserID())
	if err != nil {
		panic(fmt.Errorf("answersheet filler id: %w", err))
	}
	submissionContext := sheet.SubmissionContext()
	attribution := submissionContext.Attribution()
	var attributionPayload *eventpayload.AttributionSnapshot
	if !attribution.IsZero() {
		attributionPayload = &eventpayload.AttributionSnapshot{
			OriginType: string(attribution.OriginType()), OriginID: attribution.OriginID(), ClinicianID: attribution.ClinicianID(),
			EntryID: attribution.EntryID(), PlanID: attribution.PlanID(), EnrollmentID: attribution.EnrollmentID(), TaskID: attribution.TaskID(),
			CapturedAt: attribution.CapturedAt(), Version: attribution.Version(), Mode: string(attribution.Mode()),
		}
	}
	testeeID, err := safeconv.MetaIDToUint64(submissionContext.TesteeID())
	if err != nil {
		panic(fmt.Errorf("answersheet testee id: %w", err))
	}
	orgID, err := safeconv.MetaIDToUint64(submissionContext.OrgID())
	if err != nil {
		panic(fmt.Errorf("answersheet org id: %w", err))
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
			TaskID:               submissionContext.TaskID(),
			SubmittedAt:          sheet.FilledAt(),
			Admission:            submissionContext.Admission().ToEventPayload(),
			Attribution:          attributionPayload,
		},
	)
}
