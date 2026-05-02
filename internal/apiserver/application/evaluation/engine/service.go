package engine

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine/pipeline"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
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

// NewService 创建评估引擎服务
func NewService(
	assessmentRepo assessment.Repository,
	inputResolver evaluationinput.Resolver,
	pipelineRunner EvaluationPipelineRunner,
	opts ...ServiceOption,
) Service {
	svc := &service{
		assessmentRepo: assessmentRepo,
		inputResolver:  inputResolver,
		pipelineRunner: pipelineRunner,
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

	if s.pipelineRunner == nil {
		err := errors.WithCode(errorCode.ErrModuleInitializationFailed, "evaluation pipeline runner is not configured")
		s.failureFinalizer().MarkAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}
	if err := s.pipelineRunner.Execute(ctx, evalCtx); err != nil {
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
