package assessment

import (
	"context"
	"time"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
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
	req domainAssessment.CreateAssessmentRequest,
	dto CreateAssessmentDTO,
) error {
	occurredAt := time.Now()
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, func(saved *domainAssessment.Assessment) []event.DomainEvent {
		return []event.DomainEvent{
			domainStatistics.NewFootprintAssessmentCreatedEvent(
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
	myAssessmentListCacheHelper{cache: f.cache}.Invalidate(ctx, testeeID)
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
	myAssessmentListCacheHelper{cache: f.cache}.Invalidate(ctx, testeeID)
}
