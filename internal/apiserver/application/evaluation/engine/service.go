package engine

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// service 评估引擎服务实现
type service struct {
	// 仓储依赖
	assessmentRepo assessment.Repository
	scoreRepo      assessment.ScoreRepository
	reportRepo     report.ReportRepository
	scaleRepo      scale.Repository

	// 领域服务依赖
	reportBuilder report.ReportBuilder

	// 处理器链
	pipeline *pipeline.Chain
}

// NewService 创建评估引擎服务
func NewService(
	assessmentRepo assessment.Repository,
	scoreRepo assessment.ScoreRepository,
	reportRepo report.ReportRepository,
	scaleRepo scale.Repository,
	reportBuilder report.ReportBuilder,
) Service {
	svc := &service{
		assessmentRepo: assessmentRepo,
		scoreRepo:      scoreRepo,
		reportRepo:     reportRepo,
		scaleRepo:      scaleRepo,
		reportBuilder:  reportBuilder,
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

	// 2. 答卷分数计算处理器
	chain.AddHandler(pipeline.NewAnswerSheetScoreHandler())

	// 3. 测评分数计算处理器
	chain.AddHandler(pipeline.NewAssessmentScoreHandler(s.scoreRepo))

	// 4. 测评分析解读处理器
	chain.AddHandler(pipeline.NewInterpretationHandler(s.assessmentRepo, s.reportRepo, s.reportBuilder))

	// 5. 事件发布处理器
	// 注意：publisher 可以通过依赖注入传入，当前为 nil 表示不发布到消息队列
	// 领域事件已在 Assessment.ApplyEvaluation 中添加到聚合根
	chain.AddHandler(pipeline.NewEventPublishHandler(nil))

	return chain
}

// Evaluate 执行评估
func (s *service) Evaluate(ctx context.Context, assessmentID uint64) error {
	// 1. 加载 Assessment
	id := meta.FromUint64(assessmentID)
	a, err := s.assessmentRepo.FindByID(ctx, id)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 检查状态
	if !a.Status().IsSubmitted() {
		return errors.WithCode(errorCode.ErrAssessmentInvalidStatus, "测评状态不正确，无法评估")
	}

	// 检查是否有关联量表（纯问卷模式不需要评估）
	if a.MedicalScaleRef() == nil {
		// 纯问卷模式，直接标记为已解读（无需计分）
		// 注意：需要在领域层添加纯问卷模式的完成方法
		return nil
	}

	// 2. 加载 MedicalScale
	scaleCode := a.MedicalScaleRef().Code().String()
	medicalScale, err := s.scaleRepo.FindByCode(ctx, scaleCode)
	if err != nil {
		s.markAsFailed(ctx, a, "加载量表失败: "+err.Error())
		return errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "量表不存在")
	}

	// 3. 创建评估上下文
	// TODO: 加载答卷数据，当前传 nil，由 AnswerSheetScoreHandler 模拟得分
	evalCtx := pipeline.NewContext(a, medicalScale, nil)

	// 4. 执行处理器链
	if err := s.pipeline.Execute(ctx, evalCtx); err != nil {
		s.markAsFailed(ctx, a, "评估流程执行失败: "+err.Error())
		return err
	}

	return nil
}

// EvaluateBatch 批量评估
func (s *service) EvaluateBatch(ctx context.Context, assessmentIDs []uint64) (*BatchResult, error) {
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
		} else {
			result.SuccessCount++
		}
	}

	return result, nil
}

// markAsFailed 标记测评为失败
func (s *service) markAsFailed(ctx context.Context, a *assessment.Assessment, reason string) {
	_ = a.MarkAsFailed(reason)
	_ = s.assessmentRepo.Save(ctx, a)
}
