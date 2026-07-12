package generation

import (
	"context"
	"fmt"
	"time"

	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/footprintevent"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpolicy"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EventStager stores terminal Interpretation events in the transaction that
// commits the corresponding Generation state.
type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

type ReportCatalogProjector interface {
	ProjectCurrent(context.Context, *domainreport.InterpretReport) error
}

// InterpretationCommitter is the only terminal persistence boundary for one
// InterpretationRun. It commits immutable InterpretReport, Generation/Run state and
// durable outbox events atomically.
type InterpretationCommitter interface {
	CommitSuccess(ctx context.Context, request CommitSuccessRequest) (*CommitResult, error)
	CommitFailure(ctx context.Context, request CommitFailureRequest) (*CommitResult, error)
}

type CommitSuccessRequest struct {
	Generation           *domaingeneration.ReportGeneration
	Run                  *interpretationrun.InterpretationRun
	InterpretReport      *domainreport.InterpretReport
	BuilderIdentity      string
	ContentSchemaVersion string
	CompletedAt          time.Time
}

type CommitFailureRequest struct {
	Generation  *domaingeneration.ReportGeneration
	Run         *interpretationrun.InterpretationRun
	OutcomeID   domaingeneration.ID
	Association domainreport.Association
	Failure     interpretationrun.Failure
	FailedAt    time.Time
}

type CommitResult struct {
	Generation      *domaingeneration.ReportGeneration
	Run             *interpretationrun.InterpretationRun
	InterpretReport *domainreport.InterpretReport
}

type interpretationCommitter struct {
	txRunner     apptransaction.Runner
	generations  domaingeneration.Repository
	runs         interpretationrun.Repository
	reports      domainreport.ReportRepository
	stager       EventStager
	readyIndexer *appEventing.PostCommitReadyIndexer
	catalog      ReportCatalogProjector
}

func NewInterpretationCommitter(
	txRunner apptransaction.Runner,
	generations domaingeneration.Repository,
	runs interpretationrun.Repository,
	reports domainreport.ReportRepository,
	stager EventStager,
	readyIndexer *appEventing.PostCommitReadyIndexer,
	catalog ReportCatalogProjector,
) (InterpretationCommitter, error) {
	if txRunner == nil || generations == nil || runs == nil || reports == nil || stager == nil || catalog == nil {
		return nil, fmt.Errorf("interpretation committer dependencies are required")
	}
	return &interpretationCommitter{
		txRunner: txRunner, generations: generations, runs: runs, reports: reports, stager: stager, readyIndexer: readyIndexer, catalog: catalog,
	}, nil
}

func (c *interpretationCommitter) CommitSuccess(ctx context.Context, request CommitSuccessRequest) (*CommitResult, error) {
	if err := c.validateSuccess(request); err != nil {
		return nil, err
	}
	completedAt := request.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now()
	}

	// Build the terminal state on copies. A failed transaction must leave the
	// caller-owned running records available for lease recovery and retry.
	generationToCommit, err := cloneGeneration(request.Generation)
	if err != nil {
		return nil, err
	}
	runToCommit, err := cloneRun(request.Run)
	if err != nil {
		return nil, err
	}
	expectedVersion := generationToCommit.Version()
	if err := runToCommit.Succeed(completedAt); err != nil {
		return nil, err
	}
	if err := generationToCommit.Succeed(runToCommit.ID(), request.InterpretReport.ID(), completedAt); err != nil {
		return nil, err
	}
	events := outboxpolicy.Filter(generatedEvents(request.InterpretReport, runToCommit.Attempt(), request.BuilderIdentity, request.ContentSchemaVersion))
	if err := c.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.reports.Insert(txCtx, request.InterpretReport); err != nil {
			return err
		}
		if err := c.catalog.ProjectCurrent(txCtx, request.InterpretReport); err != nil {
			return err
		}
		if err := c.runs.Save(txCtx, runToCommit); err != nil {
			return err
		}
		if err := c.generations.Save(txCtx, generationToCommit, expectedVersion); err != nil {
			return err
		}
		return c.stage(txCtx, events)
	}); err != nil {
		return nil, err
	}
	c.enqueueAfterCommit(ctx, events, completedAt)
	return &CommitResult{Generation: generationToCommit, Run: runToCommit, InterpretReport: request.InterpretReport}, nil
}

func (c *interpretationCommitter) CommitFailure(ctx context.Context, request CommitFailureRequest) (*CommitResult, error) {
	if err := c.validateFailure(request); err != nil {
		return nil, err
	}
	failedAt := request.FailedAt
	if failedAt.IsZero() {
		failedAt = time.Now()
	}
	generationToCommit, err := cloneGeneration(request.Generation)
	if err != nil {
		return nil, err
	}
	runToCommit, err := cloneRun(request.Run)
	if err != nil {
		return nil, err
	}
	expectedVersion := generationToCommit.Version()
	if err := runToCommit.Fail(failedAt, request.Failure); err != nil {
		return nil, err
	}
	if err := generationToCommit.Fail(runToCommit.ID(), failedAt); err != nil {
		return nil, err
	}
	key := generationToCommit.Key()
	events := outboxpolicy.Filter([]event.DomainEvent{domaininterpretation.NewInterpretationReportFailedEvent(domaininterpretation.ReportFailedEventInput{
		OrgID: request.Association.OrgID, GenerationID: generationToCommit.ID().String(), RunID: runToCommit.ID().String(),
		AssessmentID: request.Association.AssessmentID.String(), OutcomeID: request.OutcomeID.String(), TesteeID: request.Association.TesteeID,
		Attempt: uint(runToCommit.Attempt()), ReportType: key.ReportType.String(), TemplateVersion: key.TemplateVersion.String(),
		FailureKind: string(request.Failure.Kind), FailureCode: request.Failure.Code, Retryable: request.Failure.Retryable,
		SafeReason: request.Failure.SafeMessage, FailedAt: failedAt,
	})})
	if err := c.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.runs.Save(txCtx, runToCommit); err != nil {
			return err
		}
		if err := c.generations.Save(txCtx, generationToCommit, expectedVersion); err != nil {
			return err
		}
		return c.stage(txCtx, events)
	}); err != nil {
		return nil, err
	}
	c.enqueueAfterCommit(ctx, events, failedAt)
	return &CommitResult{Generation: generationToCommit, Run: runToCommit}, nil
}

