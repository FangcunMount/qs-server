package participant

import (
	"context"
	"errors"
	"fmt"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	frozeninput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// RuntimeStatus is a sanitized read of the current standard report attempt.
// It contains no failure diagnostics, model inputs, claim or retry authority.
type RuntimeStatus struct {
	Status           string
	Attempt          int
	RetryDisposition retrygovernance.Disposition
}

type RuntimeQuery interface {
	Get(context.Context, Actor, uint64) (*RuntimeStatus, error)
}

type GenerationStatusReader interface {
	FindByKey(context.Context, generation.Key) (*generation.ReportGeneration, error)
	FindByID(context.Context, generation.ID) (*generation.ReportGeneration, error)
}

type RunStatusReader interface {
	FindByID(context.Context, interpretationrun.ID) (*interpretationrun.InterpretationRun, error)
}

// RuntimeStatusReader has only read ports: asking for status cannot claim a
// run, create a new generation, publish a retry or invoke a report builder.
type RuntimeStatusReader struct {
	access      Access
	outcomes    evaluationfact.Repository
	generations GenerationStatusReader
	runs        RunStatusReader
}

func NewRuntimeStatusReader(access Access, outcomes evaluationfact.Repository, generations GenerationStatusReader, runs RunStatusReader) *RuntimeStatusReader {
	return &RuntimeStatusReader{access: access, outcomes: outcomes, generations: generations, runs: runs}
}

func (r *RuntimeStatusReader) Get(ctx context.Context, actor Actor, assessmentID uint64) (*RuntimeStatus, error) {
	if actor.TesteeID == 0 || assessmentID == 0 {
		return nil, cberrors.WithCode(code.ErrInvalidArgument, "testee ID and assessment ID are required")
	}
	if r == nil || r.access == nil || r.outcomes == nil || r.generations == nil || r.runs == nil {
		return nil, cberrors.WithCode(code.ErrModuleInitializationFailed, "participant report runtime reader is not configured")
	}
	if err := r.access.AuthorizeOwnAssessment(ctx, actor.TesteeID, assessmentID); err != nil {
		return nil, err
	}
	// MySQL outcome and Mongo lifecycle are separate stores. Recheck the
	// immutable outcome identity and Generation version before returning a
	// terminal result; a retry that changes the latest Run wins the next read.
	for attempt := 0; attempt < 2; attempt++ {
		fact, err := r.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
		if errors.Is(err, evaluationfact.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if fact == nil {
			return nil, nil
		}
		if fact.ID().IsZero() || fact.AssessmentID().Uint64() != assessmentID || fact.TesteeID() != actor.TesteeID || fact.OrgID() <= 0 {
			return nil, fmt.Errorf("report runtime outcome association mismatch")
		}
		// Reuse the execution path's pure frozen-input decoder. Never resolve
		// a current model/template or choose a different historical generation.
		input, err := frozeninput.FromOutcomeRecord(fact)
		if err != nil {
			return nil, fmt.Errorf("report runtime frozen input is invalid")
		}
		key := generation.Key{OutcomeID: fact.ID(), ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion}
		current, err := r.generations.FindByKey(ctx, key)
		if errors.Is(err, generation.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, nil
		}
		if current.Key() != key || current.ID().IsZero() {
			return nil, fmt.Errorf("report runtime generation association mismatch")
		}
		result := &RuntimeStatus{Status: "pending"}
		var latest *interpretationrun.InterpretationRun
		if !current.LatestRunID().IsZero() {
			run, err := r.runs.FindByID(ctx, current.LatestRunID())
			if err != nil {
				return nil, err
			}
			if run == nil || run.ID() != current.LatestRunID() || run.GenerationID() != current.ID() {
				return nil, fmt.Errorf("report runtime latest run association mismatch")
			}
			latest = run
			result.Status, result.Attempt = string(run.Status()), run.Attempt()
			if run.Status() == interpretationrun.StatusFailed {
				if decision := run.RetryDecision(); decision != nil {
					if !decision.Disposition.IsValid() {
						return nil, fmt.Errorf("report runtime retry disposition is invalid")
					}
					result.RetryDisposition = decision.Disposition
				} else if failure := run.Failure(); failure != nil && failure.Retryable {
					result.RetryDisposition = retrygovernance.DispositionAutomatic
				} else {
					result.RetryDisposition = retrygovernance.DispositionTerminal
				}
			}
		}
		fresh, err := r.generations.FindByID(ctx, current.ID())
		if err != nil {
			return nil, err
		}
		freshFact, err := r.outcomes.FindByAssessmentID(ctx, meta.FromUint64(assessmentID))
		if err != nil {
			return nil, err
		}
		if fresh == nil || freshFact == nil {
			return nil, fmt.Errorf("report runtime source disappeared")
		}
		if freshFact.ID() != fact.ID() || freshFact.TesteeID() != actor.TesteeID || freshFact.AssessmentID().Uint64() != assessmentID || freshFact.OrgID() != fact.OrgID() || fresh.ID() != current.ID() || freshFact.VersionToken() != fact.VersionToken() || fresh.Version() != current.Version() || fresh.LatestRunID() != current.LatestRunID() || fresh.Key() != key || fresh.Status() != current.Status() {
			continue
		}
		if latest != nil {
			if (current.Status() == generation.StatusFailed && latest.Status() != interpretationrun.StatusFailed) ||
				(current.Status() == generation.StatusGenerated && latest.Status() != interpretationrun.StatusSucceeded) ||
				(current.Status() == generation.StatusGenerating && latest.Status() != interpretationrun.StatusRunning && latest.Status() != interpretationrun.StatusPending) {
				return nil, fmt.Errorf("report runtime lifecycle state mismatch")
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("report runtime changed during bounded read")
}
