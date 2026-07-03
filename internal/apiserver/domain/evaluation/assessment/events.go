package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventcatalog 包导入，保持事件类型的单一来源

const (
	// EventTypeSubmitted 测评已提交
	EventTypeSubmitted = eventcatalog.AssessmentSubmitted
	// EventTypeInterpreted 测评已解读
	EventTypeInterpreted = eventcatalog.AssessmentInterpreted
	// EventTypeFailed 测评失败
	EventTypeFailed = eventcatalog.AssessmentFailed
)

// AggregateType 聚合根类型
const AggregateType = "Assessment"

// 重新导出共享内核的 DomainEvent 接口
type DomainEvent = event.DomainEvent

// ==================== 事件 Payload 定义 ====================

// AssessmentSubmittedData 测评已提交事件数据
type AssessmentSubmittedData = eventpayload.AssessmentSubmittedData

// AssessmentInterpretedData 测评已解读事件数据
type AssessmentInterpretedData = eventpayload.AssessmentInterpretedData

// AssessmentFailedData 测评失败事件数据
type AssessmentFailedData = eventpayload.AssessmentFailedData

// ==================== 事件类型别名 ====================

// AssessmentSubmittedEvent 测评已提交事件
type AssessmentSubmittedEvent = event.Event[AssessmentSubmittedData]

// AssessmentInterpretedEvent 测评已解读事件
type AssessmentInterpretedEvent = event.Event[AssessmentInterpretedData]

// AssessmentFailedEvent 测评失败事件
type AssessmentFailedEvent = event.Event[AssessmentFailedData]

// ==================== 事件构造函数 ====================

// NewAssessmentSubmittedEvent 创建测评已提交事件
func NewAssessmentSubmittedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	modelRef *EvaluationModelRef,
	medicalScaleRef *MedicalScaleRef,
	submittedAt time.Time,
) AssessmentSubmittedEvent {
	data := AssessmentSubmittedData{
		OrgID:             orgID,
		AssessmentID:      int64(assessmentID),
		TesteeID:          testeeID.Uint64(),
		QuestionnaireCode: string(questionnaireRef.Code()),
		QuestionnaireVer:  questionnaireRef.Version(),
		AnswerSheetID:     strconv.FormatInt(int64(answerSheetRef.ID()), 10),
		SubmittedAt:       submittedAt,
	}
	if modelRef != nil && !modelRef.IsEmpty() {
		data.ModelKind = modelRef.Kind().String()
		data.ModelCode = modelRef.Code().String()
		data.ModelVersion = modelRef.Version()
	}
	if medicalScaleRef != nil && !medicalScaleRef.IsEmpty() {
		data.ScaleCode = string(medicalScaleRef.Code())
		data.ScaleVersion = medicalScaleRef.Version()
		if data.ModelKind == "" {
			data.ModelKind = EvaluationModelKindScale.String()
			data.ModelCode = data.ScaleCode
			data.ModelVersion = data.ScaleVersion
		}
	}

	return event.New(EventTypeSubmitted, AggregateType, strconv.FormatInt(int64(assessmentID), 10), data)
}

// NewAssessmentInterpretedEvent 创建测评已解读事件
func NewAssessmentInterpretedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	modelRef EvaluationModelRef,
	medicalScaleRef MedicalScaleRef,
	totalScore float64,
	riskLevel RiskLevel,
	interpretedAt time.Time,
) AssessmentInterpretedEvent {
	return event.New(EventTypeInterpreted, AggregateType, strconv.FormatInt(int64(assessmentID), 10),
		AssessmentInterpretedData{
			OrgID:         orgID,
			AssessmentID:  int64(assessmentID),
			TesteeID:      testeeID.Uint64(),
			ModelKind:     modelRef.Kind().String(),
			ModelCode:     modelRef.Code().String(),
			ModelVersion:  modelRef.Version(),
			ScaleCode:     string(medicalScaleRef.Code()),
			ScaleVersion:  medicalScaleRef.Version(),
			TotalScore:    totalScore,
			RiskLevel:     string(riskLevel),
			InterpretedAt: interpretedAt,
		},
	)
}

// NewAssessmentModelInterpretedEvent 创建通用解释模型已解读事件。
func NewAssessmentModelInterpretedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	modelRef EvaluationModelRef,
	totalScore float64,
	riskLevel RiskLevel,
	interpretedAt time.Time,
) AssessmentInterpretedEvent {
	return event.New(EventTypeInterpreted, AggregateType, strconv.FormatInt(int64(assessmentID), 10),
		AssessmentInterpretedData{
			OrgID:         orgID,
			AssessmentID:  int64(assessmentID),
			TesteeID:      testeeID.Uint64(),
			ModelKind:     modelRef.Kind().String(),
			ModelCode:     modelRef.Code().String(),
			ModelVersion:  modelRef.Version(),
			TotalScore:    totalScore,
			RiskLevel:     string(riskLevel),
			InterpretedAt: interpretedAt,
		},
	)
}

// NewAssessmentFailedEvent 创建测评失败事件
func NewAssessmentFailedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	reason string,
	failedAt time.Time,
) AssessmentFailedEvent {
	return event.New(EventTypeFailed, AggregateType, strconv.FormatInt(int64(assessmentID), 10),
		AssessmentFailedData{
			OrgID:        orgID,
			AssessmentID: int64(assessmentID),
			TesteeID:     testeeID.Uint64(),
			Reason:       reason,
			FailedAt:     failedAt,
		},
	)
}
