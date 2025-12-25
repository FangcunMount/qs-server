package engine

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// service 评估引擎服务实现
type service struct {
	// 仓储依赖
	assessmentRepo    assessment.Repository
	scoreRepo         assessment.ScoreRepository
	reportRepo        report.ReportRepository
	scaleRepo         scale.Repository
	answerSheetRepo   answersheet.Repository
	questionnaireRepo questionnaire.Repository

	// 领域服务依赖
	reportBuilder report.ReportBuilder

	// 事件发布器（可选）
	eventPublisher event.EventPublisher

	// 等待队列注册表（可选，用于长轮询）
	waiterRegistry *waiter.WaiterRegistry

	// 处理器链
	pipeline *pipeline.Chain
}

// ServiceOption 服务选项
type ServiceOption func(*service)

// WithEventPublisher 设置事件发布器
func WithEventPublisher(publisher event.EventPublisher) ServiceOption {
	return func(s *service) {
		s.eventPublisher = publisher
	}
}

// WithWaiterRegistry 设置等待队列注册表
func WithWaiterRegistry(waiterRegistry *waiter.WaiterRegistry) ServiceOption {
	return func(s *service) {
		s.waiterRegistry = waiterRegistry
	}
}

// NewService 创建评估引擎服务
func NewService(
	assessmentRepo assessment.Repository,
	scoreRepo assessment.ScoreRepository,
	reportRepo report.ReportRepository,
	scaleRepo scale.Repository,
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
	reportBuilder report.ReportBuilder,
	opts ...ServiceOption,
) Service {
	svc := &service{
		assessmentRepo:    assessmentRepo,
		scoreRepo:         scoreRepo,
		reportRepo:        reportRepo,
		scaleRepo:         scaleRepo,
		answerSheetRepo:   answerSheetRepo,
		questionnaireRepo: questionnaireRepo,
		reportBuilder:     reportBuilder,
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
	chain.AddHandler(pipeline.NewFactorScoreHandler())

	// 3. 风险等级计算处理器（计算风险等级，保存分数）
	chain.AddHandler(pipeline.NewRiskLevelHandler(s.scoreRepo))

	// 4. 测评分析解读处理器
	chain.AddHandler(pipeline.NewInterpretationHandler(s.assessmentRepo, s.reportRepo, s.reportBuilder))

	// 5. 事件发布处理器
	// 注意：如果未设置 eventPublisher，则不会发布 assessment.interpreted 事件到消息队列
	// 但领域事件仍会添加到聚合根，供仓储层使用
	eventPublishOpts := []pipeline.EventPublishHandlerOption{}
	if s.waiterRegistry != nil {
		eventPublishOpts = append(eventPublishOpts, pipeline.WithWaiterRegistry(s.waiterRegistry))
	}
	chain.AddHandler(pipeline.NewEventPublishHandler(s.eventPublisher, eventPublishOpts...))

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

	// 2. 加载 MedicalScale
	scaleCode := a.MedicalScaleRef().Code().String()
	l.Debugw("加载量表数据",
		"scale_code", scaleCode,
		"action", "read",
		"resource", "scale",
	)

	medicalScale, err := s.scaleRepo.FindByCode(ctx, scaleCode)
	if err != nil {
		l.Errorw("加载量表失败",
			"scale_code", scaleCode,
			"action", "read",
			"result", "failed",
			"error", err.Error(),
		)
		s.markAsFailed(ctx, a, "加载量表失败: "+err.Error())
		return errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "量表不存在")
	}

	l.Debugw("量表数据加载成功",
		"scale_code", scaleCode,
		"scale_title", medicalScale.GetTitle(),
		"result", "success",
	)

	// 3. 加载答卷数据
	answerSheetID := a.AnswerSheetRef().ID()
	l.Debugw("加载答卷数据",
		"answer_sheet_id", answerSheetID,
		"action", "read",
		"resource", "answersheet",
	)

	answerSheet, err := s.answerSheetRepo.FindByID(ctx, answerSheetID)
	if err != nil {
		l.Errorw("加载答卷失败",
			"answer_sheet_id", answerSheetID,
			"action", "evaluate_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		s.markAsFailed(ctx, a, "加载答卷失败: "+err.Error())
		return errors.WrapC(err, errorCode.ErrAnswerSheetNotFound, "答卷不存在")
	}

	l.Debugw("答卷数据加载成功",
		"answer_sheet_id", answerSheetID,
		"questionnaire_code", func() string { code, _, _ := answerSheet.QuestionnaireInfo(); return code }(),
		"result", "success",
	)

	// 4. 加载问卷数据（用于 cnt 等计分规则）
	qCode, qVersion, _ := answerSheet.QuestionnaireInfo()
	l.Debugw("加载问卷数据",
		"questionnaire_code", qCode,
		"questionnaire_version", qVersion,
		"action", "read",
		"resource", "questionnaire",
	)

	qnr, err := s.questionnaireRepo.FindByCodeVersion(ctx, qCode, qVersion)
	if err != nil {
		l.Warnw("加载问卷失败，将使用降级计分策略",
			"questionnaire_code", qCode,
			"questionnaire_version", qVersion,
			"error", err.Error(),
		)
		// 问卷加载失败不影响评估流程，只是无法使用 cnt 等高级计分规则
		qnr = nil
	} else {
		l.Debugw("问卷数据加载成功",
			"questionnaire_code", qCode,
			"question_count", len(qnr.GetQuestions()),
			"result", "success",
		)
	}

	// 5. 创建评估上下文
	evalCtx := pipeline.NewContext(a, medicalScale, answerSheet)
	evalCtx.Questionnaire = qnr // 设置问卷数据

	// 6. 执行处理器链
	l.Infow("开始执行评估处理器链",
		"assessment_id", assessmentID,
		"scale_code", scaleCode,
	)

	if err := s.pipeline.Execute(ctx, evalCtx); err != nil {
		l.Errorw("评估处理器链执行失败",
			"assessment_id", assessmentID,
			"scale_code", scaleCode,
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
		"scale_code", scaleCode,
		"duration_ms", duration.Milliseconds(),
	)

	return nil
}

// EvaluateBatch 批量评估
func (s *service) EvaluateBatch(ctx context.Context, assessmentIDs []uint64) (*BatchResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Infow("开始批量评估",
		"action", "evaluate_batch",
		"resource", "assessment",
		"total_count", len(assessmentIDs),
	)

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

// markAsFailed 标记测评为失败
func (s *service) markAsFailed(ctx context.Context, a *assessment.Assessment, reason string) {
	l := logger.L(ctx)

	l.Warnw("标记测评为失败",
		"assessment_id", a.ID().Uint64(),
		"reason", reason,
		"action", "mark_failed",
	)

	_ = a.MarkAsFailed(reason)
	_ = s.assessmentRepo.Save(ctx, a)
}
