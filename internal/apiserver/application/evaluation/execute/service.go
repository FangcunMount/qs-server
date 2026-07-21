package execute

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/google/uuid"
)

const defaultEvaluationRunLease = 2 * time.Minute

// service 评估引擎服务实现
type service struct {
	// 仓储依赖
	assessmentRepo assessment.Repository
	inputResolver  evaluationinput.Resolver

	txRunner    apptransaction.Runner
	eventStager EventStager
	postCommit  appEventing.PostCommitDispatcher

	descriptorRegistry  *evalpipeline.RuntimeDescriptorRegistry
	descriptorExecutor  evalpipeline.DescriptorExecutor
	runtimeResolver     *RuntimeResolver
	runRepo             evaluationrun.Repository
	runLease            time.Duration
	evaluationCommitter outcomecommit.Committer
}

// EventStager 事件暂存器
type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// EngineOption configures the concrete Evaluation engine.
type EngineOption func(*service)

func WithPostCommitDispatcher(dispatcher appEventing.PostCommitDispatcher) EngineOption {
	return func(s *service) {
		s.postCommit = dispatcher
	}
}

// WithTransactionalOutbox 配置事务和事件暂存器
func WithTransactionalOutbox(txRunner apptransaction.Runner, eventStager EventStager) EngineOption {
	return func(s *service) {
		s.txRunner = txRunner
		s.eventStager = eventStager
	}
}

// WithRuntimeDescriptorRegistry configures the sole production runtime route.
func WithRuntimeDescriptorRegistry(registry *evalpipeline.RuntimeDescriptorRegistry) EngineOption {
	return func(s *service) {
		s.descriptorRegistry = registry
	}
}

// WithDescriptorExecutor replaces descriptor execution. Production uses the
// native descriptor pipeline; the option is kept as a narrow testing seam.
func WithDescriptorExecutor(executor evalpipeline.DescriptorExecutor) EngineOption {
	return func(s *service) {
		s.descriptorExecutor = executor
	}
}

// WithRunRepository 配置评估执行 持久化。
func WithRunRepository(repo evaluationrun.Repository) EngineOption {
	return func(s *service) {
		s.runRepo = repo
	}
}

// WithRunLease configures how long one worker owns an EvaluationRun claim.
// It is primarily exposed for deterministic tests and unusually long-running
// deployments; production uses a conservative default.
func WithRunLease(lease time.Duration) EngineOption {
	return func(s *service) {
		if lease > 0 {
			s.runLease = lease
		}
	}
}

// WithEvaluationCommitter configures the canonical Evaluation success boundary.
func WithEvaluationCommitter(committer outcomecommit.Committer) EngineOption {
	return func(s *service) {
		s.evaluationCommitter = committer
	}
}

// NewEngine creates the Evaluation engine. Production assembly must configure
// an EvaluationCommitter.
func NewEngine(
	assessmentRepo assessment.Repository,
	inputResolver evaluationinput.Resolver,
	opts ...EngineOption,
) Engine {
	svc := &service{
		assessmentRepo:     assessmentRepo,
		inputResolver:      inputResolver,
		descriptorExecutor: descriptorDrivenExecutor{},
		runLease:           defaultEvaluationRunLease,
	}

	for _, opt := range opts {
		opt(svc)
	}
	svc.runtimeResolver = NewRuntimeResolver(svc.descriptorRegistry, svc.descriptorExecutor)

	return svc
}

