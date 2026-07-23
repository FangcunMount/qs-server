// Package commit provides the single reliable commit boundary for Evaluation.
package commit

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// CommitRequest is the complete input to Evaluation's reliable success boundary.
// It deliberately distinguishes the Evaluator's in-memory Execution from the
// immutable Record created by Committer.
type CommitRequest struct {
	Assessment    *assessment.Assessment
	Input         *evaluationinput.InputSnapshot
	Execution     *domainoutcome.Execution
	DescriptorKey evalpipeline.DescriptorKey
	OutcomePolicy evalpipeline.OutcomeCompletenessPolicy
	Run           *evalrun.EvaluationRun
	EvaluatedAt   time.Time
}

type Committer interface {
	Commit(ctx context.Context, request CommitRequest) (*domainoutcome.Record, error)
}

type committer struct {
	txRunner       apptransaction.Runner
	assessmentRepo assessment.Repository
	outcomeRepo    domainoutcome.Repository
	runRepo        evaluationrun.Repository
	scoreProjector outcomescoring.Projector
	eventStager    EventStager
	postCommit     appEventing.PostCommitDispatcher
	newID          func() meta.ID
}

func NewCommitter(
	txRunner apptransaction.Runner,
	assessmentRepo assessment.Repository,
	outcomeRepo domainoutcome.Repository,
	runRepo evaluationrun.Repository,
	scoreProjector outcomescoring.Projector,
	eventStager EventStager,
	postCommit appEventing.PostCommitDispatcher,
) Committer {
	return &committer{
		txRunner:       txRunner,
		assessmentRepo: assessmentRepo,
		outcomeRepo:    outcomeRepo,
		runRepo:        runRepo,
		scoreProjector: scoreProjector,
		eventStager:    eventStager,
		postCommit:     postCommit,
		newID:          meta.New,
	}
}

func (c *committer) Commit(ctx context.Context, request CommitRequest) (*domainoutcome.Record, error) {
	if err := c.validate(request); err != nil {
		return nil, err
	}
	if request.EvaluatedAt.IsZero() {
		request.EvaluatedAt = time.Now()
	}
	// Prepare terminal state on isolated copies. A failed transaction must leave
	// the caller-owned submitted Assessment and running Run available for the
	// execution service's atomic failure finalizer.
	runToCommit := *request.Run
	assessmentToCommit, err := request.Assessment.PrepareScoringProjection(evaloutcome.ScoringProjectionFromExecution(request.Execution), request.EvaluatedAt)
	if err != nil {
		return nil, evalerrors.AssessmentScoringFailed(err, "应用计分结果失败")
	}
	payload, err := evaloutcome.MarshalRecordV2(request.Execution)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical evaluation outcome: %w", err)
	}
	modelRef := evaluationinput.ModelRef{
		Kind:      evaluationinput.EvaluationModelKind(request.Execution.ModelRef.Kind()),
		Algorithm: string(request.Execution.ModelRef.Algorithm()),
		Code:      request.Execution.ModelRef.Code().String(),
		Version:   request.Execution.ModelRef.Version(),
		Title:     request.Execution.ModelRef.Title(),
	}
	opts := evaluationinput.BuildFreezeOptionsFromSnapshot(request.Input, modelRef, request.DescriptorKey.DecisionKind)
	reportInput, err := evaluationinput.MarshalReportInput(opts)
	if err != nil {
		return nil, fmt.Errorf("marshal evaluation report input: %w", err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           c.newID(),
		OrgID:        assessmentToCommit.OrgID(),
		AssessmentID: assessmentToCommit.ID(),
		TesteeID:     assessmentToCommit.TesteeID().Uint64(),
		RunID:        runToCommit.ID().String(),
		Model: domainoutcome.ModelIdentity{
			Kind:      request.Execution.ModelRef.Kind(),
			Algorithm: request.Execution.ModelRef.Algorithm(),
			Code:      request.Execution.ModelRef.Code().String(),
			Version:   request.Execution.ModelRef.Version(),
			Title:     request.Execution.ModelRef.Title(),
		},
		Runtime: domainoutcome.RuntimeIdentity{
			DecisionKind: request.DescriptorKey.DecisionKind,
		},
		InputSnapshotRef: runToCommit.InputSnapshotRef(),
		ReportInput:      reportInput,
		Payload:          payload,
		SchemaVersion:    domainoutcome.CurrentSchemaVersion,
		EvaluatedAt:      request.EvaluatedAt,
	})
	if err != nil {
		return nil, err
	}

	if err := runToCommit.Succeed(request.EvaluatedAt); err != nil {
		return nil, err
	}
	assessmentToCommit.StageEvaluatedEvent(request.EvaluatedAt, record.ID(), runToCommit.ID())
	eventsToStage := assessmentToCommit.Events()
	err = c.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := c.outcomeRepo.Save(txCtx, record); err != nil {
			return err
		}
		if c.scoreProjector != nil {
			if err := c.scoreProjector.Project(txCtx, record, assessmentToCommit, request.Execution); err != nil {
				return err
			}
		}
		if err := c.assessmentRepo.Save(txCtx, assessmentToCommit); err != nil {
			return err
		}
		if err := c.runRepo.SaveClaimed(txCtx, runToCommit); err != nil {
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
		return nil, err
	}
	// Publish the prepared terminal state only after every durable fact and the
	// outbox event have committed successfully.
	assessmentToCommit.ClearEvents()
	*request.Assessment = *assessmentToCommit
	*request.Run = runToCommit
	if c.postCommit != nil && len(eventsToStage) > 0 {
		c.postCommit.AfterCommit(ctx, eventsToStage, request.EvaluatedAt)
	}
	return record, nil
}

func (c *committer) validate(request CommitRequest) error {
	if c == nil || c.txRunner == nil || c.assessmentRepo == nil || c.outcomeRepo == nil || c.runRepo == nil || c.eventStager == nil {
		return evalerrors.ModuleNotConfigured("evaluation committer requires transaction, assessment, outcome, run and outbox dependencies")
	}
	if request.Assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if request.Execution == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	if request.Run == nil || request.Run.ID() == "" {
		return fmt.Errorf("evaluation run is required")
	}
	if request.Run.Attempt().Status != evalrun.StatusRunning {
		return fmt.Errorf("evaluation run must be running before commit")
	}
	if request.Run.AssessmentID() != request.Assessment.ID().Uint64() {
		return fmt.Errorf("evaluation run assessment does not match outcome assessment")
	}
	if request.Input == nil || request.Input.Model == nil || request.Input.DefinitionV2 == nil || !request.Input.Model.HasFrozenRuntime() {
		return fmt.Errorf("complete definition_v2 evaluation input is required")
	}
	if !evaluationinput.IsIdentityRef(request.Run.InputSnapshotRef()) {
		return fmt.Errorf("evaluation run input snapshot ref must be isn:v2")
	}
	if request.DescriptorKey.DecisionKind == "" {
		return fmt.Errorf("evaluation descriptor identity is incomplete")
	}
	if err := request.OutcomePolicy.ValidateExecution(request.Execution); err != nil {
		return fmt.Errorf("evaluation outcome completeness: %w", err)
	}
	return nil
}
