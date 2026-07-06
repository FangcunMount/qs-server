package execute

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evaluationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evaluationscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scoring"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	interpretationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// service 评估引擎服务实现
type service struct {
	// 仓储依赖
	assessmentRepo assessment.Repository
	inputResolver  evaluationinput.Resolver

	txRunner     apptransaction.Runner
	eventStager  EventStager
	readyIndexer *appEventing.PostCommitReadyIndexer

	evaluators            EvaluatorRegistry
	resultWriter          interpretationreporting.Writer
	scoringWriter         evaluationscoring.Writer
	interpretationService interpretationapp.Service
	scoringSnapshotStore  evaluationscoring.ScoringSnapshotStore
	asyncInterpretation   bool
	reportStatus          *reportstatus.Reporter
}

// EventStager 事件暂存器
type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// ServiceOption 服务选项
type ServiceOption func(*service)

func WithPostCommitReadyIndexer(indexer *appEventing.PostCommitReadyIndexer) ServiceOption {
	return func(s *service) {
		s.readyIndexer = indexer
	}
}

// WithTransactionalOutbox 配置事务和事件暂存器
func WithTransactionalOutbox(txRunner apptransaction.Runner, eventStager EventStager) ServiceOption {
	return func(s *service) {
		s.txRunner = txRunner
		s.eventStager = eventStager
	}
}

// WithEvaluatorRegistry 配置评估器注册表
func WithEvaluatorRegistry(registry EvaluatorRegistry) ServiceOption {
	return func(s *service) {
		s.evaluators = registry
	}
}

func WithReportStatusReporter(reporter *reportstatus.Reporter) ServiceOption {
	return func(s *service) {
		s.reportStatus = reporter
	}
}

// WithScoringWriter configures the scoring outcome writer for split-phase evaluation.
func WithScoringWriter(writer evaluationscoring.Writer) ServiceOption {
	return func(s *service) {
		s.scoringWriter = writer
	}
}

// WithInterpretationService configures the interpretation report generation service.
func WithInterpretationService(svc interpretationapp.Service) ServiceOption {
	return func(s *service) {
		s.interpretationService = svc
	}
}

// WithAsyncInterpretation enables split-phase evaluation (scoring event + async report).
func WithAsyncInterpretation(enabled bool) ServiceOption {
	return func(s *service) {
		s.asyncInterpretation = enabled
	}
}

// WithScoringSnapshotStore configures durable scoring snapshots for async report generation.
func WithScoringSnapshotStore(store evaluationscoring.ScoringSnapshotStore) ServiceOption {
	return func(s *service) {
		s.scoringSnapshotStore = store
	}
}

