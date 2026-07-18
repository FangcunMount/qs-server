package operator

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	evaluationrun "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type GovernedRetryCommand struct {
	AssessmentID    uint64
	ExpectedAttempt int
	Origin          retrygovernance.AttemptOrigin
	RequestID       string
	Reason          string
}

type GovernedRetryService interface {
	Authorize(context.Context, Actor, GovernedRetryCommand) (*evalrun.EvaluationRun, error)
	Latest(context.Context, uint64) (*evalrun.EvaluationRun, error)
}

func (s *governedRetryService) Latest(ctx context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error) {
	if s == nil || s.runs == nil || assessmentID == 0 {
		return nil, fmt.Errorf("evaluation retry governance is not configured")
	}
	return s.runs.FindLatestByAssessmentID(ctx, assessmentID)
}

type governedRetryService struct {
	assessments domainassessment.Repository
	runs        evaluationrun.Repository
	tx          apptransaction.Runner
	events      EventStager
	authorizer  authorizer
	now         func() time.Time
}

func NewGovernedRetryService(assessments domainassessment.Repository, runs evaluationrun.Repository, tx apptransaction.Runner, events EventStager, access AccessChecker) GovernedRetryService {
	return &governedRetryService{assessments: assessments, runs: runs, tx: tx, events: events, authorizer: authorizer{assessments: assessments, access: access}, now: time.Now}
}

func (s *governedRetryService) Authorize(ctx context.Context, actor Actor, command GovernedRetryCommand) (*evalrun.EvaluationRun, error) {
	if s == nil || s.assessments == nil || s.runs == nil || s.tx == nil || s.events == nil {
		return nil, fmt.Errorf("evaluation retry governance is not configured")
	}
	if command.AssessmentID == 0 || command.ExpectedAttempt < 1 || command.RequestID == "" || command.Reason == "" {
		return nil, fmt.Errorf("evaluation retry governance input is invalid")
	}
	assessmentRecord, err := s.authorizer.loadAssessment(ctx, actor, command.AssessmentID)
	if err != nil {
		return nil, err
	}
	if !assessmentRecord.Status().IsFailed() {
		return nil, fmt.Errorf("evaluation retry requires a failed assessment")
	}
	at := s.now()
	retryEvent := domainassessment.NewEvaluationRetryRequestedEvent(assessmentRecord, command.ExpectedAttempt, command.Origin, command.RequestID, at)
	authorizer, ok := s.runs.(evaluationrun.RetryAuthorizer)
	if !ok {
		return nil, fmt.Errorf("evaluation run repository does not support retry authorization")
	}
	var authorized *evalrun.EvaluationRun
	err = s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var authorizeErr error
		authorized, authorizeErr = authorizer.AuthorizeRetry(txCtx, evaluationrun.RetryAuthorizationRequest{
			AssessmentID: command.AssessmentID, ExpectedAttempt: command.ExpectedAttempt, Origin: command.Origin,
			RequestID: command.RequestID, EventID: retryEvent.EventID(), AuthorizedAt: at,
		})
		if authorizeErr != nil {
			return authorizeErr
		}
		if scheduled, scheduledOK := s.events.(outboxport.ScheduledStager); scheduledOK {
			return scheduled.StageAt(txCtx, at, retryEvent)
		}
		return s.events.Stage(txCtx, event.DomainEvent(retryEvent))
	})
	return authorized, err
}
