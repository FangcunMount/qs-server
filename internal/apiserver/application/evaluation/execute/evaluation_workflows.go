package execute

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evaluationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// loadedAssessment 加载的评估数据
type loadedAssessment struct {
	assessment     *assessment.Assessment
	skipEvaluation bool
}

// assessmentLoader 评估数据加载器
type assessmentLoader struct {
	repo assessment.Repository
}

// LoadForEvaluation 加载评估数据
func (l assessmentLoader) LoadForEvaluation(ctx context.Context, assessmentID uint64) (*loadedAssessment, error) {
	log := logger.L(ctx)
	log.Debugw("加载测评数据",
		"assessment_id", assessmentID,
		"action", "read",
	)

	a, err := l.repo.FindByID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		log.Errorw("加载测评数据失败",
			"assessment_id", assessmentID,
			"action", "read",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}

	log.Debugw("测评数据加载成功",
		"assessment_id", assessmentID,
		"status", a.Status().String(),
		"result", "success",
	)

	if a.Status().IsInterpreted() {
		log.Infow("测评已解读，跳过重复评估",
			"assessment_id", assessmentID,
			"status", a.Status().String(),
			"result", "duplicate_skipped",
		)
		return &loadedAssessment{assessment: a, skipEvaluation: true}, nil
	}

	if !a.Status().IsSubmitted() {
		log.Warnw("测评状态不正确",
			"assessment_id", assessmentID,
			"status", a.Status().String(),
			"expected_status", "submitted",
			"result", "failed",
		)
		return nil, evalerrors.AssessmentInvalidStatus("测评状态不正确，无法评估")
	}

	if !a.NeedsEvaluation() {
		log.Infow("纯问卷模式，跳过评估",
			"assessment_id", assessmentID,
			"mode", "questionnaire_only",
			"result", "skipped",
		)
		return &loadedAssessment{assessment: a, skipEvaluation: true}, nil
	}

	return &loadedAssessment{assessment: a}, nil
}

// LoadForInterpretation loads an assessment ready for report generation.
func (l assessmentLoader) LoadForInterpretation(ctx context.Context, assessmentID uint64) (*loadedAssessment, error) {
	log := logger.L(ctx)
	a, err := l.repo.FindByID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	if a.Status().IsInterpreted() {
		log.Infow("测评已解读，跳过重复报告生成",
			"assessment_id", assessmentID,
			"status", a.Status().String(),
		)
		return &loadedAssessment{assessment: a, skipEvaluation: true}, nil
	}
	if !a.Status().IsEvaluated() {
		return nil, evalerrors.AssessmentInvalidStatus("测评尚未计分，无法生成报告")
	}
	if !a.NeedsEvaluation() {
		return &loadedAssessment{assessment: a, skipEvaluation: true}, nil
	}
	return &loadedAssessment{assessment: a}, nil
}

// EnsureAssessmentInOrg 确保评估数据属于当前机构
func (l assessmentLoader) EnsureAssessmentInOrg(ctx context.Context, orgID int64, assessmentID uint64) error {
	a, err := l.repo.FindByID(ctx, meta.FromUint64(assessmentID))
	if err != nil {
		return evalerrors.AssessmentNotFound(err, "测评不存在")
	}

	if a.OrgID() != orgID {
		return evalerrors.PermissionDenied("测评不属于当前机构")
	}

	return nil
}

// evaluationInputWorkflow 评估输入解析器
type evaluationInputWorkflow struct {
	resolver evaluationinput.Resolver
}

// Resolve 解析评估输入
func (w evaluationInputWorkflow) Resolve(ctx context.Context, a *assessment.Assessment, assessmentID uint64) (*evaluationinput.InputSnapshot, error) {
	if w.resolver == nil {
		return nil, evalerrors.ModuleNotConfigured("evaluation input resolver is not configured")
	}
	// 解析评估输入
	snapshot, err := w.resolver.Resolve(ctx, inputRefFromAssessment(a, assessmentID))
	if err != nil {
		return nil, mapInputResolveError(err)
	}
	return snapshot, nil
}

// mapInputResolveError 映射评估输入解析错误
func mapInputResolveError(err error) error {
	var carrier evaluationinput.FailureKindCarrier
	if !stderrors.As(err, &carrier) {
		return err
	}

	switch carrier.FailureKind() {
	case evaluationinput.FailureKindModelNotFound, evaluationinput.FailureKindUnsupportedModel:
		return evalerrors.InvalidArgument("解释模型不可用")
	case evaluationinput.FailureKindScaleNotFound:
		return mapScaleInputResolveError(err)
	case evaluationinput.FailureKindAnswerSheetNotFound:
		return evalerrors.AnswerSheetNotFound(err, "答卷不存在")
	case evaluationinput.FailureKindQuestionnaireNotFound:
		return evalerrors.QuestionnaireNotFound(err, "加载问卷失败")
	case evaluationinput.FailureKindQuestionnaireVersionMismatch:
		return evalerrors.QuestionnaireNotFound(err, "问卷不存在或版本不匹配")
	default:
		return err
	}
}

