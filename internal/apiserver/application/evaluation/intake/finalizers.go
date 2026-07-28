package intake

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
)

// assessmentCreateFinalizer 测评创建最终化器
type assessmentCreateFinalizer struct {
	repo          domainAssessment.Repository
	txRunner      apptransaction.Runner
	eventStager   EventStager
	cache         assessmentListCache
	postCommit    appEventing.PostCommitDispatcher
	stageRecorder stageport.Recorder
}

// SaveAndStage 保存并阶段测评
func (f assessmentCreateFinalizer) SaveAndStage(
	ctx context.Context,
	a *domainAssessment.Assessment,
	req assessmentCreateSpec,
	dto CreateCommand,
) error {
	completion := stageport.Completion{Stage: stageport.StageAssessmentCreated, BusinessAt: a.CreatedAt(), ResourceType: "assessment", ResourceID: a.ID().String(), Payload: struct {
		AnswerSheetID string `json:"answersheet_id"`
	}{AnswerSheetID: a.AnswerSheetRef().ID().String()}}
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, f.postCommit, f.stageRecorder, completion); err != nil {
		return evalerrors.Database(err, "保存测评失败")
	}
	return nil
}

// InvalidateCache 失效缓存
func (f assessmentCreateFinalizer) InvalidateCache(ctx context.Context, testeeID uint64) {
	invalidateAssessmentListCache(ctx, f.cache, testeeID)
}

// assessmentSubmitFinalizer 测评提交最终化器
type assessmentSubmitFinalizer struct {
	repo          domainAssessment.Repository
	txRunner      apptransaction.Runner
	eventStager   EventStager
	cache         assessmentListCache
	postCommit    appEventing.PostCommitDispatcher
	stageRecorder stageport.Recorder
}

// SaveAndStage 保存并阶段测评
func (f assessmentSubmitFinalizer) SaveAndStage(ctx context.Context, a *domainAssessment.Assessment) error {
	submittedAt := a.SubmittedAt()
	if submittedAt == nil {
		return evalerrors.AssessmentSubmitFailed(domainAssessment.ErrInvalidArgument, "测评提交时间为空")
	}
	completion := stageport.Completion{Stage: stageport.StageAssessmentSubmitted, BusinessAt: *submittedAt, ResourceType: "assessment", ResourceID: a.ID().String(), Payload: struct {
		AssessmentID string `json:"assessment_id"`
	}{AssessmentID: a.ID().String()}}
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, f.postCommit, f.stageRecorder, completion); err != nil {
		return evalerrors.Database(err, "保存测评失败")
	}
	return nil
}

// InvalidateCache 失效缓存
func (f assessmentSubmitFinalizer) InvalidateCache(ctx context.Context, testeeID uint64) {
	invalidateAssessmentListCache(ctx, f.cache, testeeID)
}

func invalidateAssessmentListCache(ctx context.Context, cache assessmentListCache, testeeID uint64) {
	if cache == nil || testeeID == 0 {
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	if err := cache.Invalidate(cacheCtx, testeeID); err != nil {
		logger.L(ctx).Warnw("失效我的测评列表缓存失败", "action", "invalidate_my_assessment_list_cache", "user_id", testeeID, "duration_ms", time.Since(startedAt).Milliseconds(), "error", err.Error())
		return
	}
	if elapsed := time.Since(startedAt); elapsed > 200*time.Millisecond {
		logger.L(ctx).Warnw("失效我的测评列表缓存较慢", "action", "invalidate_my_assessment_list_cache", "user_id", testeeID, "duration_ms", elapsed.Milliseconds())
	}
}
