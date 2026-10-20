package operator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	storeport "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

type GovernedRetryOption func(*governedRetryService)
type futureRetrySchedules struct {
	store   storeport.Store
	builder storeport.PlanBuilder
}

// WithRetrySchedules selects business scheduling explicitly. An invalid
// selected binding fails closed, rather than falling back to old Outbox Stage.
func WithRetrySchedules(store storeport.Store, builder storeport.PlanBuilder) GovernedRetryOption {
	return func(s *governedRetryService) { s.future = &futureRetrySchedules{store: store, builder: builder} }
}

func (s *governedRetryService) authorizeScheduled(ctx context.Context, a *assessment.Assessment, command GovernedRetryCommand) (*evalrun.EvaluationRun, error) {
	if s.future.store == nil || s.future.builder == nil {
		return nil, transaction.ErrBinding
	}
	if (command.Origin != retrygovernance.AttemptOriginManual && command.Origin != retrygovernance.AttemptOriginForce) ||
		(command.Origin == retrygovernance.AttemptOriginManual && command.AuthorizationAction != "retry") ||
		(command.Origin == retrygovernance.AttemptOriginForce && command.AuthorizationAction != "force_retry") {
		return nil, fmt.Errorf("retry origin and authorized action mismatch")
	}
	current, err := s.runs.FindLatestByAssessmentID(ctx, command.AssessmentID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Attempt().Status != evalrun.StatusFailed || current.RetryDecision() == nil {
		return nil, ErrRetryStateConflict
	}
	if command.ExpectedAttempt == 0 {
		command.ExpectedAttempt = current.Attempt().Number
	}
	if command.ExpectedAttempt != current.Attempt().Number {
		return nil, ErrRetryStateConflict
	}
	at := s.now().UTC()
	evt := assessment.NewEvaluationRetryRequestedEvent(a, command.ExpectedAttempt, command.Origin, command.RequestID, at)
	d := current.RetryDecision()
	expected := retrygovernance.DispositionManualRequired
	if command.Origin == retrygovernance.AttemptOriginForce {
		expected = retrygovernance.DispositionTerminal
	}
	duplicate := d.Disposition == retrygovernance.DispositionAutomatic && d.ActionRequestID == command.RequestID && d.RetryEventID == evt.EventID()
	if d.Disposition != expected && !duplicate {
		return nil, ErrRetryStateConflict
	}
	d.Disposition = retrygovernance.DispositionAutomatic
	d.NextAttemptAt = &at
	d.RetryEventID = evt.EventID()
	d.ActionRequestID = command.RequestID
	frozen, err := s.future.builder.Freeze(model.Authority{Owner: "qs.mysql", OrgID: a.OrgID(), Kind: model.Evaluation, SubjectID: a.ID().String(), RunID: current.ID().String(), AssessmentID: command.AssessmentID, TesteeID: a.TesteeID().Uint64(), ExpectedAttempt: command.ExpectedAttempt, Origin: command.Origin, RequestID: command.RequestID}, evt, d, command)
	if err != nil {
		return nil, err
	}
	if _, err := s.future.store.Find(ctx, frozen); err == nil {
		return s.recoverScheduled(ctx, frozen)
	} else if !errors.Is(err, transaction.ErrNotFound) {
		return nil, errors.Join(events.ErrUnresolved, err)
	}
	if duplicate {
		return nil, events.ErrUnresolved
	} // A decision without its exact plan is not permission to recreate it.
	authorizer, ok := s.runs.(evaluationrun.RetryAuthorizer)
	if !ok {
		return nil, transaction.ErrBinding
	}
	rootErr := s.tx.WithinTransaction(ctx, func(root context.Context) error {
		// Schedule first, then original Run: consistent with due dispatch's lock order.
		winner, err := s.future.store.SaveFrozen(root, frozen)
		if err != nil {
			return err
		}
		actual, err := authorizer.AuthorizeRetry(root, evaluationrun.RetryAuthorizationRequest{AssessmentID: command.AssessmentID, ExpectedAttempt: command.ExpectedAttempt, Origin: command.Origin, RequestID: command.RequestID, EventID: winner.Message().Input().ID, AuthorizedAt: winner.DueAt()})
		if err != nil {
			return err
		}
		if actual == nil || actual.ID().String() != winner.Authority().RunID || winner.MatchDecision(actual.RetryDecision(), winner.DueAt()) != nil {
			return model.ErrConflict
		}
		return nil
	})
	// Even a root returning nil is resolved from fresh original facts. A lost
	// reply never generates a second authorization, message or business attempt.
	result, recoverErr := s.recoverScheduled(ctx, frozen)
	if recoverErr != nil {
		return nil, errors.Join(events.ErrUnresolved, rootErr, recoverErr)
	}
	return result, nil
}

func (s *governedRetryService) recoverScheduled(ctx context.Context, candidate model.Frozen) (*evalrun.EvaluationRun, error) {
	read, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	record, err := s.future.store.Find(read, candidate)
	if err != nil {
		return nil, err
	}
	if record == nil || record.Validate() != nil || record.State == model.Canceled {
		return nil, events.ErrUnresolved
	}
	current, err := s.runs.FindLatestByAssessmentID(read, candidate.Authority().AssessmentID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ID().String() != record.Plan.Authority().RunID || current.Attempt().Status != evalrun.StatusFailed || record.Plan.MatchDecision(current.RetryDecision(), record.Plan.BusinessDue()) != nil {
		return nil, events.ErrUnresolved
	}
	return current, nil
}
