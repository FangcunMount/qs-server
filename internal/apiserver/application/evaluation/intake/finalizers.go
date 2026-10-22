package intake

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// assessmentCreateFinalizer 测评创建最终化器
type assessmentCreateFinalizer struct {
	repo        domainAssessment.Repository
	txRunner    apptransaction.Runner
	eventStager EventStager
	postCommit  appEventing.PostCommitDispatcher
}

// SaveAndStage 保存并阶段测评
func (f assessmentCreateFinalizer) SaveAndStage(
	ctx context.Context,
	a *domainAssessment.Assessment,
	req assessmentCreateSpec,
	dto CreateCommand,
) error {
	if err := saveAssessmentAndStageEvents(ctx, f.repo.Save, f.txRunner, f.eventStager, a, f.postCommit); err != nil {
		return evalerrors.Database(err, "保存测评失败")
	}
	return nil
}

// assessmentSubmitFinalizer 测评提交最终化器
type assessmentSubmitFinalizer struct {
	repo        domainAssessment.Repository
	txRunner    apptransaction.Runner
	eventStager EventStager
	postCommit  appEventing.PostCommitDispatcher
}

// SaveAndStage 保存并阶段测评
func (f assessmentSubmitFinalizer) SaveAndStage(ctx context.Context, a *domainAssessment.Assessment) error {
	submittedAt := a.SubmittedAt()
	if submittedAt == nil {
		return evalerrors.AssessmentSubmitFailed(domainAssessment.ErrInvalidArgument, "测评提交时间为空")
	}
	submitter, ok := f.repo.(domainAssessment.PendingSubmissionRepository)
	if !ok {
		return evalerrors.ModuleNotConfigured("assessment pending submission repository is required")
	}
	if err := saveAssessmentAndStageEvents(ctx, submitter.SavePendingSubmission, f.txRunner, f.eventStager, a, f.postCommit); err != nil {
		return evalerrors.Database(err, "保存测评失败")
	}
	return nil
}

// SavePending stores an eventless draft on the original local business root.
// It is selected only with an explicit transaction submitter. A future domain
// event cannot silently disappear through this creation path.
func (f assessmentCreateFinalizer) SavePending(ctx context.Context, a *domainAssessment.Assessment) error {
	if f.repo == nil || f.txRunner == nil || a == nil || len(a.Events()) != 0 {
		return evalerrors.ModuleNotConfigured("eventless pending assessment and original transaction runner required")
	}
	return f.txRunner.WithinTransaction(ctx, func(root context.Context) error { return f.repo.Save(root, a) })
}
