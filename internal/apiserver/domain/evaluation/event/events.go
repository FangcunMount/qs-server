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
	TypeRequested        = eventcatalog.EvaluationRequested
	TypeOutcomeCommitted = eventcatalog.EvaluationOutcomeCommitted
	TypeFailed           = eventcatalog.EvaluationFailed
	// Deprecated identifiers resolve to the new wire contract.
	TypeSubmitted = TypeRequested
	TypeEvaluated = TypeOutcomeCommitted
)

// AggregateType 聚合根类型
const AggregateType = "Evaluation"

// DomainEvent 重新导出共享内核的 DomainEvent 接口
type DomainEvent = event.DomainEvent

// ==================== 事件 Payload 定义 ====================

type RequestedData = eventpayload.EvaluationRequestedData

type FailedData = eventpayload.EvaluationFailedData

type OutcomeCommittedData = eventpayload.EvaluationOutcomeCommittedData

// ==================== 事件类型别名 ====================

type RequestedEvent = event.Event[RequestedData]

// FailedEvent 测评失败事件
type FailedEvent = event.Event[FailedData]

type OutcomeCommittedEvent = event.Event[OutcomeCommittedData]

// RequestedInput constructs an evaluation requested event.
type RequestedInput struct {
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
	RequestedAt       time.Time
}

func NewRequestedEvent(in RequestedInput) RequestedEvent {
	data := RequestedData{
		OrgID:             in.OrgID,
		AssessmentID:      in.AssessmentID,
		TesteeID:          in.TesteeID,
		QuestionnaireCode: in.QuestionnaireCode,
		QuestionnaireVer:  in.QuestionnaireVer,
		AnswerSheetID:     in.AnswerSheetID,
		RequestedAt:       in.RequestedAt,
		ModelKind:         in.ModelKind,
		ModelSubKind:      in.ModelSubKind,
		ModelAlgorithm:    in.ModelAlgorithm,
		ModelCode:         in.ModelCode,
		ModelVersion:      in.ModelVersion,
		ScaleCode:         in.ScaleCode,
		ScaleVersion:      in.ScaleVersion,
	}
	return event.New(TypeRequested, AggregateType, strconv.FormatInt(in.AssessmentID, 10), data)
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

func NewOutcomeCommittedEvent(
	orgID int64,
	assessmentID int64,
	testeeID uint64,
	outcomeID string,
	evaluationRunID string,
	committedAt time.Time,
) OutcomeCommittedEvent {
	return event.New(TypeOutcomeCommitted, AggregateType, strconv.FormatInt(assessmentID, 10),
		OutcomeCommittedData{
			OrgID:           orgID,
			AssessmentID:    assessmentID,
			TesteeID:        testeeID,
			OutcomeID:       outcomeID,
			EvaluationRunID: evaluationRunID,
			CommittedAt:     committedAt,
		},
	)
}
