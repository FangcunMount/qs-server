package generation

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ExecuteStatus string

const (
	ExecuteStatusGenerated  ExecuteStatus = "generated"
	ExecuteStatusProcessing ExecuteStatus = "processing"
)

type ExecuteResult struct {
	Status     ExecuteStatus
	Generation *domaingeneration.ReportGeneration
	Run        *interpretationrun.InterpretationRun
	Artifact   *domainreport.Artifact
}

// Executor is the production Interpretation write use case. It consumes a
// durable EvaluationOutcome, never an Assessment or live Evaluator result.
type Executor interface {
	Execute(ctx context.Context, input interpinput.InterpretationInput, traceID string) (*ExecuteResult, error)
}

type executor struct {
	starter   Starter
	builders  registry.ReportBuilderRegistry
	committer InterpretationCommitter
	now       func() time.Time
	newID     func() meta.ID
}

func NewExecutor(
	starter Starter,
	builders registry.ReportBuilderRegistry,
	committer InterpretationCommitter,
) (Executor, error) {
	if starter == nil || builders == nil || committer == nil {
		return nil, fmt.Errorf("interpretation executor dependencies are required")
	}
	return &executor{starter: starter, builders: builders, committer: committer, now: time.Now, newID: meta.New}, nil
}

func (e *executor) Execute(ctx context.Context, input interpinput.InterpretationInput, traceID string) (*ExecuteResult, error) {
	if e == nil {
		return nil, fmt.Errorf("interpretation executor is not configured")
	}
	if input.OutcomeID.IsZero() || input.Report.ReportType.IsEmpty() || input.Report.TemplateVersion.IsEmpty() {
		return nil, fmt.Errorf("interpretation input outcome, report type and template version are required")
	}
	start, err := e.starter.Start(ctx, StartRequest{Key: domaingeneration.Key{OutcomeID: input.OutcomeID, ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion}, TraceID: traceID})
	if err != nil {
		return nil, err
	}
	switch start.Status {
	case StartStatusGenerated:
		return &ExecuteResult{Status: ExecuteStatusGenerated, Generation: start.Generation, Artifact: start.Artifact}, nil
	case StartStatusProcessing:
		return &ExecuteResult{Status: ExecuteStatusProcessing, Generation: start.Generation, Run: start.Run}, nil
	case StartStatusStarted:
		return e.buildAndCommit(ctx, input, start.Generation, start.Run)
	default:
		return nil, fmt.Errorf("unsupported generation start status %s", start.Status)
	}
}

func (e *executor) buildAndCommit(ctx context.Context, input interpinput.InterpretationInput, generationRecord *domaingeneration.ReportGeneration, runRecord *interpretationrun.InterpretationRun) (*ExecuteResult, error) {
	key, ok := registry.MechanismReportBuilderKeyFromInput(input)
	if !ok {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindInput, Code: "unsupported_mechanism", SafeMessage: "报告生成配置不受支持", Retryable: false})
	}
	builder, err := e.builders.ResolveByMechanism(key)
	if err != nil {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindTemplate, Code: "builder_not_found", SafeMessage: "报告生成器未配置", Retryable: false})
	}
	draft, err := builder.Build(ctx, input)
	if err != nil {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "报告生成失败", Retryable: true})
	}
	if draft == nil {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "empty_draft", SafeMessage: "报告生成失败", Retryable: true})
	}

	at := e.now()
	artifact, err := domainreport.NewArtifact(domainreport.ArtifactInput{
		ID: e.newID(), GenerationID: generationRecord.ID(), OutcomeID: input.OutcomeID, InterpretationRunID: runRecord.ID(),
		Association: input.Association, ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion,
		Content: draft.Content(), GeneratedAt: at,
	})
	if err != nil {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "invalid_artifact", SafeMessage: "报告生成失败", Retryable: false})
	}
	committed, err := e.committer.CommitSuccess(ctx, CommitSuccessRequest{
		Generation: generationRecord, Run: runRecord, Artifact: artifact, BuilderIdentity: builder.BuilderIdentity(),
		ContentSchemaVersion: builder.ContentSchemaVersion(), CompletedAt: at,
	})
	if err != nil {
		return nil, err
	}
	return &ExecuteResult{Status: ExecuteStatusGenerated, Generation: committed.Generation, Run: committed.Run, Artifact: committed.Artifact}, nil
}

func (e *executor) fail(ctx context.Context, generationRecord *domaingeneration.ReportGeneration, runRecord *interpretationrun.InterpretationRun, input interpinput.InterpretationInput, failure interpretationrun.Failure) error {
	if generationRecord == nil || runRecord == nil {
		return fmt.Errorf("interpretation generation and run are required")
	}
	committed, err := e.committer.CommitFailure(ctx, CommitFailureRequest{
		Generation: generationRecord, Run: runRecord, OutcomeID: input.OutcomeID, Association: input.Association, Failure: failure, FailedAt: e.now(),
	})
	if err != nil {
		return err
	}
	return &FailedError{GenerationID: committed.Generation.ID(), RunID: committed.Run.ID(), Failure: failure}
}

// ExecuteOutcome is the production adapter from the immutable Evaluation
// fact to the Interpretation executor. It lives here to make the direct read
// boundary explicit to callers while keeping Builder contracts Evaluation-free.
func ExecuteOutcome(ctx context.Context, service Executor, record interface{ ID() meta.ID }, input interpinput.InterpretationInput, traceID string) (*ExecuteResult, error) {
	if service == nil || record == nil {
		return nil, fmt.Errorf("interpretation executor and evaluation outcome are required")
	}
	if input.OutcomeID != record.ID() {
		return nil, fmt.Errorf("interpretation input outcome id does not match record")
	}
	return service.Execute(ctx, input, traceID)
}
