package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evaldomainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	EventTypeSubmitted   = evaldomainevent.TypeSubmitted
	EventTypeEvaluated   = evaldomainevent.TypeEvaluated
	EventTypeInterpreted = evaldomainevent.TypeInterpreted
	EventTypeFailed      = evaldomainevent.TypeFailed
)

const AggregateType = evaldomainevent.AggregateType

type DomainEvent = evaldomainevent.DomainEvent

type AssessmentSubmittedData = eventpayload.AssessmentSubmittedData
type AssessmentFailedData = eventpayload.AssessmentFailedData
type AssessmentEvaluatedData = eventpayload.AssessmentEvaluatedData

type AssessmentSubmittedEvent = event.Event[AssessmentSubmittedData]
type AssessmentFailedEvent = event.Event[AssessmentFailedData]
type AssessmentEvaluatedEvent = event.Event[AssessmentEvaluatedData]

// NewAssessmentSubmittedEvent 创建测评已提交事件
func NewAssessmentSubmittedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	modelRef *EvaluationModelRef,
	submittedAt time.Time,
) AssessmentSubmittedEvent {
	in := evaldomainevent.SubmittedInput{
		OrgID:             orgID,
		AssessmentID:      int64(assessmentID),
		TesteeID:          testeeID.Uint64(),
		QuestionnaireCode: string(questionnaireRef.Code()),
		QuestionnaireVer:  questionnaireRef.Version(),
		AnswerSheetID:     strconv.FormatInt(int64(answerSheetRef.ID()), 10),
		SubmittedAt:       submittedAt,
	}
	if modelRef != nil && !modelRef.IsEmpty() {
		in.ModelKind = modelRef.Kind().String()
		if subKind := modelRef.SubKind(); subKind != "" {
			in.ModelSubKind = string(subKind)
		}
		if algorithm := modelRef.Algorithm(); algorithm != "" {
			in.ModelAlgorithm = string(algorithm)
		}
		in.ModelCode = modelRef.Code().String()
		in.ModelVersion = modelRef.Version()
	}
	return evaldomainevent.NewSubmittedEvent(in)
}

// NewAssessmentFailedEvent 创建测评失败事件
func NewAssessmentFailedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	reason string,
	failedAt time.Time,
) AssessmentFailedEvent {
	return evaldomainevent.NewFailedEvent(orgID, int64(assessmentID), testeeID.Uint64(), reason, failedAt)
}

// NewAssessmentEvaluatedEvent 创建测评已计分事件
func NewAssessmentEvaluatedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	evaluatedAt time.Time,
) AssessmentEvaluatedEvent {
	return evaldomainevent.NewEvaluatedEvent(orgID, int64(assessmentID), testeeID.Uint64(), evaluatedAt)
}
