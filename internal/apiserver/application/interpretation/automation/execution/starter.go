// Package generation owns the application use case that reliably claims one
// InterpretationRun for a ReportGeneration.
package generation

import (
	"context"
	"errors"
	"fmt"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type StartStatus string

const (
	StartStatusStarted    StartStatus = "started"
	StartStatusProcessing StartStatus = "processing"
	StartStatusGenerated  StartStatus = "generated"
)

type StartRequest struct {
	Key     domaingeneration.Key
	TraceID string
}

type StartResult struct {
	Status          StartStatus
	Generation      *domaingeneration.ReportGeneration
	Run             *interpretationrun.InterpretationRun
	InterpretReport *domainreport.InterpretReport
}

type Starter interface {
	Start(ctx context.Context, request StartRequest) (*StartResult, error)
}

type starter struct {
	txRunner      apptransaction.Runner
	generations   domaingeneration.Repository
	runs          interpretationrun.Repository
	reports       domainreport.ReportRepository
	leaseDuration time.Duration
	now           func() time.Time
	newID         func() meta.ID
}

func NewStarter(
	txRunner apptransaction.Runner,
	generations domaingeneration.Repository,
	runs interpretationrun.Repository,
	reports domainreport.ReportRepository,
	leaseDuration time.Duration,
) (Starter, error) {
	if txRunner == nil || generations == nil || runs == nil || reports == nil {
		return nil, fmt.Errorf("generation starter requires transaction, generation, run and artifact repositories")
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("generation starter lease duration must be positive")
	}
	return &starter{
		txRunner:      txRunner,
		generations:   generations,
		runs:          runs,
		reports:       reports,
		leaseDuration: leaseDuration,
		now:           time.Now,
		newID:         meta.New,
	}, nil
}

func (s *starter) Start(ctx context.Context, request StartRequest) (*StartResult, error) {
	if s == nil || s.txRunner == nil || s.generations == nil || s.runs == nil || s.reports == nil {
		return nil, fmt.Errorf("generation starter is not configured")
	}
	if err := request.Key.Validate(); err != nil {
		return nil, err
	}

	// A duplicate-key insert or CAS conflict means another worker won a claim.
	// Re-read once and return its generated/processing state instead of issuing a
	// second attempt.
	for claim := 0; claim < 2; claim++ {
		generationRecord, err := s.generations.FindByKey(ctx, request.Key)
		if errors.Is(err, domaingeneration.ErrNotFound) {
			result, claimErr := s.createAndStart(ctx, request)
			if isClaimConflict(claimErr) {
				continue
			}
			return result, claimErr
		}
		if err != nil {
			return nil, err
		}
		result, claimErr := s.startExisting(ctx, generationRecord, request)
		if isClaimConflict(claimErr) {
			continue
		}
		return result, claimErr
	}
	return nil, fmt.Errorf("report generation claim conflicted repeatedly")
}

func (s *starter) createAndStart(ctx context.Context, request StartRequest) (*StartResult, error) {
	at := s.now()
	generationRecord, err := domaingeneration.New(s.newID(), request.Key, at)
	if err != nil {
		return nil, err
	}
	runRecord, err := interpretationrun.NewPending(s.newID(), generationRecord.ID(), 1)
	if err != nil {
		return nil, err
	}
	if err := runRecord.StartWithLease(at, request.TraceID, at.Add(s.leaseDuration)); err != nil {
		return nil, err
	}
	if err := generationRecord.Begin(runRecord.ID(), at); err != nil {
		return nil, err
	}
	if err := s.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.generations.Create(txCtx, generationRecord); err != nil {
			return err
		}
		return s.runs.Create(txCtx, runRecord)
	}); err != nil {
		return nil, err
	}
	return &StartResult{Status: StartStatusStarted, Generation: generationRecord, Run: runRecord}, nil
}

