package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evaldomainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const (
	EventTypeRequested        = evaldomainevent.TypeRequested
	EventTypeOutcomeCommitted = evaldomainevent.TypeOutcomeCommitted
	EventTypeFailed           = evaldomainevent.TypeFailed
	// Deprecated identifiers resolve to the new wire contract.
	EventTypeSubmitted = EventTypeRequested
	EventTypeEvaluated = EventTypeOutcomeCommitted
)

const AggregateType = evaldomainevent.AggregateType

type DomainEvent = evaldomainevent.DomainEvent

type EvaluationRequestedData = eventpayload.EvaluationRequestedData
type EvaluationFailedData = eventpayload.EvaluationFailedData
type EvaluationOutcomeCommittedData = eventpayload.EvaluationOutcomeCommittedData

type EvaluationRequestedEvent = event.Event[EvaluationRequestedData]
type EvaluationFailedEvent = event.Event[EvaluationFailedData]
type EvaluationOutcomeCommittedEvent = event.Event[EvaluationOutcomeCommittedData]

// Deprecated type aliases keep tests and internal fixtures source-compatible
// while publishing only the new event types.
type AssessmentSubmittedEvent = EvaluationRequestedEvent
type AssessmentEvaluatedEvent = EvaluationOutcomeCommittedEvent
type AssessmentFailedEvent = EvaluationFailedEvent

func NewEvaluationRequestedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	modelRef *EvaluationModelRef,
	submittedAt time.Time,
) EvaluationRequestedEvent {
	in := evaldomainevent.RequestedInput{
		OrgID:             orgID,
		AssessmentID:      int64(assessmentID),
		TesteeID:          testeeID.Uint64(),
		QuestionnaireCode: string(questionnaireRef.Code()),
		QuestionnaireVer:  questionnaireRef.Version(),
		AnswerSheetID:     strconv.FormatInt(int64(answerSheetRef.ID()), 10),
		RequestedAt:       submittedAt,
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
	return evaldomainevent.NewRequestedEvent(in)
}

func NewEvaluationFailedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	reason string,
	failedAt time.Time,
) EvaluationFailedEvent {
	return evaldomainevent.NewFailedEvent(orgID, int64(assessmentID), testeeID.Uint64(), reason, failedAt)
}

func NewEvaluationOutcomeCommittedEvent(
	orgID int64,
	assessmentID ID,
	testeeID testee.ID,
	outcomeID meta.ID,
	evaluationRunID evalrun.ID,
	evaluatedAt time.Time,
) EvaluationOutcomeCommittedEvent {
	return evaldomainevent.NewOutcomeCommittedEvent(
		orgID,
		int64(assessmentID),
		testeeID.Uint64(),
		outcomeID.String(),
		evaluationRunID.String(),
		evaluatedAt,
	)
}
