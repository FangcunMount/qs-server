package readmission

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
)

type Command struct {
	OrgID                  int64
	FailureFingerprint     string
	ExpectedReason         admission.Kind
	ExpectedOutcomeVersion string
	Reason                 string
	RequestID              string
	OperatorUserID         int64
}

type Result struct {
	OutcomeID    uint64
	GenerationID uint64
	RunID        uint64
	ReportID     uint64
	Status       automation.Status
}

type Service interface {
	Readmit(context.Context, Command) (Result, error)
}

type service struct {
	failures   admission.Repository
	outcomes   evaluationfact.Repository
	automation automation.Service
}

func NewService(failures admission.Repository, outcomes evaluationfact.Repository, automationService automation.Service) Service {
	return &service{failures: failures, outcomes: outcomes, automation: automationService}
}

func (s *service) Readmit(ctx context.Context, command Command) (Result, error) {
	if s == nil || s.failures == nil || s.outcomes == nil || s.automation == nil {
		return Result{}, fmt.Errorf("interpretation readmission service is not configured")
	}
	if command.OrgID == 0 || command.OperatorUserID == 0 || command.FailureFingerprint == "" ||
		command.ExpectedReason == "" || command.ExpectedOutcomeVersion == "" || command.Reason == "" || command.RequestID == "" {
		return Result{}, fmt.Errorf("readmission governance identity, expected state, reason and request_id are required")
	}
	failure, err := s.failures.FindByFingerprint(ctx, command.FailureFingerprint)
	if err != nil {
		return Result{}, err
	}
	if failure.OrgID() != command.OrgID || failure.Kind() != command.ExpectedReason {
		return Result{}, fmt.Errorf("admission failure expected state changed")
	}
	outcome, err := s.outcomes.FindByID(ctx, failure.OutcomeID())
	if err != nil {
		return Result{}, err
	}
	if outcome == nil {
		return Result{}, evaluationfact.ErrNotFound
	}
	if outcome.OrgID() != command.OrgID || outcome.AssessmentID() != failure.AssessmentID() ||
		outcome.TesteeID() != failure.TesteeID() || outcome.VersionToken() != command.ExpectedOutcomeVersion ||
		(failure.OutcomeVersion() != "" && failure.OutcomeVersion() != command.ExpectedOutcomeVersion) {
		return Result{}, fmt.Errorf("committed outcome expected state changed")
	}
	generated, err := s.automation.Generate(ctx, automation.GenerateCommand{
		Actor:     automation.TrustedServiceActor("system-governance/readmit"),
		OutcomeID: outcome.ID(), TraceID: command.RequestID,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		OutcomeID: outcome.ID().Uint64(), GenerationID: generated.GenerationID.Uint64(),
		RunID: generated.RunID.Uint64(), ReportID: generated.ReportID.Uint64(), Status: generated.Status,
	}, nil
}