// Evaluate 执行单次评估
func (s *service) Evaluate(ctx context.Context, assessmentID uint64) error {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始执行评估",
		"action", "evaluate",
		"resource", "assessment",
		"assessment_id", assessmentID,
	)

	if assessmentID == 0 {
		l.Warnw("评估ID为空", "action", "evaluate", "result", "invalid_params")
		return evalerrors.InvalidArgument("评估ID不能为空")
	}

	// 加载评估数据
	loaded, err := s.assessmentLoader().LoadForEvaluation(ctx, assessmentID)
	if err != nil {
		return err
	}
	if loaded.skipEvaluation {
		return nil
	}
	a := loaded.assessment
	claimAt := time.Now()
	var previousInputSnapshotRef string
	if a.Status().IsFailed() {
		if s.runRepo == nil {
			return evalerrors.ModuleNotConfigured("evaluation run repository is not configured")
		}
		latest, latestErr := s.runRepo.FindLatestByAssessmentID(ctx, assessmentID)
		if latestErr != nil {
			return fmt.Errorf("load retryable evaluation run: %w", latestErr)
		}
		retryableFailure := latest != nil && latest.Retryable()
		expiredRunningAttempt := latest != nil && latest.Attempt().Status == evalrun.StatusRunning && !latest.HasActiveLease(claimAt)
		if !retryableFailure && !expiredRunningAttempt {
			l.Infow("测评失败且不存在可重试运行，跳过重复请求",
				"assessment_id", assessmentID,
				"result", "terminal_failure_skipped",
			)
			return nil
		}
		if latest != nil {
			previousInputSnapshotRef = latest.InputSnapshotRef()
		}
	}
	claim, err := s.claimEvaluationRun(ctx, assessmentID, uuid.NewString(), log.ExtractTraceID(ctx), claimAt)
	if err != nil {
		return err
	}
	if !claim.Claimed {
		l.Infow("测评执行已有有效 claim，跳过重复执行",
			"assessment_id", assessmentID,
			"evaluation_run_id", claim.Run.ID().String(),
			"result", "duplicate_skipped",
		)
		return nil
	}
	evaluationRun := claim.Run
	// EV-R010: observe wall time only for claimed attempts (not skips/duplicates).
	algorithmFamily := "unknown"
	runResult := "failed"
	defer func() {
		observeEvaluationRunDuration(algorithmFamily, runResult, time.Since(startTime), s.runLease)
	}()
	if a.Status().IsFailed() {
		if err := a.ResumeForExecutionRetry(); err != nil {
			return err
		}
	}
	// 解析评估输入
	input, err := evaluationInputWorkflow{resolver: s.inputResolver}.Resolve(ctx, a, assessmentID)
	if err != nil {
		if isInputResolveInterrupted(err) {
			return err
		}
		return s.finalizeEvaluationFailure(ctx, a, &evaluationRun, inputResolveFailureReason(err), runFailureFromInputResolveError(err), err)
	}

	if ref := inputSnapshotRefFromResolvedInput(a, input); ref != "" {
		// EV-R009: a retry must execute against the same verified input as the
		// previous attempt; drift is a terminal validation failure, not a
		// silent recompute over different data.
		if err := validateInputSnapshotRefAcrossAttempts(previousInputSnapshotRef, ref); err != nil {
			return s.finalizeEvaluationFailure(ctx, a, &evaluationRun, "评估输入在重试间发生漂移: "+err.Error(), evalrun.Failure{Kind: evalrun.FailureKindValidation, Message: err.Error()}, err)
		}
		if err := evaluationRun.AttachInputSnapshot(ref); err != nil {
			return s.finalizeEvaluationFailure(ctx, a, &evaluationRun, "评估输入快照无效: "+err.Error(), evalrun.Failure{Kind: evalrun.FailureKindInternal, Message: err.Error()}, err)
		}
		if err := s.persistClaimedEvaluationRun(ctx, evaluationRun); err != nil {
			return err
		}
	}

	// 解析并执行评估模型
	if s.runtimeResolver == nil {
		err := evalerrors.ModuleNotConfigured("evaluation runtime resolver is not configured")
		return s.finalizeEvaluationFailure(ctx, a, &evaluationRun, "评估流程执行失败: "+err.Error(), evalrun.Failure{Kind: evalrun.FailureKindInternal, Message: err.Error()}, err)
	}

	resolved, resolveErr := s.runtimeResolver.ResolveExecution(a, input)
	if resolveErr != nil {
		l.Errorw("评估运行时解析失败",
			"assessment_id", assessmentID,
			"evaluation_run", evaluationRun.String(),
			"evaluation_run_id", evaluationRun.ID().String(),
			"model_key", resolved.ExecutionIdentity.String(),
			"runtime_descriptor_key", resolved.DescriptorKey.String(),
			"result", "failed",
			"error", resolveErr.Error(),
		)
		return s.finalizeEvaluationFailure(ctx, a, &evaluationRun, "评估流程执行失败: "+resolveErr.Error(), evalrun.Failure{Kind: evalrun.FailureKindValidation, Message: resolveErr.Error()}, resolveErr)
	}
	if family := string(resolved.DescriptorKey.AlgorithmFamily); family != "" {
		algorithmFamily = family
	}

	l.Infow("开始执行评估器",
		"assessment_id", assessmentID,
		"evaluation_run", evaluationRun.String(),
		"evaluation_run_id", evaluationRun.ID().String(),
		"model_key", resolved.ExecutionIdentity.String(),
		"runtime_descriptor_key", resolved.DescriptorKey.String(),
		"model_code", evaluationModelCode(a, input),
	)

	evaluationOutcome, err := s.runtimeResolver.ExecuteResolved(ctx, resolved, a, input)
	if err != nil {
		l.Errorw("评估模型执行失败",
			"assessment_id", assessmentID,
			"evaluation_run", evaluationRun.String(),
			"evaluation_run_id", evaluationRun.ID().String(),
			"model_key", resolved.ExecutionIdentity.String(),
			"runtime_descriptor_key", resolved.DescriptorKey.String(),
			"model_code", evaluationModelCode(a, input),
			"result", "failed",
			"error", err.Error(),
		)
		return s.finalizeEvaluationFailure(ctx, a, &evaluationRun, "评估流程执行失败: "+err.Error(), evalrun.Failure{Kind: evalrun.FailureKindCalculation, Message: err.Error(), Retryable: true}, err)
	}

	// 执行评估成功，可靠提交规范 EvaluationOutcome。
	err = s.persistEvaluationOutcome(ctx, outcomecommit.CommitRequest{
		Assessment:    a,
		Input:         input,
		Execution:     evaluationOutcome,
		DescriptorKey: resolved.DescriptorKey,
		Run:           &evaluationRun,
		EvaluatedAt:   time.Now(),
	})
	if err != nil {
		l.Errorw("评估结果写入失败",
			"assessment_id", assessmentID,
			"model_key", resolved.ExecutionIdentity.String(),
			"runtime_descriptor_key", resolved.DescriptorKey.String(),
			"model_code", evaluationModelCode(a, input),
			"result", "failed",
			"error", err.Error(),
		)
		return s.finalizeEvaluationFailure(ctx, a, &evaluationRun, "评估流程执行失败: "+err.Error(), evalrun.Failure{Kind: evalrun.FailureKindInternal, Message: err.Error(), Retryable: true}, err)
	}

	l.Infow("评估执行完成",
		"action", "evaluate",
		"resource", "assessment",
		"result", "success",
		"assessment_id", assessmentID,
		"evaluation_run", evaluationRun.String(),
		"evaluation_run_id", evaluationRun.ID().String(),
		"model_key", resolved.ExecutionIdentity.String(),
		"runtime_descriptor_key", resolved.DescriptorKey.String(),
		"model_code", evaluationModelCode(a, input),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	runResult = "success"
	return nil
}

