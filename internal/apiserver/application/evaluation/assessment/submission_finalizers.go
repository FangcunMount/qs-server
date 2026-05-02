package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type assessmentCreateFinalizer struct {
	repo        domainAssessment.Repository
	txRunner    apptransaction.Runner
	eventStager EventStager
	cache       assessmentListCache
}

func (f assessmentCreateFinalizer) SaveAndStage(
	ctx context.Context,
	a *domainAssessment.Assessment,
	req domainAssessment.CreateAssessmentRequest,
	dto CreateAssessmentDTO,
) error {
	occurredAt := time.Now()
	additionalEvents := []event.DomainEvent{
		domainStatistics.NewFootprintAssessmentCreatedEvent(req.OrgID, dto.TesteeID, dto.AnswerSheetID, a.ID().Uint64(), occurredAt),
	}
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, additionalEvents); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}
	return nil
}

func (f assessmentCreateFinalizer) InvalidateCache(ctx context.Context, testeeID uint64) {
	myAssessmentListCacheHelper{cache: f.cache}.Invalidate(ctx, testeeID)
}

type assessmentSubmitFinalizer struct {
	repo        domainAssessment.Repository
	txRunner    apptransaction.Runner
	eventStager EventStager
	cache       assessmentListCache
}

func (f assessmentSubmitFinalizer) SaveAndStage(ctx context.Context, a *domainAssessment.Assessment) error {
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, nil); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}
	return nil
}

func (f assessmentSubmitFinalizer) InvalidateCache(ctx context.Context, testeeID uint64) {
	myAssessmentListCacheHelper{cache: f.cache}.Invalidate(ctx, testeeID)
}
