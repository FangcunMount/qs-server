package engine

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine/pipeline"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type EvaluationPipelineRunner interface {
	Execute(ctx context.Context, evalCtx *pipeline.Context) error
}

// service 评估引擎服务实现
type service struct {
	// 仓储依赖
	assessmentRepo assessment.Repository
	inputResolver  evaluationinput.Resolver

	txRunner    apptransaction.Runner
	eventStager EventStager

	// 处理器链
	pipelineRunner EvaluationPipelineRunner
	evaluators     EvaluatorRegistry
}

type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// ServiceOption 服务选项
type ServiceOption func(*service)

func WithTransactionalOutbox(txRunner apptransaction.Runner, eventStager EventStager) ServiceOption {
	return func(s *service) {
		s.txRunner = txRunner
		s.eventStager = eventStager
	}
}

func WithEvaluatorRegistry(registry EvaluatorRegistry) ServiceOption {
	return func(s *service) {
		s.evaluators = registry
	}
}

// NewService 创建评估引擎服务
func NewService(
	assessmentRepo assessment.Repository,
	inputResolver evaluationinput.Resolver,
	pipelineRunner EvaluationPipelineRunner,
	opts ...ServiceOption,
) Service {
	registry, _ := NewEvaluatorRegistry()
	svc := &service{
		assessmentRepo: assessmentRepo,
		inputResolver:  inputResolver,
		pipelineRunner: pipelineRunner,
		evaluators:     registry,
	}

	// 应用选项
	for _, opt := range opts {
		opt(svc)
	}

	return svc
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
		return evalerrors.InvalidArgument("测评ID不能为空")
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

	modelKind := resolveEvaluationModelKind(a, input)

	l.Infow("开始执行评估解释器",
		"assessment_id", assessmentID,
		"model_kind", modelKind.String(),
		"model_code", evaluationModelCode(a, input),
	)

	if s.evaluators == nil {
		err := evalerrors.ModuleNotConfigured("evaluation evaluator registry is not configured")
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}
	evaluator, err := s.evaluators.Resolve(modelKind)
	if err != nil {
		l.Errorw("评估解释器解析失败",
			"assessment_id", assessmentID,
			"model_kind", modelKind.String(),
			"result", "failed",
			"error", err.Error(),
		)
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}
	if err := evaluator.Evaluate(ctx, ExecutionInput{Assessment: a, Input: input}); err != nil {
		l.Errorw("评估解释器执行失败",
			"assessment_id", assessmentID,
			"model_kind", modelKind.String(),
			"model_code", evaluationModelCode(a, input),
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
		"model_kind", modelKind.String(),
		"model_code", evaluationModelCode(a, input),
		"duration_ms", duration.Milliseconds(),
	)

	return nil
}

func resolveEvaluationModelKind(a *assessment.Assessment, input *evaluationinput.InputSnapshot) assessment.EvaluationModelKind {
	if input != nil && input.Model != nil && input.Model.Kind != "" {
		return assessment.EvaluationModelKind(input.Model.Kind)
	}
	if a != nil && a.EvaluationModelRef() != nil {
		return a.EvaluationModelRef().Kind()
	}
	return ""
}

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
