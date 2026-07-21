package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ExecuteStatus string

const (
	ExecuteStatusGenerated  ExecuteStatus = "generated"
	ExecuteStatusProcessing ExecuteStatus = "processing"
	ExecuteStatusBlocked    ExecuteStatus = "blocked"
)

type ExecuteResult struct {
	Status          ExecuteStatus
	Generation      *domaingeneration.ReportGeneration
	Run             *interpretationrun.InterpretationRun
	InterpretReport *domainreport.InterpretReport
}

// Executor is the production Interpretation write use case. It consumes a
// durable EvaluationOutcome, never an Assessment or live Evaluator result.
type Executor interface {
	Execute(ctx context.Context, input interpinput.InterpretationInput, traceID string) (*ExecuteResult, error)
}

type executor struct {
	starter       Starter
	builders      rendering.Registry
	committer     InterpretationCommitter
	now           func() time.Time
	newID         func() meta.ID
	logBuildError func(context.Context, error, *domaingeneration.ReportGeneration, *interpretationrun.InterpretationRun, rendering.Builder)
}

func NewExecutor(
	starter Starter,
	builders rendering.Registry,
	committer InterpretationCommitter,
) (Executor, error) {
	if starter == nil || builders == nil || committer == nil {
		return nil, fmt.Errorf("interpretation executor dependencies are required")
	}
	return &executor{
		starter: starter, builders: builders, committer: committer, now: time.Now, newID: meta.New,
		logBuildError: logBuilderFailure,
	}, nil
}

func (e *executor) Execute(ctx context.Context, input interpinput.InterpretationInput, traceID string) (*ExecuteResult, error) {
	if e == nil {
		return nil, fmt.Errorf("interpretation executor is not configured")
	}
	if input.OutcomeID.IsZero() || input.Report.ReportType.IsEmpty() || input.Report.TemplateVersion.IsEmpty() {
		return nil, fmt.Errorf("interpretation input outcome, report type and template version are required")
	}
	logger.L(ctx).Infow("开始生成报告",
		"action", "generate_interpretation_report",
		"outcome_id", input.OutcomeID.String(),
		"assessment_id", input.Association.AssessmentID.String(),
		"testee_id", input.Association.TesteeID,
		"report_type", input.Report.ReportType.String(),
		"template_version", input.Report.TemplateVersion.String(),
		"trace_id", traceID,
	)
	start, err := e.starter.Start(ctx, StartRequest{Key: domaingeneration.Key{OutcomeID: input.OutcomeID, ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion}, TraceID: traceID})
	if err != nil {
		return nil, err
	}
	switch start.Status {
	case StartStatusGenerated:
		logger.L(ctx).Infow("报告已生成，复用已有结果",
			"action", "generate_interpretation_report",
			"outcome_id", input.OutcomeID.String(),
			"generation_id", start.Generation.ID().String(),
			"report_id", start.InterpretReport.ID().String(),
			"result", "idempotent_hit",
		)
		return &ExecuteResult{Status: ExecuteStatusGenerated, Generation: start.Generation, InterpretReport: start.InterpretReport}, nil
	case StartStatusProcessing:
		logger.L(ctx).Infow("报告生成正在进行，复用运行中任务",
			"action", "generate_interpretation_report",
			"outcome_id", input.OutcomeID.String(),
			"generation_id", start.Generation.ID().String(),
			"run_id", start.Run.ID().String(),
			"result", "processing",
		)
		return &ExecuteResult{Status: ExecuteStatusProcessing, Generation: start.Generation, Run: start.Run}, nil
	case StartStatusBlocked:
		return &ExecuteResult{Status: ExecuteStatusBlocked, Generation: start.Generation, Run: start.Run}, nil
	case StartStatusStarted:
		return e.buildAndCommit(ctx, input, start.Generation, start.Run)
	default:
		return nil, fmt.Errorf("unsupported generation start status %s", start.Status)
	}
}

func (e *executor) buildAndCommit(ctx context.Context, input interpinput.InterpretationInput, generationRecord *domaingeneration.ReportGeneration, runRecord *interpretationrun.InterpretationRun) (*ExecuteResult, error) {
	key, ok := rendering.KeyFromInput(input)
	if !ok {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindInput, Code: "unsupported_mechanism", SafeMessage: "报告生成配置不受支持", Retryable: false})
	}
	builder, err := e.builders.ResolveByMechanism(key)
	if err != nil {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindTemplate, Code: "builder_not_found", SafeMessage: "报告生成器未配置", Retryable: false})
	}
	draft, err := builder.Build(ctx, input)
	if err != nil {
		e.logBuildError(ctx, err, generationRecord, runRecord, builder)
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "build_failed", SafeMessage: "报告生成失败", Retryable: true})
	}
	if draft == nil {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "empty_draft", SafeMessage: "报告生成失败", Retryable: true})
	}

	at := e.now()
	artifact, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID: e.newID(), GenerationID: generationRecord.ID(), OutcomeID: input.OutcomeID, InterpretationRunID: runRecord.ID(),
		Association: input.Association, ReportType: input.Report.ReportType, TemplateVersion: input.Report.TemplateVersion,
		BuilderIdentity: builder.BuilderIdentity(), ContentSchemaVersion: builder.ContentSchemaVersion(),
		Content: draft.Content(), GeneratedAt: at,
	})
	if err != nil {
		return nil, e.fail(ctx, generationRecord, runRecord, input, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "invalid_artifact", SafeMessage: "报告生成失败", Retryable: false})
	}
	committed, err := e.committer.CommitSuccess(ctx, CommitSuccessRequest{
		Generation: generationRecord, Run: runRecord, InterpretReport: artifact, BuilderIdentity: builder.BuilderIdentity(),
		ContentSchemaVersion: builder.ContentSchemaVersion(), CompletedAt: at,
	})
	if err != nil {
		return nil, err
	}
	logger.L(ctx).Infow("报告已生成并持久化",
		"action", "generate_interpretation_report",
		"outcome_id", input.OutcomeID.String(),
		"assessment_id", input.Association.AssessmentID.String(),
		"generation_id", committed.Generation.ID().String(),
		"run_id", committed.Run.ID().String(),
		"report_id", committed.InterpretReport.ID().String(),
		"builder_identity", builder.BuilderIdentity(),
		"result", "success",
	)
	return &ExecuteResult{Status: ExecuteStatusGenerated, Generation: committed.Generation, Run: committed.Run, InterpretReport: committed.InterpretReport}, nil
}

func logBuilderFailure(ctx context.Context, err error, generationRecord *domaingeneration.ReportGeneration, runRecord *interpretationrun.InterpretationRun, builder rendering.Builder) {
	fields := []interface{}{"error", err}
	if generationRecord != nil {
		fields = append(fields, "generation_id", generationRecord.ID().String(), "outcome_id", generationRecord.Key().OutcomeID.String(), "template_version", generationRecord.Key().TemplateVersion.String())
	}
	if runRecord != nil {
		fields = append(fields, "run_id", runRecord.ID().String(), "trace_id", runRecord.TraceID())
	}
	if builder != nil {
		fields = append(fields, "builder_identity", builder.BuilderIdentity())
	}
	logger.L(ctx).Errorw("interpretation report builder failed", fields...)
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
	return &FailedError{GenerationID: committed.Generation.ID(), RunID: committed.Run.ID(), Failure: failure, Origin: committed.Run.Origin(), Decision: committed.Run.RetryDecision()}
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
