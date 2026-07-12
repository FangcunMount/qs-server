// Package automation owns the trusted-system use case that generates or
// retries an Interpretation report from a persisted EvaluationOutcome.
package automation

import (
	"context"
	"fmt"

	interpretationgeneration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Actor struct {
	Source string
}

func TrustedServiceActor(source string) Actor { return Actor{Source: source} }

type GenerateCommand struct {
	Actor     Actor
	OutcomeID meta.ID
	TraceID   string
}

type Status string

const (
	StatusGenerated  Status = "generated"
	StatusProcessing Status = "processing"
)

type Result struct {
	Status       Status
	GenerationID meta.ID
	RunID        meta.ID
	ReportID     meta.ID
}

type Service interface {
	Generate(ctx context.Context, command GenerateCommand) (*Result, error)
}

type service struct {
	outcomes evaluationfact.Repository
	executor interpretationgeneration.Executor
}

func NewService(outcomes evaluationfact.Repository, executor interpretationgeneration.Executor) (Service, error) {
	if outcomes == nil || executor == nil {
		return nil, fmt.Errorf("interpretation automation dependencies are required")
	}
	return &service{outcomes: outcomes, executor: executor}, nil
}

func (s *service) Generate(ctx context.Context, command GenerateCommand) (*Result, error) {
	if command.Actor.Source == "" {
		return nil, fmt.Errorf("trusted automation actor is required")
	}
	if command.OutcomeID.IsZero() {
		return nil, fmt.Errorf("evaluation outcome id is required")
	}
	record, err := s.outcomes.FindByID(ctx, command.OutcomeID)
	if err != nil {
		return nil, err
	}
	input, err := interpretationinput.FromOutcomeRecord(record)
	if err != nil {
		return nil, err
	}
	executed, err := interpretationgeneration.ExecuteOutcome(ctx, s.executor, record, input, command.TraceID)
	if err != nil {
		return nil, err
	}
	result := &Result{}
	if executed.Status == interpretationgeneration.ExecuteStatusProcessing {
		result.Status = StatusProcessing
	} else {
		result.Status = StatusGenerated
	}
	if executed.Generation != nil {
		result.GenerationID = executed.Generation.ID()
	}
	if executed.Run != nil {
		result.RunID = executed.Run.ID()
	}
	if executed.InterpretReport != nil {
		result.ReportID = executed.InterpretReport.ID()
		if result.RunID.IsZero() {
			result.RunID = executed.InterpretReport.InterpretationRunID()
		}
	}
	return result, nil
}

type Failure struct {
	GenerationID meta.ID
	RunID        meta.ID
	Kind         run.FailureKind
	Code         string
	SafeMessage  string
	Retryable    bool
}

func FailureFrom(err error) (Failure, bool) {
	failed, ok := interpretationgeneration.FailureFrom(err)
	if !ok {
		return Failure{}, false
	}
	return Failure{
		GenerationID: failed.GenerationID,
		RunID:        failed.RunID,
		Kind:         failed.Failure.Kind,
		Code:         failed.Failure.Code,
		SafeMessage:  failed.Failure.SafeMessage,
		Retryable:    failed.Failure.Retryable,
	}, true
}
