package execute

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// service 评估引擎服务实现
type service struct {
	// 仓储依赖
	assessmentRepo assessment.Repository
	inputResolver  evaluationinput.Resolver

	txRunner    apptransaction.Runner
	eventStager EventStager

	evaluators   EvaluatorRegistry
	resultWriter evaluationresult.Writer
}

// EventStager 事件暂存器
type EventStager interface {
	Stage(ctx context.Context, events ...event.DomainEvent) error
}

// ServiceOption 服务选项
type ServiceOption func(*service)

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

// NewService 创建评估引擎服务实例
func NewService(
	assessmentRepo assessment.Repository,
	inputResolver evaluationinput.Resolver,
	resultWriter evaluationresult.Writer,
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

	// 解析评估输入
	input, err := evaluationInputWorkflow{resolver: s.inputResolver}.Resolve(ctx, a, assessmentID)
	if err != nil {
		s.failureFinalizer().MarkAsFailed(ctx, a, inputResolveFailureReason(err))
		return err
	}

	// 解析评估模型类型
	modelKind := resolveEvaluationModelKind(a, input)

	l.Infow("开始执行评估解释器",
		"assessment_id", assessmentID,
		"model_kind", modelKind.String(),
		"model_code", evaluationModelCode(a, input),
	)

	// 解析评估模型执行器
	if s.evaluators == nil {
		err := evalerrors.ModuleNotConfigured("evaluation evaluator registry is not configured")
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	// 解析评估模型执行器
	evaluator, err := s.evaluators.Resolve(modelKind)
	if err != nil {
		l.Errorw("评估模型执行器解析失败",
			"assessment_id", assessmentID,
			"model_kind", modelKind.String(),
			"result", "failed",
			"error", err.Error(),
		)
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	// 执行评估模型
	evaluationResult, err := evaluator.Execute(ctx, ExecutionInput{Assessment: a, Input: input})
	if err != nil {
		l.Errorw("评估模型执行失败",
			"assessment_id", assessmentID,
			"model_kind", modelKind.String(),
			"model_code", evaluationModelCode(a, input),
			"result", "failed",
			"error", err.Error(),
		)
		// 标记评估流程执行失败
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	// 执行评估成功，写入评估结果和报告
	if s.resultWriter == nil {
		err := evalerrors.ModuleNotConfigured("evaluation result writer is not configured")
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}
	if err := s.resultWriter.Write(ctx, evaluationresult.Outcome{Assessment: a, Input: input, Result: evaluationResult}); err != nil {
		l.Errorw("评估结果写入失败",
			"assessment_id", assessmentID,
			"model_kind", modelKind.String(),
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
		"model_kind", modelKind.String(),
		"model_code", evaluationModelCode(a, input),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return nil
}

// resolveEvaluationModelKind 解析评估模型类型
func resolveEvaluationModelKind(a *assessment.Assessment, input *evaluationinput.InputSnapshot) assessment.EvaluationModelKind {
	if input != nil && input.Model != nil && input.Model.Kind != "" {
		return assessment.EvaluationModelKind(input.Model.Kind)
	}
	if a != nil && a.EvaluationModelRef() != nil {
		return a.EvaluationModelRef().Kind()
	}
	return ""
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
		repo:        s.assessmentRepo,
		txRunner:    s.txRunner,
		eventStager: s.eventStager,
	}
}
