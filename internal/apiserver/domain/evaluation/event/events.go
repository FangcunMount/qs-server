package event

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 事件类型常量 ====================
// 从 eventcatalog 包导入，保持事件类型的单一来源

const (
	// TypeSubmitted 测评已提交
	TypeSubmitted = eventcatalog.AssessmentSubmitted
	// TypeEvaluated 测评已计分
	TypeEvaluated = eventcatalog.AssessmentEvaluated
	// TypeInterpreted 测评已解读（结果投影见 events_outcome.go）
	TypeInterpreted = eventcatalog.AssessmentInterpreted
	// TypeFailed 测评失败
	TypeFailed = eventcatalog.AssessmentFailed
)

// AggregateType 聚合根类型
const AggregateType = "Assessment"

// DomainEvent 重新导出共享内核的 DomainEvent 接口
type DomainEvent = event.DomainEvent

// ==================== 事件 Payload 定义 ====================

// SubmittedData 测评已提交事件数据
type SubmittedData = eventpayload.AssessmentSubmittedData

// FailedData 测评失败事件数据
type FailedData = eventpayload.AssessmentFailedData

// EvaluatedData 测评已计分事件数据
type EvaluatedData = eventpayload.AssessmentEvaluatedData

// ==================== 事件类型别名 ====================

// SubmittedEvent 测评已提交事件
type SubmittedEvent = event.Event[SubmittedData]

// FailedEvent 测评失败事件
type FailedEvent = event.Event[FailedData]

// EvaluatedEvent 测评已计分事件
type EvaluatedEvent = event.Event[EvaluatedData]

// SubmittedInput 构造 submitted 事件所需字段。
type SubmittedInput struct {
	OrgID             int64
	AssessmentID      int64
	TesteeID          uint64
	QuestionnaireCode string
	QuestionnaireVer  string
	AnswerSheetID     string
	ModelKind         string
	ModelSubKind      string
	ModelAlgorithm    string
	ModelCode         string
	ModelVersion      string
	ScaleCode         string
	ScaleVersion      string
	SubmittedAt       time.Time
}

// NewSubmittedEvent 创建测评已提交事件
func NewSubmittedEvent(in SubmittedInput) SubmittedEvent {
	data := SubmittedData{
		OrgID:             in.OrgID,
		AssessmentID:      in.AssessmentID,
		TesteeID:          in.TesteeID,
		QuestionnaireCode: in.QuestionnaireCode,
		QuestionnaireVer:  in.QuestionnaireVer,
		AnswerSheetID:     in.AnswerSheetID,
		SubmittedAt:       in.SubmittedAt,
		ModelKind:         in.ModelKind,
		ModelSubKind:      in.ModelSubKind,
		ModelAlgorithm:    in.ModelAlgorithm,
		ModelCode:         in.ModelCode,
		ModelVersion:      in.ModelVersion,
		ScaleCode:         in.ScaleCode,
		ScaleVersion:      in.ScaleVersion,
	}
	return event.New(TypeSubmitted, AggregateType, strconv.FormatInt(in.AssessmentID, 10), data)
}

// NewFailedEvent 创建测评失败事件
func NewFailedEvent(
	orgID int64,
	assessmentID int64,
	testeeID uint64,
	reason string,
	failedAt time.Time,
) FailedEvent {
	return event.New(TypeFailed, AggregateType, strconv.FormatInt(assessmentID, 10),
		FailedData{
			OrgID:        orgID,
			AssessmentID: assessmentID,
			TesteeID:     testeeID,
			Reason:       reason,
			FailedAt:     failedAt,
		},
	)
}

// NewEvaluatedEvent 创建测评已计分事件
func NewEvaluatedEvent(
	orgID int64,
	assessmentID int64,
	testeeID uint64,
	evaluatedAt time.Time,
) EvaluatedEvent {
	return event.New(TypeEvaluated, AggregateType, strconv.FormatInt(assessmentID, 10),
		EvaluatedData{
			OrgID:        orgID,
			AssessmentID: assessmentID,
			TesteeID:     testeeID,
			EvaluatedAt:  evaluatedAt,
		},
	)
}
