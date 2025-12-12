package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventconfig 包导入，保持事件类型的单一来源

const (
	// EventTypeSubmitted 测评已提交
	EventTypeSubmitted = eventconfig.AssessmentSubmitted
	// EventTypeInterpreted 测评已解读
	EventTypeInterpreted = eventconfig.AssessmentInterpreted
	// EventTypeFailed 测评失败
	EventTypeFailed = eventconfig.AssessmentFailed
)

// AggregateType 聚合根类型
const AggregateType = "Assessment"

// 重新导出共享内核的 DomainEvent 接口
type DomainEvent = event.DomainEvent

// ==================== 事件 Payload 定义 ====================

// AssessmentSubmittedData 测评已提交事件数据
type AssessmentSubmittedData struct {
	AssessmentID      int64     `json:"assessment_id"`
	TesteeID          uint64    `json:"testee_id"`
	QuestionnaireCode string    `json:"questionnaire_code"`
	QuestionnaireVer  string    `json:"questionnaire_version"`
	AnswerSheetID     string    `json:"answersheet_id"`
	ScaleCode         string    `json:"scale_code,omitempty"`
	ScaleVersion      string    `json:"scale_version,omitempty"`
	SubmittedAt       time.Time `json:"submitted_at"`
}

// NeedsEvaluation 是否需要评估（有量表才需要）
func (d AssessmentSubmittedData) NeedsEvaluation() bool {
	return d.ScaleCode != ""
}

// AssessmentInterpretedData 测评已解读事件数据
type AssessmentInterpretedData struct {
	AssessmentID  int64     `json:"assessment_id"`
	TesteeID      uint64    `json:"testee_id"`
	ScaleCode     string    `json:"scale_code"`
	ScaleVersion  string    `json:"scale_version"`
	TotalScore    float64   `json:"total_score"`
	RiskLevel     string    `json:"risk_level"`
	InterpretedAt time.Time `json:"interpreted_at"`
}

// IsHighRisk 是否高风险
func (d AssessmentInterpretedData) IsHighRisk() bool {
	return IsHighRisk(RiskLevel(d.RiskLevel))
}

// AssessmentFailedData 测评失败事件数据
type AssessmentFailedData struct {
	AssessmentID int64     `json:"assessment_id"`
	TesteeID     uint64    `json:"testee_id"`
	Reason       string    `json:"reason"`
	FailedAt     time.Time `json:"failed_at"`
}

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
	assessmentID ID,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	medicalScaleRef *MedicalScaleRef,
	submittedAt time.Time,
) AssessmentSubmittedEvent {
	data := AssessmentSubmittedData{
		AssessmentID:      int64(assessmentID),
		TesteeID:          uint64(testeeID),
		QuestionnaireCode: string(questionnaireRef.Code()),
		QuestionnaireVer:  questionnaireRef.Version(),
		AnswerSheetID:     strconv.FormatInt(int64(answerSheetRef.ID()), 10),
		SubmittedAt:       submittedAt,
	}
	if medicalScaleRef != nil && !medicalScaleRef.IsEmpty() {
		data.ScaleCode = string(medicalScaleRef.Code())
		data.ScaleVersion = medicalScaleRef.Name()
	}

	return event.New(EventTypeSubmitted, AggregateType, strconv.FormatInt(int64(assessmentID), 10), data)
}

// NewAssessmentInterpretedEvent 创建测评已解读事件
func NewAssessmentInterpretedEvent(
	assessmentID ID,
	testeeID testee.ID,
	medicalScaleRef MedicalScaleRef,
	totalScore float64,
	riskLevel RiskLevel,
	interpretedAt time.Time,
) AssessmentInterpretedEvent {
	return event.New(EventTypeInterpreted, AggregateType, strconv.FormatInt(int64(assessmentID), 10),
		AssessmentInterpretedData{
			AssessmentID:  int64(assessmentID),
			TesteeID:      uint64(testeeID),
			ScaleCode:     string(medicalScaleRef.Code()),
			ScaleVersion:  medicalScaleRef.Name(),
			TotalScore:    totalScore,
			RiskLevel:     string(riskLevel),
			InterpretedAt: interpretedAt,
		},
	)
}

// NewAssessmentFailedEvent 创建测评失败事件
func NewAssessmentFailedEvent(
	assessmentID ID,
	testeeID testee.ID,
	reason string,
	failedAt time.Time,
) AssessmentFailedEvent {
	return event.New(EventTypeFailed, AggregateType, strconv.FormatInt(int64(assessmentID), 10),
		AssessmentFailedData{
			AssessmentID: int64(assessmentID),
			TesteeID:     uint64(testeeID),
			Reason:       reason,
			FailedAt:     failedAt,
		},
	)
}
