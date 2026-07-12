// Package worker contains the application use case for one scoring-worker
// attempt. It owns the complete worker receipt, including ACK/NACK metadata.
package worker

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Command struct{ AssessmentID uint64 }

type Outcome struct {
	ID         string
	ModelKind  string
	SubKind    string
	Algorithm  string
	ModelCode  string
	Version    string
	Title      string
	TotalScore *float64
	RiskLevel  string
}

type Result struct {
	Status           string
	Outcome          *Outcome
	RunID            string
	Retryable        bool
	FailureKind      string
	FailureMessage   string
	TraceID          string
	InputSnapshotRef string
}

func (r Result) ShouldRetry() bool { return r.Status == "failed" && r.Retryable }

type Engine interface {
	Evaluate(context.Context, uint64) error
}

type Service interface {
	Execute(context.Context, Command) (*Result, error)
}

type service struct {
	engine      Engine
	assessments domainassessment.Repository
	outcomes    domainoutcome.Repository
	runs        evaluationrun.Repository
}

func NewService(engine Engine, assessments domainassessment.Repository, outcomes domainoutcome.Repository, runs evaluationrun.Repository) Service {
	return &service{engine: engine, assessments: assessments, outcomes: outcomes, runs: runs}
}

func (s *service) Execute(ctx context.Context, command Command) (*Result, error) {
	if command.AssessmentID == 0 {
		return nil, evalerrors.InvalidArgument("assessment id is required")
	}
	if s.engine == nil || s.assessments == nil || s.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation worker is not configured")
	}
	executionErr := s.engine.Evaluate(ctx, command.AssessmentID)
	result, readErr := s.readReceipt(ctx, command.AssessmentID)
	if readErr != nil {
		if executionErr != nil {
			return nil, executionErr
		}
		return nil, readErr
	}
	if executionErr != nil {
		if result.Status == "" {
			result.Status = "failed"
		}
		if result.FailureMessage == "" {
			result.FailureMessage = executionErr.Error()
		}
	}
	return result, nil
}

func (s *service) readReceipt(ctx context.Context, assessmentID uint64) (*Result, error) {
	a, err := s.assessments.FindByID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	result := &Result{Status: a.Status().String()}
	latest, err := s.runs.FindLatestByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		result.RunID = latest.RunID.String()
		result.TraceID = latest.TraceID
		result.InputSnapshotRef = latest.InputSnapshotRef
		result.Retryable = latest.Retryable()
		if latest.Failure != nil {
			result.FailureKind = latest.Failure.Kind.String()
			result.FailureMessage = latest.Failure.Message
		}
	}
	if !a.Status().IsEvaluated() {
		return result, nil
	}
	out := &Outcome{TotalScore: a.TotalScore()}
	if risk := a.RiskLevel(); risk != nil {
		out.RiskLevel = risk.String()
	}
	if ref := a.EvaluationModelRef(); ref != nil {
		out.ModelKind = string(ref.Kind())
		out.SubKind = string(ref.SubKind())
		out.Algorithm = string(ref.Algorithm())
		out.ModelCode = ref.Code().String()
		out.Version = ref.Version()
		out.Title = ref.Title()
	}
	if s.outcomes != nil {
		record, findErr := s.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
		if findErr != nil {
			return nil, findErr
		}
		if record != nil {
			out.ID = record.ID().String()
		}
	}
	result.Outcome = out
	return result, nil
}