func (s *starter) startExisting(ctx context.Context, generationRecord *domaingeneration.ReportGeneration, request StartRequest) (*StartResult, error) {
	if generationRecord == nil {
		return nil, fmt.Errorf("report generation is required")
	}
	switch generationRecord.Status() {
	case domaingeneration.StatusGenerated:
		artifact, err := s.reports.FindByID(ctx, generationRecord.ReportID())
		if err != nil {
			return nil, fmt.Errorf("load generated interpretation report: %w", err)
		}
		return &StartResult{Status: StartStatusGenerated, Generation: generationRecord, InterpretReport: artifact}, nil
	case domaingeneration.StatusGenerating:
		return s.resumeOrRecover(ctx, generationRecord, request)
	case domaingeneration.StatusPending, domaingeneration.StatusFailed:
		return s.startNext(ctx, generationRecord, nil, request)
	default:
		return nil, fmt.Errorf("unsupported report generation status %s", generationRecord.Status())
	}
}

func (s *starter) resumeOrRecover(ctx context.Context, generationRecord *domaingeneration.ReportGeneration, request StartRequest) (*StartResult, error) {
	latest, err := s.runs.FindByID(ctx, generationRecord.LatestRunID())
	if err != nil {
		return nil, fmt.Errorf("load generating interpretation run: %w", err)
	}
	at := s.now()
	if latest.HasActiveLease(at) {
		return &StartResult{Status: StartStatusProcessing, Generation: generationRecord, Run: latest}, nil
	}
	if latest.Status() != interpretationrun.StatusRunning {
		return nil, fmt.Errorf("generating report generation has non-running run %s", latest.Status())
	}
	failure := interpretationrun.Failure{
		Kind:        interpretationrun.FailureKindTimeout,
		Code:        "lease_expired",
		SafeMessage: "报告生成任务超时，已重新调度",
		Retryable:   true,
	}
	if err := latest.Fail(at, failure); err != nil {
		return nil, err
	}
	if err := generationRecord.Fail(latest.ID(), at); err != nil {
		return nil, err
	}
	return s.startNext(ctx, generationRecord, latest, request)
}

// startNext persists the currently running Generation and a new running Run in
// one Mongo transaction. staleRun, when present, has already transitioned to
// failed and is persisted in the same transaction before the new attempt.
func (s *starter) startNext(ctx context.Context, generationRecord *domaingeneration.ReportGeneration, staleRun *interpretationrun.InterpretationRun, request StartRequest) (*StartResult, error) {
	expectedVersion := generationRecord.Version()
	if staleRun != nil {
		// Fail followed by Begin advances the Generation twice. CAS protects the
		// original version while storing the final generating state atomically.
		expectedVersion -= 1
	}
	var runRecord *interpretationrun.InterpretationRun
	var err error
	if staleRun != nil {
		runRecord, err = interpretationrun.Next(s.newID(), staleRun)
	} else if generationRecord.Status() == domaingeneration.StatusPending {
		runRecord, err = interpretationrun.NewPending(s.newID(), generationRecord.ID(), 1)
	} else {
		latest, findErr := s.runs.FindLatestByGenerationID(ctx, generationRecord.ID())
		if findErr != nil {
			return nil, fmt.Errorf("load latest interpretation run: %w", findErr)
		}
		runRecord, err = interpretationrun.Next(s.newID(), latest)
	}
	if err != nil {
		return nil, err
	}
	at := s.now()
	if err := runRecord.StartWithLease(at, request.TraceID, at.Add(s.leaseDuration)); err != nil {
		return nil, err
	}
	if err := generationRecord.Begin(runRecord.ID(), at); err != nil {
		return nil, err
	}
	if err := s.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if staleRun != nil {
			if err := s.runs.Save(txCtx, staleRun); err != nil {
				return err
			}
		}
		if err := s.runs.Create(txCtx, runRecord); err != nil {
			return err
		}
		return s.generations.Save(txCtx, generationRecord, expectedVersion)
	}); err != nil {
		return nil, err
	}
	return &StartResult{Status: StartStatusStarted, Generation: generationRecord, Run: runRecord}, nil
}

func isClaimConflict(err error) bool {
	return errors.Is(err, domaingeneration.ErrAlreadyExists) || errors.Is(err, domaingeneration.ErrVersionConflict)
}
