package intake

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/footprintevent"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// assessmentCreateFinalizer 测评创建最终化器
type assessmentCreateFinalizer struct {
	repo        domainAssessment.Repository
	txRunner    apptransaction.Runner
	eventStager EventStager
	cache       assessmentListCache
	immediate   *appEventing.ImmediateDispatcher
}

// SaveAndStage 保存并阶段测评
func (f assessmentCreateFinalizer) SaveAndStage(
	ctx context.Context,
	a *domainAssessment.Assessment,
	req assessmentCreateSpec,
	dto CreateCommand,
) error {
	occurredAt := time.Now()
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, func(saved *domainAssessment.Assessment) []event.DomainEvent {
		return []event.DomainEvent{
			footprintevent.NewFootprintAssessmentCreatedEvent(
				req.OrgID,
				dto.TesteeID,
				dto.AnswerSheetID,
				saved.ID().Uint64(),
				occurredAt,
			),
		}
	}, f.immediate); err != nil {
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
	repo        domainAssessment.Repository
	txRunner    apptransaction.Runner
	eventStager EventStager
	cache       assessmentListCache
	immediate   *appEventing.ImmediateDispatcher
}

// SaveAndStage 保存并阶段测评
func (f assessmentSubmitFinalizer) SaveAndStage(ctx context.Context, a *domainAssessment.Assessment) error {
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, nil, f.immediate); err != nil {
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
