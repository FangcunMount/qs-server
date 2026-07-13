package execute

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

	if a.Status().IsEvaluated() {
		log.Infow("测评已有成功评估事实，跳过重复评估",
			"assessment_id", assessmentID,
			"status", a.Status().String(),
			"result", "duplicate_skipped",
		)
		return &loadedAssessment{assessment: a, skipEvaluation: true}, nil
	}

	// A failed Assessment may be reopened only after the engine proves that its
	// latest Run is retryable and atomically claims the next attempt.
	if a.Status().IsFailed() {
		return &loadedAssessment{assessment: a}, nil
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
		return evalerrors.InvalidArgument("评估模型不可用")
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
	repo        assessment.Repository
	runRepo     evaluationrun.Repository
	txRunner    apptransaction.Runner
	eventStager EventStager
	postCommit  appEventing.PostCommitDispatcher
}

// Finalize atomically persists the Assessment failure, terminal Run and failure event.
func (f evaluationFailureFinalizer) Finalize(
	ctx context.Context,
	a *assessment.Assessment,
	run *evalrun.EvaluationRun,
	reason string,
	failure evalrun.Failure,
) error {
	if f.repo == nil || f.runRepo == nil || f.txRunner == nil || f.eventStager == nil {
		return evalerrors.ModuleNotConfigured("evaluation failure finalizer requires transaction, assessment, run and outbox dependencies")
	}
	if a == nil || run == nil {
		return fmt.Errorf("assessment and evaluation run are required")
	}
	if run.AssessmentID() != a.ID().Uint64() {
		return fmt.Errorf("evaluation run assessment does not match failure assessment")
	}
	log := logger.L(ctx)

	log.Warnw("标记测评为失败",
		"assessment_id", a.ID().Uint64(),
		"reason", reason,
		"action", "mark_failed",
	)

	failedAt := time.Now()
	assessmentToCommit, err := a.PrepareFailure(reason, failedAt)
	if err != nil {
		log.Warnw("failed to transition assessment to failed",
			"assessment_id", a.ID().Uint64(),
			"error", err.Error(),
		)
		return err
	}
	runToCommit := *run
	if err := runToCommit.Fail(failedAt, failure); err != nil {
		return err
	}
	eventsToStage := assessmentToCommit.Events()
	if err := f.txRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := f.repo.Save(txCtx, assessmentToCommit); err != nil {
			return err
		}
		if err := f.runRepo.SaveClaimed(txCtx, runToCommit); err != nil {
			return err
		}
		if len(eventsToStage) > 0 {
			return f.eventStager.Stage(txCtx, eventsToStage...)
		}
		return nil
	}); err != nil {
		log.Warnw("failed to persist failed assessment, run and outbox",
			"assessment_id", a.ID().Uint64(),
			"error", err.Error(),
		)
		return err
	}
	assessmentToCommit.ClearEvents()
	*a = *assessmentToCommit
	*run = runToCommit
	if f.postCommit != nil && len(eventsToStage) > 0 {
		f.postCommit.AfterCommit(ctx, eventsToStage, time.Now())
	}
	return nil
}
