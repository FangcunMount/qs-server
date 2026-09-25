package testee

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

// LatestRunReader is deliberately smaller than the execution repository. A
// participant status read must not acquire or change a run claim.
type LatestRunReader interface {
	FindLatestByAssessmentID(context.Context, uint64) (*evalrun.EvaluationRun, error)
}

// RuntimeStatus contains only the latest persisted attempt identity and phase.
// It does not expose failure diagnostics, claim tokens or model inputs.
type RuntimeStatus struct {
	Attempt int
	Status  evalrun.Status
}

type RuntimeStatusReader struct {
	access Service
	runs   LatestRunReader
}

func NewRuntimeStatusReader(access Service, runs LatestRunReader) *RuntimeStatusReader {
	return &RuntimeStatusReader{access: access, runs: runs}
}

// Get authorizes the participant before a fresh MySQL run read. The assessment
// detail cache is intentionally not used: its status can describe an older
// attempt while an authorized retry has already been persisted.
func (r *RuntimeStatusReader) Get(ctx context.Context, actor Actor, assessmentID uint64) (*RuntimeStatus, error) {
	if r == nil || r.access == nil || r.runs == nil {
		return nil, evalerrors.ModuleNotConfigured("runtime status reader is not configured")
	}
	if err := r.access.AuthorizeAssessment(ctx, actor, assessmentID); err != nil {
		return nil, err
	}
	run, err := r.runs.FindLatestByAssessmentID(ctx, assessmentID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, nil
	}
	return &RuntimeStatus{Attempt: run.Attempt().Number, Status: run.Attempt().Status}, nil
}
