package evaluationfailure

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	schedule "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtransaction"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

type frozenFailure struct {
	Version    int
	Assessment *repository.AssessmentPO
	Run        evalrun.ReconstructInput
	Future     *message.Input
	Due        time.Time
}

func failureKey(id evalrun.ID) string { return "evaluation-failure/" + id.String() }

func snapshot(r evalrun.EvaluationRun) evalrun.ReconstructInput {
	return evalrun.ReconstructInput{RunID: r.ID(), AssessmentID: r.AssessmentID(), Attempt: r.Attempt(), Failure: r.Failure(), TraceID: r.TraceID(), InputSnapshotRef: r.InputSnapshotRef(), ClaimToken: r.ClaimToken(), LeaseExpiresAt: r.LeaseExpiresAt(), StartedAt: r.StartedAt(), FinishedAt: r.FinishedAt(), Origin: r.Origin(), ActionRequestID: r.ActionRequestID(), RetryDecision: r.RetryDecision()}
}

func freeze(q port.FailureRequest, resolver catalog.TopicResolver) (eventport.Request, error) {
	if q.Assessment == nil || q.Assessment.ID().IsZero() || !q.Assessment.Status().IsFailed() || q.Assessment.FailedAt() == nil || q.Run.Attempt().Status != evalrun.StatusFailed || q.Run.AssessmentID() != q.Assessment.ID().Uint64() || q.Run.FinishedAt() == nil || !q.Run.FinishedAt().Equal(*q.Assessment.FailedAt()) || len(q.Immediate) != 1 || q.Immediate[0].EventType() != assessment.EventTypeFailed {
		return eventport.Request{}, transaction.ErrInvalidPlan
	}
	f := frozenFailure{Version: 2, Assessment: repository.NewAssessmentMapper().ToPO(q.Assessment), Run: snapshot(q.Run)}
	if q.FutureRetry != nil {
		prepared, err := eventmessage.PrepareIntents([]assessment.DomainEvent{q.FutureRetry}, resolver, "api-server")
		if err != nil {
			return eventport.Request{}, err
		}
		in := prepared[0].Message.Input()
		f.Future = &in
		f.Due = q.RetryAt
	}
	if err := f.validate(); err != nil {
		return eventport.Request{}, err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return eventport.Request{}, err
	}
	hash, err := f.inputHash()
	if err != nil {
		return eventport.Request{}, err
	}
	return eventport.Request{Key: failureKey(f.Run.RunID), InputHash: hash, Artifact: raw, Events: q.Immediate}, nil
}

func (f frozenFailure) inputHash() ([32]byte, error) {
	// Keep the original stable input identity so an old winner is recovered,
	// never superseded by the new schedule ownership version.
	f.Version = 1
	po := *f.Assessment
	po.FailedAt = nil
	f.Assessment = &po
	f.Run.ClaimToken = ""
	f.Run.TraceID = ""
	f.Run.LeaseExpiresAt = nil
	f.Run.StartedAt = time.Time{}
	f.Run.FinishedAt = nil
	if f.Run.RetryDecision != nil {
		d := *f.Run.RetryDecision
		d.NextAttemptAt = nil
		f.Run.RetryDecision = &d
	}
	f.Future = nil
	f.Due = time.Time{}
	raw, err := json.Marshal(f)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

func (f frozenFailure) validate() error {
	a, r := f.Assessment, f.Run
	if (f.Version != 1 && f.Version != 2) || a == nil || a.ID.IsZero() || a.Status != assessment.StatusFailed.String() || a.FailedAt == nil || a.FailureReason == nil || r.RunID == "" || r.Attempt.Number < 1 || r.RunID != evalrun.NewEvaluationRunWithAttempt(r.AssessmentID, r.Attempt.Number).ID() || r.AssessmentID != a.ID.Uint64() || r.Attempt.Status != evalrun.StatusFailed || r.Failure == nil || r.FinishedAt == nil || !r.FinishedAt.Equal(*a.FailedAt) || r.RetryDecision == nil || repository.NewAssessmentMapper().ToDomain(a) == nil {
		return transaction.ErrInvalidPlan
	}
	d := r.RetryDecision
	if d.Disposition != retrygovernance.DispositionAutomatic {
		if d.NextAttemptAt != nil || f.Future != nil || !f.Due.IsZero() {
			return transaction.ErrInvalidPlan
		}
		return nil
	}
	if f.Future == nil || d.NextAttemptAt == nil || f.Due.IsZero() || !f.Due.Equal(*d.NextAttemptAt) || d.RetryEventID == "" || f.Future.ID != d.RetryEventID || f.Future.EventType != assessment.EventTypeRetryRequested || f.Future.Scope != fmt.Sprintf("org:%d", a.OrgID) {
		return transaction.ErrInvalidPlan
	}
	m, err := message.New(*f.Future)
	if err != nil {
		return err
	}
	ref := eventmessage.ReferenceFromMessage(m)
	envelope, err := eventmessage.VerifyReference(*f.Future, ref.Fingerprint, ref)
	if err != nil {
		return err
	}
	var body assessment.EvaluationRequestedData
	if json.Unmarshal(envelope.Data, &body) != nil || envelope.AggregateType != assessment.AggregateType || envelope.AggregateID != a.ID.String() || body.AssessmentID != int64(a.ID.Uint64()) || body.OrgID != a.OrgID || body.ExpectedAttempt != r.Attempt.Number || body.AttemptOrigin != string(retrygovernance.AttemptOriginAutomatic) || body.Mode != "next_attempt" || !body.RequestedAt.Equal(f.Due) {
		return transaction.ErrInvalidPlan
	}
	return nil
}

func read(raw []byte) (frozenFailure, error) {
	var f frozenFailure
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, err
	}
	canonical, err := json.Marshal(f)
	if err != nil || !bytes.Equal(raw, canonical) {
		return f, transaction.ErrInvalidPlan
	}
	return f, f.validate()
}

// retryPlan is materialized from the winning failure artifact before the root.
// V1 keeps its original Outbox obligation; only V2 creates a new business plan.
func (f frozenFailure) retryPlan() (schedule.Frozen, error) {
	if f.Version != 2 || f.Future == nil {
		return schedule.Frozen{}, transaction.ErrInvalidPlan
	}
	digest, err := schedule.DecisionDigest(f.Run.RetryDecision)
	if err != nil {
		return schedule.Frozen{}, err
	}
	a := f.Assessment
	return schedule.New(schedule.Authority{Owner: "qs.mysql", OrgID: a.OrgID, Kind: schedule.Evaluation, SubjectID: a.ID.String(), RunID: f.Run.RunID.String(), AssessmentID: a.ID.Uint64(), TesteeID: a.TesteeID, ExpectedAttempt: f.Run.Attempt.Number, Origin: retrygovernance.AttemptOriginAutomatic, DecisionHash: digest}, *f.Future, f.Due)
}
