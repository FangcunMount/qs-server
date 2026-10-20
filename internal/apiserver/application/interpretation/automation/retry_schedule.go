package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	storeport "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	outcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

type GovernedRetryOption func(*governedRetryService)
type futureRetrySchedules struct {
	store   storeport.Store
	builder storeport.PlanBuilder
	guard   storeport.AuthorizationGuard
}

func WithRetrySchedules(store storeport.Store, builder storeport.PlanBuilder, guard storeport.AuthorizationGuard) GovernedRetryOption {
	return func(s *governedRetryService) {
		s.future = &futureRetrySchedules{store: store, builder: builder, guard: guard}
	}
}

func (s *governedRetryService) authorizeScheduled(ctx context.Context, command GovernedRetryCommand, g *generation.ReportGeneration, fact *evaluationfact.Record) (*domainrun.InterpretationRun, error) {
	if s.future.store == nil || s.future.builder == nil || s.future.guard == nil {
		return nil, transaction.ErrBinding
	}
	if command.Origin != retrygovernance.AttemptOriginManual && command.Origin != retrygovernance.AttemptOriginForce {
		return nil, domainrun.ErrInvalidRetrySchedule
	}
	current, err := s.runs.FindByID(ctx, g.LatestRunID())
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status() != domainrun.StatusFailed || current.Attempt() != command.ExpectedAttempt || current.RetryDecision() == nil {
		return nil, domainrun.ErrInvalidRetrySchedule
	}
	at := s.now().UTC()
	evt := domain.NewInterpretationRetryRequestedEvent(domain.RetryRequestedEventInput{OrgID: command.OrgID, GenerationID: g.ID().String(), RunID: current.ID().String(), AssessmentID: fact.AssessmentID().String(), OutcomeID: fact.ID().String(), TesteeID: fact.TesteeID(), ExpectedAttempt: current.Attempt(), AttemptOrigin: string(command.Origin), ActionRequestID: command.RequestID, Mode: "next_attempt", RequestedAt: at})
	d := current.RetryDecision()
	expected := retrygovernance.DispositionManualRequired
	if command.Origin == retrygovernance.AttemptOriginForce {
		expected = retrygovernance.DispositionTerminal
	}
	duplicate := d.Disposition == retrygovernance.DispositionAutomatic && d.ActionRequestID == command.RequestID && d.RetryEventID == evt.EventID()
	if d.Disposition != expected && !duplicate {
		return nil, domainrun.ErrInvalidRetrySchedule
	}
	d.Disposition = retrygovernance.DispositionAutomatic
	d.NextAttemptAt = &at
	d.RetryEventID = evt.EventID()
	d.ActionRequestID = command.RequestID
	frozen, err := s.future.builder.Freeze(model.Authority{Owner: "qs.mongo", OrgID: command.OrgID, Kind: model.Interpretation, SubjectID: g.ID().String(), RunID: current.ID().String(), AssessmentID: fact.AssessmentID().Uint64(), OutcomeID: fact.ID().Uint64(), TesteeID: fact.TesteeID(), ExpectedAttempt: current.Attempt(), Origin: command.Origin, RequestID: command.RequestID}, evt, d, command)
	if err != nil {
		return nil, err
	}
	if _, err := s.future.store.Find(ctx, frozen); err == nil {
		return s.recoverScheduled(ctx, frozen, current.ID())
	} else if !errors.Is(err, transaction.ErrNotFound) {
		return nil, errors.Join(events.ErrUnresolved, err)
	}
	if duplicate {
		return nil, events.ErrUnresolved
	}
	authorizer, ok := s.runs.(domainrun.RetryAuthorizer)
	if !ok {
		return nil, transaction.ErrBinding
	}
	rootErr := s.tx.WithinTransaction(ctx, func(root context.Context) error {
		winner, err := s.future.store.SaveFrozen(root, frozen)
		if err != nil {
			return err
		}
		if err := s.future.guard.GuardAuthorization(root, winner); err != nil {
			return err
		}
		// Derive DB-only identity evidence from the immutable winner, not from a
		// candidate with a later clock. No new wire, ID, signing, MQ or model I/O.
		proof, err := scheduledRetryEvidence(winner)
		if err != nil {
			return err
		}
		actual, err := authorizer.AuthorizeRetry(root, domainrun.RetryAuthorizationRequest{GenerationID: g.ID(), ExpectedAttempt: current.Attempt(), Origin: command.Origin, RequestID: command.RequestID, EventID: winner.Message().Input().ID, AuthorizedAt: winner.DueAt(), EventEvidence: proof})
		if err != nil {
			return err
		}
		if actual == nil || actual.ID().String() != winner.Authority().RunID || winner.MatchDecision(actual.RetryDecision(), winner.DueAt()) != nil {
			return model.ErrConflict
		}
		return nil
	})
	accepted, recoverErr := s.recoverScheduled(ctx, frozen, current.ID())
	if recoverErr != nil {
		return nil, errors.Join(events.ErrUnresolved, rootErr, recoverErr)
	}
	return accepted, nil
}

func scheduledRetryEvidence(f model.Frozen) (*evidence.EventEvidenceV1, error) {
	msg := f.Message()
	ref := standard.ReferenceFromMessage(msg)
	wire, err := standard.VerifyReference(msg.Input(), ref.Fingerprint, ref)
	if err != nil {
		return nil, err
	}
	var data outcome.InterpretationRetryRequestedPayload
	if json.Unmarshal(wire.Data, &data) != nil {
		return nil, model.ErrInvalid
	}
	binding, err := eventevidencebinding.Retry(data)
	if err != nil {
		return nil, err
	}
	return evidence.NewStandard(ref, binding)
}

func (s *governedRetryService) recoverScheduled(ctx context.Context, candidate model.Frozen, id domainrun.ID) (*domainrun.InterpretationRun, error) {
	read, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	record, err := s.future.store.Find(read, candidate)
	if err != nil {
		return nil, err
	}
	if record == nil || record.Validate() != nil || record.State == model.Canceled {
		return nil, events.ErrUnresolved
	}
	current, err := s.runs.FindByID(read, id)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ID().String() != record.Plan.Authority().RunID || current.Status() != domainrun.StatusFailed || record.Plan.MatchDecision(current.RetryDecision(), record.Plan.BusinessDue()) != nil {
		return nil, events.ErrUnresolved
	}
	proof, err := scheduledRetryEvidence(record.Plan)
	if err != nil || current.RetryEventEvidence() == nil || current.RetryEventEvidence().Reference == nil || *current.RetryEventEvidence().Reference != *proof.Reference || current.RetryEventEvidence().BusinessBindingSHA256 != proof.BusinessBindingSHA256 {
		return nil, events.ErrUnresolved
	}
	return current, nil
}
