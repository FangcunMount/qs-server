// Package commit provides the single reliable commit boundary for Evaluation.
package commit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

type Request struct {
	Outcome     evaloutcome.Outcome
	Run         *evalrun.EvaluationRun
	EvaluatedAt time.Time
}

type Committer interface {
	Commit(ctx context.Context, request Request) (*domainoutcome.Record, error)
}

type committer struct {
	txRunner       apptransaction.Runner
	assessmentRepo assessment.Repository
	outcomeRepo    domainoutcome.Repository
	runRepo        evaluationrun.Repository
	scoreProjector outcomescoring.Projector
	eventStager    EventStager
	readyIndexer   *appEventing.PostCommitReadyIndexer
	newID          func() meta.ID
}

func NewCommitter(
	txRunner apptransaction.Runner,
	assessmentRepo assessment.Repository,
	outcomeRepo domainoutcome.Repository,
	runRepo evaluationrun.Repository,
	scoreProjector outcomescoring.Projector,
	eventStager EventStager,
	readyIndexer *appEventing.PostCommitReadyIndexer,
) Committer {
	return &committer{
		txRunner:       txRunner,
		assessmentRepo: assessmentRepo,
		outcomeRepo:    outcomeRepo,
		runRepo:        runRepo,
		scoreProjector: scoreProjector,
		eventStager:    eventStager,
		readyIndexer:   readyIndexer,
		newID:          meta.New,
	}
}

func (c *committer) Commit(ctx context.Context, request Request) (*domainoutcome.Record, error) {
	if err := c.validate(request); err != nil {
		return nil, err
	}
	if request.EvaluatedAt.IsZero() {
		request.EvaluatedAt = time.Now()
	}
	if err := request.Outcome.Assessment.ApplyScoringOutcome(request.Outcome.Execution); err != nil {
		return nil, evalerrors.AssessmentInterpretFailed(err, "应用计分结果失败")
	}
	payload, err := json.Marshal(request.Outcome.Execution)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical evaluation outcome: %w", err)
	}
	var reportInput json.RawMessage
	if request.Outcome.Input != nil && request.Outcome.Input.ModelPayload != nil {
		reportInput, err = json.Marshal(request.Outcome.Input.ModelPayload)
		if err != nil {
			return nil, fmt.Errorf("marshal evaluation report input: %w", err)
		}
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           c.newID(),
		OrgID:        request.Outcome.Assessment.OrgID(),
		AssessmentID: request.Outcome.Assessment.ID(),
		TesteeID:     request.Outcome.Assessment.TesteeID().Uint64(),
		RunID:        request.Run.RunID.String(),
		Model: domainoutcome.ModelIdentity{
			Kind:      request.Outcome.Execution.ModelRef.Kind(),
			SubKind:   request.Outcome.Execution.ModelRef.SubKind(),
			Algorithm: request.Outcome.Execution.ModelRef.Algorithm(),
			Code:      request.Outcome.Execution.ModelRef.Code().String(),
			Version:   request.Outcome.Execution.ModelRef.Version(),
			Title:     request.Outcome.Execution.ModelRef.Title(),
		},
		Runtime: domainoutcome.RuntimeIdentity{
			AlgorithmFamily: request.Outcome.RuntimeDescriptorKey.AlgorithmFamily,
			DecisionKind:    request.Outcome.RuntimeDescriptorKey.DecisionKind,
			PayloadFormat:   request.Outcome.RuntimeDescriptorKey.PayloadFormat,
		},
		InputSnapshotRef: request.Run.InputSnapshotRef,
		ReportInput:      reportInput,
		Payload:          payload,
		SchemaVersion:    domainoutcome.CurrentSchemaVersion,
		EvaluatedAt:      request.EvaluatedAt,
	})
	if err != nil {
		return nil, err
	}

	request.Run.Succeed(request.EvaluatedAt)
	request.Outcome.Assessment.StageEvaluatedEvent(request.EvaluatedAt, record.ID(), request.Run.RunID)
	eventsToStage := request.Outcome.Assessment.Events()
	err = c.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.outcomeRepo.Save(txCtx, record); err != nil {
			return err
		}
		if c.scoreProjector != nil {
			if err := c.scoreProjector.Project(txCtx, request.Outcome); err != nil {
				return err
			}
		}
		if err := c.assessmentRepo.Save(txCtx, request.Outcome.Assessment); err != nil {
			return err
		}
		if err := c.runRepo.Save(txCtx, *request.Run); err != nil {
			return err
		}
		if len(eventsToStage) > 0 {
			if err := c.eventStager.Stage(txCtx, eventsToStage...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		request.Outcome.Assessment.ClearEvents()
		return nil, err
	}
	request.Outcome.Assessment.ClearEvents()
	if c.readyIndexer != nil && len(eventsToStage) > 0 {
		c.readyIndexer.EnqueueAfterCommit(ctx, eventsToStage, request.EvaluatedAt)
	}
	return record, nil
}

func (c *committer) validate(request Request) error {
	if c == nil || c.txRunner == nil || c.assessmentRepo == nil || c.outcomeRepo == nil || c.runRepo == nil || c.eventStager == nil {
		return evalerrors.ModuleNotConfigured("evaluation committer requires transaction, assessment, outcome, run and outbox dependencies")
	}
	if request.Outcome.Assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if request.Outcome.Execution == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	if request.Run == nil || request.Run.RunID == "" {
		return fmt.Errorf("evaluation run is required")
	}
	if request.Run.AssessmentID != request.Outcome.Assessment.ID().Uint64() {
		return fmt.Errorf("evaluation run assessment does not match outcome assessment")
	}
	return nil
}