// inputResolveFailureReason 映射评估输入解析失败原因
func inputResolveFailureReason(err error) string {
	var carrier evaluationinput.FailureReasonCarrier
	if stderrors.As(err, &carrier) {
		return carrier.FailureReason()
	}
	return "评估输入加载失败: " + err.Error()
}

// evaluationFailureFinalizer 评估失败标记器
type evaluationFailureFinalizer struct {
	repo         assessment.Repository
	txRunner     apptransaction.Runner
	eventStager  EventStager
	reportStatus *reportstatus.Reporter
	readyIndexer *appEventing.PostCommitReadyIndexer
}

// MarkAsFailed 标记评估失败
func (f evaluationFailureFinalizer) MarkAsFailed(ctx context.Context, a *assessment.Assessment, reason string) {
	log := logger.L(ctx)

	log.Warnw("标记测评为失败",
		"assessment_id", a.ID().Uint64(),
		"reason", reason,
		"action", "mark_failed",
	)

	if err := a.MarkAsFailed(reason); err != nil {
		log.Warnw("failed to transition assessment to failed",
			"assessment_id", a.ID().Uint64(),
			"error", err.Error(),
		)
		return
	}
	if err := f.SaveAssessmentWithEvents(ctx, a); err != nil {
		log.Warnw("failed to persist failed assessment with outbox",
			"assessment_id", a.ID().Uint64(),
			"error", err.Error(),
		)
	}
	if f.reportStatus != nil {
		assessmentID, answerSheetID := evaluationapp.ReportStatusIDs(a)
		f.reportStatus.SetFailed(ctx, assessmentID, answerSheetID, "evaluation_failed", reason)
	}
}

// SaveAssessmentWithEvents 保存评估数据和事件
func (f evaluationFailureFinalizer) SaveAssessmentWithEvents(ctx context.Context, a *assessment.Assessment) error {
	if f.txRunner == nil || f.eventStager == nil {
		return evalerrors.ModuleNotConfigured("assessment engine transactional outbox requires transaction runner and event stager")
	}
	if a == nil {
		return nil
	}
	var stagedEvents []event.DomainEvent
	err := f.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := f.repo.Save(txCtx, a); err != nil {
			return err
		}
		eventsToStage := a.Events()
		if len(eventsToStage) == 0 {
			return nil
		}
		stagedEvents = eventsToStage
		return f.eventStager.Stage(txCtx, eventsToStage...)
	})
	if err != nil {
		return err
	}
	if f.readyIndexer != nil && len(stagedEvents) > 0 {
		f.readyIndexer.EnqueueAfterCommit(ctx, stagedEvents, time.Now())
	}
	a.ClearEvents()
	return nil
}

// batchEvaluator 批量评估器
type batchEvaluator struct {
	loader   assessmentLoader
	evaluate func(ctx context.Context, assessmentID uint64) error
}

// EvaluateBatch 批量评估
func (b batchEvaluator) EvaluateBatch(ctx context.Context, orgID int64, assessmentIDs []uint64) (*BatchResult, error) {
	log := logger.L(ctx)
	startTime := time.Now()

	log.Infow("开始批量评估",
		"action", "evaluate_batch",
		"resource", "assessment",
		"org_id", orgID,
		"total_count", len(assessmentIDs),
	)

	if orgID == 0 {
		return nil, evalerrors.InvalidArgument("机构ID不能为空")
	}

	// 确保评估数据属于当前机构
	for _, id := range assessmentIDs {
		if err := b.loader.EnsureAssessmentInOrg(ctx, orgID, id); err != nil {
			log.Warnw("批量评估的机构范围校验失败",
				"assessment_id", id,
				"org_id", orgID,
				"error", err.Error(),
			)
			return nil, err
		}
	}

	// 批量评估
	result := &BatchResult{
		TotalCount:   len(assessmentIDs),
		SuccessCount: 0,
		FailedCount:  0,
		FailedIDs:    make([]uint64, 0),
	}

	// 执行单次评估
	for _, id := range assessmentIDs {
		if err := b.evaluate(ctx, id); err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, id)
			log.Warnw("单个评估失败",
				"assessment_id", id,
				"error", err.Error(),
			)
		} else {
			result.SuccessCount++
		}
	}

	log.Infow("批量评估完成",
		"action", "evaluate_batch",
		"resource", "assessment",
		"result", "success",
		"total_count", result.TotalCount,
		"success_count", result.SuccessCount,
		"failed_count", result.FailedCount,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return result, nil
}
