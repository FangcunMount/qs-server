package engine

import (
	"context"
	stderrors "errors"
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
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
	chain.AddHandler(pipeline.NewRiskLevelHandler(s.scoreRepo))

	// 4. 测评分析解读处理器
	interpretationHandler := pipeline.NewInterpretationHandler(s.assessmentRepo, s.reportRepo, s.reportBuilder)
	interpretationHandler.SetReportDurableSaver(s.reportSaver)
	interpretationHandler.SetInterpretEngine(s.interpreter, s.defaultProvider)
	chain.AddHandler(interpretationHandler)

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

	// 1. 加载 Assessment
	l.Debugw("加载测评数据",
		"assessment_id", assessmentID,
		"action", "read",
	)

	id := meta.FromUint64(assessmentID)
	a, err := s.assessmentRepo.FindByID(ctx, id)
	if err != nil {
		l.Errorw("加载测评数据失败",
			"assessment_id", assessmentID,
			"action", "read",
			"result", "failed",
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	l.Debugw("测评数据加载成功",
		"assessment_id", assessmentID,
		"status", a.Status().String(),
		"result", "success",
	)

	// 检查状态
	if !a.Status().IsSubmitted() {
		l.Warnw("测评状态不正确",
			"assessment_id", assessmentID,
			"status", a.Status().String(),
			"expected_status", "submitted",
			"result", "failed",
		)
		return errors.WithCode(errorCode.ErrAssessmentInvalidStatus, "测评状态不正确，无法评估")
	}

	// 检查是否有关联量表（纯问卷模式不需要评估）
	if a.MedicalScaleRef() == nil {
		l.Infow("纯问卷模式，跳过评估",
			"assessment_id", assessmentID,
			"mode", "questionnaire_only",
			"result", "skipped",
		)
		return nil
	}

	if s.inputResolver == nil {
		return errors.WithCode(errorCode.ErrModuleInitializationFailed, "evaluation input resolver is not configured")
	}
	input, err := s.inputResolver.Resolve(ctx, evaluationinput.InputRef{
		AssessmentID:         assessmentID,
		MedicalScaleCode:     a.MedicalScaleRef().Code().String(),
		AnswerSheetID:        a.AnswerSheetRef().ID().Uint64(),
		QuestionnaireCode:    a.QuestionnaireRef().Code().String(),
		QuestionnaireVersion: a.QuestionnaireRef().Version(),
	})
	if err != nil {
		s.markAsFailed(ctx, a, inputResolveFailureReason(err))
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
		s.markAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
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

func inputResolveFailureReason(err error) string {
	var carrier evaluationinput.FailureReasonCarrier
	if stderrors.As(err, &carrier) {
		return carrier.FailureReason()
	}
	return "评估输入加载失败: " + err.Error()
}

// EvaluateBatch 批量评估
func (s *service) EvaluateBatch(ctx context.Context, orgID int64, assessmentIDs []uint64) (*BatchResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始批量评估",
		"action", "evaluate_batch",
		"resource", "assessment",
		"org_id", orgID,
		"total_count", len(assessmentIDs),
	)

	if orgID == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "机构ID不能为空")
	}

	for _, id := range assessmentIDs {
		if err := s.ensureAssessmentInOrg(ctx, orgID, id); err != nil {
			l.Warnw("批量评估的机构范围校验失败",
				"assessment_id", id,
				"org_id", orgID,
				"error", err.Error(),
			)
			return nil, err
		}
	}

	result := &BatchResult{
		TotalCount:   len(assessmentIDs),
		SuccessCount: 0,
		FailedCount:  0,
		FailedIDs:    make([]uint64, 0),
	}

	for _, id := range assessmentIDs {
		if err := s.Evaluate(ctx, id); err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, id)
			l.Warnw("单个评估失败",
				"assessment_id", id,
				"error", err.Error(),
			)
		} else {
			result.SuccessCount++
		}
	}

	duration := time.Since(startTime)
	l.Infow("批量评估完成",
		"action", "evaluate_batch",
		"resource", "assessment",
		"result", "success",
		"total_count", result.TotalCount,
		"success_count", result.SuccessCount,
		"failed_count", result.FailedCount,
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

func (s *service) ensureAssessmentInOrg(ctx context.Context, orgID int64, assessmentID uint64) error {
	id := meta.FromUint64(assessmentID)
	a, err := s.assessmentRepo.FindByID(ctx, id)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	if a.OrgID() != orgID {
		return errors.WithCode(errorCode.ErrPermissionDenied, "测评不属于当前机构")
	}

	return nil
}

// markAsFailed 标记测评为失败
func (s *service) markAsFailed(ctx context.Context, a *assessment.Assessment, reason string) {
	l := logger.L(ctx)

	l.Warnw("标记测评为失败",
		"assessment_id", a.ID().Uint64(),
		"reason", reason,
		"action", "mark_failed",
	)

	if err := a.MarkAsFailed(reason); err != nil {
		l.Warnw("failed to transition assessment to failed",
			"assessment_id", a.ID().Uint64(),
			"error", err.Error(),
		)
		return
	}
	if err := s.saveAssessmentWithEvents(ctx, a); err != nil {
		l.Warnw("failed to persist failed assessment with outbox",
			"assessment_id", a.ID().Uint64(),
			"error", err.Error(),
		)
	}
}

func (s *service) saveAssessmentWithEvents(ctx context.Context, a *assessment.Assessment) error {
	if s.txRunner == nil || s.eventStager == nil {
		return errors.WithCode(errorCode.ErrModuleInitializationFailed, "assessment engine transactional outbox requires transaction runner and event stager")
	}
	if a == nil {
		return nil
	}
	err := s.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.assessmentRepo.Save(txCtx, a); err != nil {
			return err
		}
		eventsToStage := a.Events()
		if len(eventsToStage) == 0 {
			return nil
		}
		return s.eventStager.Stage(txCtx, eventsToStage...)
	})
	if err != nil {
		return err
	}
	a.ClearEvents()
	return nil
}
