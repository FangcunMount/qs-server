// Package recovery owns operator-authorized participant AI explanation retry.
// It does not call the Provider; it durably authorizes and budgets one next
// attempt, then emits a privacy-minimal Worker wake-up.
package recovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	aiexplanationevents "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/events"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type Command struct {
	OrgID                   int64
	GenerationID            meta.ID
	ExpectedAttempt         int
	RequestID               string
	Actor                   string
	Reason                  string
	AcceptResultUnknownRisk bool
}

type Result struct {
	Generation    *domaingeneration.AIExplanationGeneration
	FailedRun     *domainrun.AIExplanationRun
	Authorization domainrun.RetryAuthorization
	Created       bool
}

type Service interface {
	Authorize(context.Context, Command) (*Result, error)
}

type service struct {
	generations domaingeneration.Repository
	runs        domainrun.Repository
	committer   appport.RetryAuthorizationCommitter
	now         func() time.Time
}

func NewService(
	generations domaingeneration.Repository,
	runs domainrun.Repository,
	committer appport.RetryAuthorizationCommitter,
	now func() time.Time,
) (Service, error) {
	if generations == nil || runs == nil || committer == nil {
		return nil, fmt.Errorf("AI explanation participant recovery dependencies are required")
	}
	if now == nil {
		now = time.Now
	}
	return &service{generations: generations, runs: runs, committer: committer, now: now}, nil
}

func (s *service) Authorize(ctx context.Context, command Command) (result *Result, err error) {
	defer func() { observeParticipantRetryAuthorization(result, err) }()
	command.RequestID = strings.TrimSpace(command.RequestID)
	command.Actor = strings.TrimSpace(command.Actor)
	command.Reason = strings.TrimSpace(command.Reason)
	if command.OrgID <= 0 || command.GenerationID.IsZero() || command.ExpectedAttempt < 1 ||
		command.RequestID == "" || len(command.RequestID) > 256 || command.Actor == "" || len(command.Actor) > 256 ||
		command.Reason == "" || len(command.Reason) > 1000 {
		return nil, fmt.Errorf("AI explanation participant recovery input is invalid")
	}
	generationRecord, err := s.generations.FindByID(ctx, command.GenerationID)
	if err != nil {
		return nil, err
	}
	if generationRecord.Status() != domaingeneration.StatusFailed {
		return nil, domainrun.ErrRetryNotAllowed
	}
	if generationRecord.Association().OrgID != command.OrgID {
		return nil, fmt.Errorf("AI explanation participant recovery organization mismatch")
	}
	latest, err := s.runs.FindLatestByGenerationID(ctx, generationRecord.ID())
	if err != nil {
		return nil, err
	}
	if latest.ID() != generationRecord.LatestRunID() || latest.Status() != domainrun.StatusFailed || latest.Attempt() != command.ExpectedAttempt {
		return nil, domainrun.ErrConflict
	}
	eventID := aiexplanationevents.RetryEventID(generationRecord.ID().String(), command.ExpectedAttempt, command.RequestID)
	authorization := domainrun.RetryAuthorization{
		ExpectedAttempt: command.ExpectedAttempt, NextAttempt: command.ExpectedAttempt + 1,
		Origin: retrygovernance.AttemptOriginManual, RequestID: command.RequestID, EventID: eventID,
		Actor: command.Actor, Reason: command.Reason, AcceptedResultUnknownRisk: command.AcceptResultUnknownRisk,
		AuthorizedAt: s.now().UTC(),
	}
	if existing := latest.RetryAuthorization(); existing != nil {
		if sameAuthorization(*existing, authorization) {
			return &Result{Generation: generationRecord, FailedRun: latest, Authorization: *existing}, nil
		}
		return nil, domainrun.ErrConflict
	}
	if err := latest.AuthorizeManualRetry(authorization); err != nil {
		return nil, err
	}
	authorized, created, err := s.committer.CommitRetryAuthorization(ctx, generationRecord, latest, authorization)
	if err != nil {
		return nil, err
	}
	if authorized == nil || authorized.GenerationID() != generationRecord.ID() || authorized.Attempt() != command.ExpectedAttempt {
		return nil, fmt.Errorf("AI explanation participant recovery returned an invalid authorization")
	}
	stored := authorized.RetryAuthorization()
	if stored == nil || !stored.SameAction(authorization) {
		return nil, fmt.Errorf("AI explanation participant recovery authorization was not persisted")
	}
	return &Result{Generation: generationRecord, FailedRun: authorized, Authorization: *stored, Created: created}, nil
}

func sameAuthorization(existing, requested domainrun.RetryAuthorization) bool {
	return existing.SameAction(requested)
}

var _ Service = (*service)(nil)
