package assessment

import (
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evaldomainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

const (
	EventTypeRequested        = evaldomainevent.TypeRequested
	EventTypeRetryRequested   = evaldomainevent.TypeRetryRequested
	EventTypeOutcomeCommitted = evaldomainevent.TypeOutcomeCommitted
	EventTypeFailed           = evaldomainevent.TypeFailed
)

const AggregateType = evaldomainevent.AggregateType

type DomainEvent = evaldomainevent.DomainEvent

type EvaluationRequestedData = eventpayload.EvaluationRequestedData
type EvaluationFailedData = eventpayload.EvaluationFailedData
type EvaluationOutcomeCommittedData = eventpayload.EvaluationOutcomeCommittedData

type EvaluationRequestedEvent = event.Event[EvaluationRequestedData]
type EvaluationFailedEvent = event.Event[EvaluationFailedData]
type EvaluationOutcomeCommittedEvent = event.Event[EvaluationOutcomeCommittedData]

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

// NewEvaluationRetryRequestedEvent creates a deterministic retry wake-up.
func NewEvaluationRetryRequestedEvent(a *Assessment, expectedAttempt int, origin retrygovernance.AttemptOrigin, actionRequestID string, requestedAt time.Time) EvaluationRequestedEvent {
	if a == nil {
		return EvaluationRequestedEvent{}
	}
	eventID := fmt.Sprintf("eval-retry:%d:%d:%s", a.ID(), expectedAttempt, origin)
	if actionRequestID != "" {
		eventID += ":" + actionRequestID
	}
	in := evaldomainevent.RequestedInput{
		EventID: eventID,
		OrgID:   a.OrgID(), AssessmentID: int64(a.ID()), TesteeID: a.TesteeID().Uint64(),
		QuestionnaireCode: string(a.QuestionnaireRef().Code()), QuestionnaireVer: a.QuestionnaireRef().Version(),
		AnswerSheetID: strconv.FormatInt(int64(a.AnswerSheetRef().ID()), 10), RequestedAt: requestedAt,
		ExpectedAttempt: expectedAttempt, AttemptOrigin: string(origin), ActionRequestID: actionRequestID, Mode: "next_attempt",
	}
	if modelRef := a.EvaluationModelRef(); modelRef != nil && !modelRef.IsEmpty() {
		in.ModelKind, in.ModelCode, in.ModelVersion = modelRef.Kind().String(), modelRef.Code().String(), modelRef.Version()
		in.ModelSubKind, in.ModelAlgorithm = string(modelRef.SubKind()), string(modelRef.Algorithm())
	}
	return evaldomainevent.NewRetryRequestedEvent(in)
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