func (c *interpretationCommitter) validateSuccess(request CommitSuccessRequest) error {
	if c == nil || c.txRunner == nil || c.generations == nil || c.runs == nil || c.reports == nil || c.stager == nil || c.catalog == nil {
		return fmt.Errorf("interpretation committer is not configured")
	}
	if request.Generation == nil || request.Run == nil || request.InterpretReport == nil {
		return fmt.Errorf("interpretation generation, run and artifact are required")
	}
	if request.BuilderIdentity == "" || request.ContentSchemaVersion == "" {
		return fmt.Errorf("interpretation builder identity and content schema version are required")
	}
	key := request.Generation.Key()
	if request.Generation.ID() != request.Run.GenerationID() ||
		request.Generation.LatestRunID() != request.Run.ID() ||
		request.InterpretReport.GenerationID() != request.Generation.ID() ||
		request.InterpretReport.InterpretationRunID() != request.Run.ID() ||
		request.InterpretReport.OutcomeID() != key.OutcomeID ||
		request.InterpretReport.ReportType() != key.ReportType ||
		request.InterpretReport.TemplateVersion() != key.TemplateVersion {
		return fmt.Errorf("interpretation commit references do not match")
	}
	return nil
}

func (c *interpretationCommitter) validateFailure(request CommitFailureRequest) error {
	if c == nil || c.txRunner == nil || c.generations == nil || c.runs == nil || c.reports == nil || c.stager == nil {
		return fmt.Errorf("interpretation committer is not configured")
	}
	if request.Generation == nil || request.Run == nil || request.OutcomeID.IsZero() {
		return fmt.Errorf("interpretation generation, run and outcome are required")
	}
	if request.Generation.ID() != request.Run.GenerationID() ||
		request.Generation.LatestRunID() != request.Run.ID() ||
		request.OutcomeID != request.Generation.Key().OutcomeID {
		return fmt.Errorf("interpretation failure commit references do not match")
	}
	return request.Failure.Validate()
}

func (c *interpretationCommitter) stage(ctx context.Context, events []event.DomainEvent) error {
	if len(events) == 0 {
		return nil
	}
	return c.stager.Stage(ctx, events...)
}

func (c *interpretationCommitter) enqueueAfterCommit(ctx context.Context, events []event.DomainEvent, at time.Time) {
	if c.readyIndexer != nil && len(events) > 0 {
		c.readyIndexer.EnqueueAfterCommit(ctx, events, at)
	}
}

func cloneGeneration(source *domaingeneration.ReportGeneration) (*domaingeneration.ReportGeneration, error) {
	if source == nil {
		return nil, fmt.Errorf("report generation is required")
	}
	return domaingeneration.Restore(domaingeneration.RestoreInput{
		ID: source.ID(), Key: source.Key(), Status: source.Status(), LatestRunID: source.LatestRunID(), ReportID: source.ReportID(),
		Version: source.Version(), CreatedAt: source.CreatedAt(), UpdatedAt: source.UpdatedAt(),
	})
}

func cloneRun(source *interpretationrun.InterpretationRun) (*interpretationrun.InterpretationRun, error) {
	if source == nil {
		return nil, fmt.Errorf("interpretation run is required")
	}
	return interpretationrun.Restore(interpretationrun.RestoreInput{
		ID: source.ID(), GenerationID: source.GenerationID(), Attempt: source.Attempt(), Status: source.Status(), Failure: source.Failure(), TraceID: source.TraceID(),
		StartedAt: source.StartedAt(), LeaseExpiresAt: source.LeaseExpiresAt(), FinishedAt: source.FinishedAt(),
	})
}

func generatedEvents(artifact *domainreport.InterpretReport, attempt int, builderIdentity, contentSchemaVersion string) []event.DomainEvent {
	if artifact == nil {
		return nil
	}
	association, content := artifact.Association(), artifact.Content()
	generated := domaininterpretation.NewInterpretationReportGeneratedEvent(domaininterpretation.ReportGeneratedEventInput{
		OrgID: association.OrgID, GenerationID: artifact.GenerationID().String(), RunID: artifact.InterpretationRunID().String(), ReportID: artifact.ID().String(),
		AssessmentID: association.AssessmentID.String(), OutcomeID: artifact.OutcomeID().String(), TesteeID: association.TesteeID, Attempt: uint(attempt),
		ReportType: artifact.ReportType().String(), TemplateVersion: artifact.TemplateVersion().String(), BuilderIdentity: builderIdentity,
		ContentSchemaVersion: contentSchemaVersion, Model: domaininterpretation.EventModelIdentityFrom(content.Model),
		PrimaryScore: domaininterpretation.EventScoreValueFrom(content.PrimaryScore), Level: domaininterpretation.EventResultLevelFrom(content.Level), GeneratedAt: artifact.GeneratedAt(),
	})
	footprint := footprintevent.NewFootprintReportGeneratedEvent(association.OrgID, association.TesteeID, association.AssessmentID.Uint64(), artifact.ID().Uint64(), artifact.GeneratedAt())
	return []event.DomainEvent{generated, footprint}
}
