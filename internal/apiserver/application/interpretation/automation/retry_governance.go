package automation

import (
	"context"
	"fmt"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	evaluationfact "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type GovernedRetryCommand struct {
	OrgID           int64
	GenerationID    meta.ID
	ExpectedAttempt int
	Origin          retrygovernance.AttemptOrigin
	RequestID       string
	Reason          string
}

type GovernedRetryService interface {
	Authorize(context.Context, GovernedRetryCommand) (*interpretationrun.InterpretationRun, error)
}

type governedRetryService struct {
	generations domaingeneration.Repository
	runs        interpretationrun.Repository
	outcomes    evaluationfact.Repository
	tx          apptransaction.Runner
	events      interface {
		Stage(context.Context, ...event.DomainEvent) error
	}
	now func() time.Time
}

func NewGovernedRetryService(generations domaingeneration.Repository, runs interpretationrun.Repository, outcomes evaluationfact.Repository, tx apptransaction.Runner, events interface {
	Stage(context.Context, ...event.DomainEvent) error
}) GovernedRetryService {
	return &governedRetryService{generations: generations, runs: runs, outcomes: outcomes, tx: tx, events: events, now: time.Now}
}

func (s *governedRetryService) Authorize(ctx context.Context, command GovernedRetryCommand) (*interpretationrun.InterpretationRun, error) {
	if s == nil || s.generations == nil || s.runs == nil || s.outcomes == nil || s.tx == nil || s.events == nil {
		return nil, fmt.Errorf("interpretation retry governance is not configured")
	}
	if command.OrgID == 0 || command.GenerationID.IsZero() || command.ExpectedAttempt < 1 || command.RequestID == "" || command.Reason == "" {
		return nil, fmt.Errorf("interpretation retry governance input is invalid")
	}
	preparer, ok := s.events.(outboxport.ReferencePreparer)
	if !ok {
		return nil, fmt.Errorf("interpretation retry requires original event reference preparation")
	}
	scheduled, ok := s.events.(outboxport.ScheduledStager)
	if !ok {
		return nil, fmt.Errorf("interpretation retry requires scheduled staging")
	}
	generationRecord, err := s.generations.FindByID(ctx, command.GenerationID)
	if err != nil {
		return nil, err
	}
	if generationRecord.Status() != domaingeneration.StatusFailed {
		return nil, fmt.Errorf("interpretation retry requires a failed generation")
	}
	outcome, err := s.outcomes.FindByID(ctx, generationRecord.Key().OutcomeID)
	if err != nil {
		return nil, err
	}
	if outcome == nil || outcome.OrgID() != command.OrgID {
		return nil, fmt.Errorf("interpretation retry organization mismatch")
	}
	at := s.now()
	retryEvent := domaininterpretation.NewInterpretationRetryRequestedEvent(domaininterpretation.RetryRequestedEventInput{
		OrgID: command.OrgID, GenerationID: generationRecord.ID().String(), RunID: generationRecord.LatestRunID().String(),
		AssessmentID: outcome.AssessmentID().String(), OutcomeID: outcome.ID().String(), TesteeID: outcome.TesteeID(),
		ExpectedAttempt: command.ExpectedAttempt, AttemptOrigin: string(command.Origin), ActionRequestID: command.RequestID,
		Mode: "next_attempt", RequestedAt: at,
	})
	binding, err := eventevidencebinding.Retry(retryEvent.Data)
	if err != nil {
		return nil, err
	}
	ref, err := preparer.PrepareReference(retryEvent)
	if err != nil {
		return nil, err
	}
	proof, err := evidence.NewStandard(ref, binding)
	if err != nil {
		return nil, err
	}
	authorizer, ok := s.runs.(interpretationrun.RetryAuthorizer)
	if !ok {
		return nil, fmt.Errorf("interpretation run repository does not support retry authorization")
	}
	var authorized *interpretationrun.InterpretationRun
	err = s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var authorizeErr error
		authorized, authorizeErr = authorizer.AuthorizeRetry(txCtx, interpretationrun.RetryAuthorizationRequest{
			GenerationID: command.GenerationID, ExpectedAttempt: command.ExpectedAttempt, Origin: command.Origin,
			RequestID: command.RequestID, EventID: retryEvent.EventID(), AuthorizedAt: at,
			EventEvidence: proof,
		})
		if authorizeErr != nil {
			return authorizeErr
		}
		return scheduled.StageAt(txCtx, at, retryEvent)
	})
	return authorized, err
}
