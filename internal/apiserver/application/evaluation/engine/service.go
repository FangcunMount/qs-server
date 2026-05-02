package engine

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine/pipeline"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type waiterNotifier = evaluationwaiter.Notifier

// service 评估引擎服务实现
type service struct {
	// 仓储依赖
	assessmentRepo assessment.Repository
	scoreRepo      assessment.ScoreRepository
	reportRepo     report.ReportRepository
	inputResolver  evaluationinput.Resolver

	// 领域服务依赖
	reportBuilder report.ReportBuilder

	// 等待队列注册表（可选，用于长轮询）
	waiterRegistry  waiterNotifier
	txRunner        apptransaction.Runner
	eventStager     EventStager
	reportSaver     pipeline.ReportDurableSaver
	factorScorer    ruleengine.ScaleFactorScorer
	interpreter     interpretengine.Interpreter
	defaultProvider interpretengine.DefaultProvider

	// 处理器链
	pipeline *pipeline.Chain
}

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// ServiceOption 服务选项
type ServiceOption func(*service)

// WithWaiterRegistry 设置等待队列注册表
func WithWaiterRegistry(waiterRegistry waiterNotifier) ServiceOption {
	return func(s *service) {
		s.waiterRegistry = waiterRegistry
	}
}

func WithTransactionalOutbox(txRunner apptransaction.Runner, eventStager EventStager) ServiceOption {
	return func(s *service) {
		s.txRunner = txRunner
		s.eventStager = eventStager
	}
}

func WithReportDurableSaver(saver pipeline.ReportDurableSaver) ServiceOption {
	return func(s *service) {
		s.reportSaver = saver
	}
}

func WithScaleFactorScorer(scorer ruleengine.ScaleFactorScorer) ServiceOption {
	return func(s *service) {
		s.factorScorer = scorer
	}
}

func WithInterpretEngine(interpreter interpretengine.Interpreter, defaultProvider interpretengine.DefaultProvider) ServiceOption {
	return func(s *service) {
		s.interpreter = interpreter
		s.defaultProvider = defaultProvider
	}
}

func WithInputResolver(resolver evaluationinput.Resolver) ServiceOption {
	return func(s *service) {
		if resolver != nil {
			s.inputResolver = resolver
		}
	}
}

// NewService 创建评估引擎服务
func NewService(
	assessmentRepo assessment.Repository,
	scoreRepo assessment.ScoreRepository,
	reportRepo report.ReportRepository,
	inputResolver evaluationinput.Resolver,
	reportBuilder report.ReportBuilder,
	opts ...ServiceOption,
) Service {
	svc := &service{
		assessmentRepo: assessmentRepo,
		scoreRepo:      scoreRepo,
		reportRepo:     reportRepo,
		inputResolver:  inputResolver,
		reportBuilder:  reportBuilder,
	}

	// 应用选项
	for _, opt := range opts {
		opt(svc)
	}

	// 构建处理器链
	svc.pipeline = svc.buildPipeline()

	return svc
}

// buildPipeline 构建处理器链
// 按顺序添加各个处理器，形成完整的评估流程
func (s *service) buildPipeline() *pipeline.Chain {
	chain := pipeline.NewChain()

	// 1. 前置校验处理器
	chain.AddHandler(pipeline.NewValidationHandler())

	// 2. 因子分数计算处理器（从答卷读取预计算分数，按因子聚合）
	chain.AddHandler(pipeline.NewFactorScoreHandler(s.factorScorer))

	// 3. 风险等级计算处理器（计算风险等级，保存分数）
	chain.AddHandler(pipeline.NewRiskLevelHandler(
		pipeline.NewRiskClassifier(),
		pipeline.NewAssessmentScoreWriter(s.scoreRepo),
	))

	// 4. 测评分析解读处理器
	reportSaver := s.reportSaver
	if reportSaver == nil {
		reportSaver = pipeline.NewReportDurableSaver(s.reportRepo)
	}
	chain.AddHandler(pipeline.NewInterpretationHandler(
		pipeline.NewInterpretationGenerator(s.interpreter, s.defaultProvider),
		pipeline.NewInterpretationFinalizer(
			pipeline.NewAssessmentResultWriter(s.assessmentRepo),
			pipeline.NewInterpretReportWriter(s.reportBuilder, reportSaver),
		),
	))

	// 5. 本地 waiter 通知处理器
	if s.waiterRegistry != nil {
		chain.AddHandler(pipeline.NewWaiterNotifyHandler(s.waiterRegistry))
	}

	return chain
}

// Evaluate 执行评估
func (s *service) Evaluate(ctx context.Context, assessmentID uint64) error {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始执行评估",
		"action", "evaluate",
		"resource", "assessment",
		"assessment_id", assessmentID,
	)

	// 参数校验
	if assessmentID == 0 {
		l.Warnw("测评ID为空", "action", "evaluate", "result", "invalid_params")
		return errors.WithCode(errorCode.ErrInvalidArgument, "测评ID不能为空")
	}

	loaded, err := s.assessmentLoader().LoadForEvaluation(ctx, assessmentID)
	if err != nil {
		return err
	}
	if loaded.skipEvaluation {
		return nil
	}
	a := loaded.assessment

	input, err := evaluationInputWorkflow{resolver: s.inputResolver}.Resolve(ctx, a, assessmentID)
	if err != nil {
		s.failureFinalizer().MarkAsFailed(ctx, a, inputResolveFailureReason(err))
		return err
	}

	// 2. 创建评估上下文
	evalCtx := pipeline.NewContext(a, input)

	// 3. 执行处理器链
	l.Infow("开始执行评估处理器链",
		"assessment_id", assessmentID,
		"scale_code", a.MedicalScaleRef().Code().String(),
	)

	if err := s.pipeline.Execute(ctx, evalCtx); err != nil {
		l.Errorw("评估处理器链执行失败",
			"assessment_id", assessmentID,
			"scale_code", a.MedicalScaleRef().Code().String(),
			"result", "failed",
			"error", err.Error(),
		)
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	duration := time.Since(startTime)
	l.Infow("评估执行完成",
		"action", "evaluate",
		"resource", "assessment",
		"result", "success",
		"assessment_id", assessmentID,
		"scale_code", a.MedicalScaleRef().Code().String(),
		"duration_ms", duration.Milliseconds(),
	)

	return nil
}

// EvaluateBatch 批量评估
func (s *service) EvaluateBatch(ctx context.Context, orgID int64, assessmentIDs []uint64) (*BatchResult, error) {
	return batchEvaluator{
		loader:   s.assessmentLoader(),
		evaluate: s.Evaluate,
	}.EvaluateBatch(ctx, orgID, assessmentIDs)
}

func (s *service) assessmentLoader() assessmentLoader {
	return assessmentLoader{repo: s.assessmentRepo}
}

func (s *service) failureFinalizer() evaluationFailureFinalizer {
	return evaluationFailureFinalizer{
		repo:        s.assessmentRepo,
		txRunner:    s.txRunner,
		eventStager: s.eventStager,
	}
}