func (s *service) persistEvaluationOutcome(
	ctx context.Context,
	request outcomecommit.CommitRequest,
) error {
	if s.evaluationCommitter == nil {
		return evalerrors.ModuleNotConfigured("evaluation committer is not configured")
	}
	record, err := s.evaluationCommitter.Commit(ctx, request)
	if err != nil {
		return err
	}
	if record != nil {
		logger.L(ctx).Infow("评估结果已持久化并投递报告生成事件",
			"action", "evaluate",
			"assessment_id", request.Assessment.ID().Uint64(),
			"evaluation_run_id", request.Run.ID().String(),
			"outcome_id", record.ID().String(),
			"model_code", record.Model().Code,
			"result", "success",
		)
	}
	return nil
}

func (s *service) finalizeEvaluationFailure(
	ctx context.Context,
	a *assessment.Assessment,
	run *evalrun.EvaluationRun,
	reason string,
	failure evalrun.Failure,
	cause error,
) error {
	if err := s.failureFinalizer().Finalize(ctx, a, run, reason, failure); err != nil {
		return fmt.Errorf("persist evaluation failure: %w", err)
	}
	return cause
}

// resolveExecutionIdentity 解析 v2 评估执行键。
func resolveExecutionIdentity(a *assessment.Assessment, input *evaluationinput.InputSnapshot) evaluation.ExecutionIdentity {
	if input != nil && input.Model != nil {
		inputKey := input.Model.ModelRef().ExecutionIdentity()
		if a == nil || a.EvaluationModelRef() == nil || a.EvaluationModelRef().IsEmpty() {
			return inputKey
		}
		assessmentKey := a.EvaluationModelRef().ExecutionIdentity()
		if assessmentKey.Algorithm == "" && inputKey.Algorithm != "" {
			return inputKey
		}
		return assessmentKey
	}
	if a != nil && a.EvaluationModelRef() != nil && !a.EvaluationModelRef().IsEmpty() {
		return a.EvaluationModelRef().ExecutionIdentity()
	}
	return evaluation.ExecutionIdentity{}
}

// evaluationModelCode 解析评估模型代码
func evaluationModelCode(a *assessment.Assessment, input *evaluationinput.InputSnapshot) string {
	if input != nil && input.Model != nil && input.Model.Code != "" {
		return input.Model.Code
	}
	if a != nil && a.EvaluationModelRef() != nil {
		return a.EvaluationModelRef().Code().String()
	}
	return ""
}

// assessmentLoader 评估数据加载器
func (s *service) assessmentLoader() assessmentLoader {
	return assessmentLoader{repo: s.assessmentRepo}
}

// failureFinalizer 评估失败标记器
func (s *service) failureFinalizer() evaluationFailureFinalizer {
	return evaluationFailureFinalizer{
		repo:        s.assessmentRepo,
		runRepo:     s.runRepo,
		txRunner:    s.txRunner,
		eventStager: s.eventStager,
		postCommit:  s.postCommit,
	}
}