// NewService 创建评估引擎服务实例
func NewService(
	assessmentRepo assessment.Repository,
	inputResolver evaluationinput.Resolver,
	resultWriter interpretationreporting.Writer,
	opts ...ServiceOption,
) Service {
	svc := &service{
		assessmentRepo: assessmentRepo,
		inputResolver:  inputResolver,
		evaluators:     newEmptyEvaluatorRegistry(),
		resultWriter:   resultWriter,
	}

	// 应用选项
	for _, opt := range opts {
		opt(svc)
	}

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
	if s.reportStatus != nil {
		assessmentID, answerSheetID := evaluationapp.ReportStatusIDs(a)
		s.reportStatus.SetProcessing(ctx, assessmentID, answerSheetID, "scoring")
	}

	// 解析评估输入
	input, err := evaluationInputWorkflow{resolver: s.inputResolver}.Resolve(ctx, a, assessmentID)
	if err != nil {
		s.failureFinalizer().MarkAsFailed(ctx, a, inputResolveFailureReason(err))
		return err
	}

	// 解析评估模型执行键
	evaluatorKey := resolveEvaluatorKey(a, input)

	l.Infow("开始执行评估解释器",
		"assessment_id", assessmentID,
		"model_key", evaluatorKey.String(),
		"model_code", evaluationModelCode(a, input),
	)

	// 解析评估模型执行器
	if s.evaluators == nil {
		err := evalerrors.ModuleNotConfigured("evaluation evaluator registry is not configured")
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	// 解析评估模型执行器
	evaluator, err := s.evaluators.Resolve(evaluatorKey)
	if err != nil {
		l.Errorw("评估模型执行器解析失败",
			"assessment_id", assessmentID,
			"model_key", evaluatorKey.String(),
			"result", "failed",
			"error", err.Error(),
		)
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	// 执行评估模型
	evaluationOutcome, err := evaluator.Execute(ctx, ExecutionInput{Assessment: a, Input: input})
	if err != nil {
		l.Errorw("评估模型执行失败",
			"assessment_id", assessmentID,
			"model_key", evaluatorKey.String(),
			"model_code", evaluationModelCode(a, input),
			"result", "failed",
			"error", err.Error(),
		)
		// 标记评估流程执行失败
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	// 执行评估成功，写入计分结果并生成报告
	if err := s.persistEvaluationOutcome(ctx, evaloutcome.Outcome{Assessment: a, Input: input, Execution: evaluationOutcome}); err != nil {
		l.Errorw("评估结果写入失败",
			"assessment_id", assessmentID,
			"model_key", evaluatorKey.String(),
			"model_code", evaluationModelCode(a, input),
			"result", "failed",
			"error", err.Error(),
		)
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	l.Infow("评估执行完成",
		"action", "evaluate",
		"resource", "assessment",
		"result", "success",
		"assessment_id", assessmentID,
		"model_key", evaluatorKey.String(),
		"model_code", evaluationModelCode(a, input),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return nil
}

func (s *service) persistEvaluationOutcome(ctx context.Context, outcome evaloutcome.Outcome) error {
	if s.scoringWriter != nil && s.interpretationService != nil {
		if s.asyncInterpretation {
			if err := s.scoringWriter.Write(ctx, outcome); err != nil {
				return err
			}
			outcome.Assessment.StageEvaluatedEvent(time.Now())
			return s.failureFinalizer().SaveAssessmentWithEvents(ctx, outcome.Assessment)
		}
		if err := s.scoringWriter.Write(ctx, outcome); err != nil {
			return err
		}
		return s.interpretationService.GenerateAndPersist(ctx, outcome)
	}
	if s.resultWriter == nil {
		return evalerrors.ModuleNotConfigured("evaluation result writer is not configured")
	}
	return s.resultWriter.Write(ctx, outcome)
}

// GenerateReport generates and persists the interpretation report for an evaluated assessment.
func (s *service) GenerateReport(ctx context.Context, assessmentID uint64) error {
	l := logger.L(ctx)
	if assessmentID == 0 {
		return evalerrors.InvalidArgument("评估ID不能为空")
	}
	loaded, err := s.assessmentLoader().LoadForInterpretation(ctx, assessmentID)
	if err != nil {
		return err
	}
	if loaded.skipEvaluation {
		return nil
	}
	a := loaded.assessment
	markReportFailed := func(reason string, err error) error {
		s.failureFinalizer().MarkAsFailed(ctx, a, reason)
		return err
	}
	input, err := evaluationInputWorkflow{resolver: s.inputResolver}.Resolve(ctx, a, assessmentID)
	if err != nil {
		return markReportFailed("报告生成输入解析失败: "+inputResolveFailureReason(err), err)
	}

	var execution *assessment.AssessmentOutcome
	if a.Status().IsEvaluated() && s.scoringSnapshotStore != nil {
		execution, err = s.scoringSnapshotStore.Load(ctx, assessmentID)
		if err != nil {
			return markReportFailed("报告生成计分快照读取失败: "+err.Error(), err)
		}
		if execution == nil {
			err = evalerrors.InvalidArgument("计分快照不存在")
			return markReportFailed(err.Error(), err)
		}
	} else {
		evaluatorKey := resolveEvaluatorKey(a, input)
		evaluator, resolveErr := s.evaluators.Resolve(evaluatorKey)
		if resolveErr != nil {
			return markReportFailed("报告生成执行器解析失败: "+resolveErr.Error(), resolveErr)
		}
		execution, err = evaluator.Execute(ctx, ExecutionInput{Assessment: a, Input: input})
		if err != nil {
			return markReportFailed("报告生成计分失败: "+err.Error(), err)
		}
	}
	if s.interpretationService == nil {
		err = evalerrors.ModuleNotConfigured("interpretation service is not configured")
		return markReportFailed(err.Error(), err)
	}
	outcome := evaloutcome.Outcome{Assessment: a, Input: input, Execution: execution}
	if err := s.interpretationService.GenerateAndPersist(ctx, outcome); err != nil {
		return markReportFailed("报告生成持久化失败: "+err.Error(), err)
	}
	if s.scoringSnapshotStore != nil {
		_ = s.scoringSnapshotStore.Delete(ctx, assessmentID)
	}
	l.Infow("报告生成完成",
		"action", "generate_report",
		"assessment_id", assessmentID,
	)
	return nil
}

// resolveEvaluatorKey 解析 v2 评估执行键。
func resolveEvaluatorKey(a *assessment.Assessment, input *evaluationinput.InputSnapshot) evaluation.EvaluatorKey {
	if input != nil && input.Model != nil {
		inputKey := input.Model.ModelRef().EvaluatorKey()
		if a == nil || a.EvaluationModelRef() == nil || a.EvaluationModelRef().IsEmpty() {
			return inputKey
		}
		assessmentKey := a.EvaluationModelRef().EvaluatorKey()
		if assessmentKey.Algorithm == "" && inputKey.Algorithm != "" {
			return inputKey
		}
		return assessmentKey
	}
	if a != nil && a.EvaluationModelRef() != nil && !a.EvaluationModelRef().IsEmpty() {
		return a.EvaluationModelRef().EvaluatorKey()
	}
	return evaluation.EvaluatorKey{}
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

// EvaluateBatch 批量评估
func (s *service) EvaluateBatch(ctx context.Context, orgID int64, assessmentIDs []uint64) (*BatchResult, error) {
	return batchEvaluator{
		loader:   s.assessmentLoader(),
		evaluate: s.Evaluate,
	}.EvaluateBatch(ctx, orgID, assessmentIDs)
}

// assessmentLoader 评估数据加载器
func (s *service) assessmentLoader() assessmentLoader {
	return assessmentLoader{repo: s.assessmentRepo}
}

// failureFinalizer 评估失败标记器
func (s *service) failureFinalizer() evaluationFailureFinalizer {
	return evaluationFailureFinalizer{
		repo:         s.assessmentRepo,
		txRunner:     s.txRunner,
		eventStager:  s.eventStager,
		reportStatus: s.reportStatus,
		readyIndexer: s.readyIndexer,
	}
}
