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
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, f.postCommit); err != nil {
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
	if err := saveAssessmentAndStageEvents(ctx, f.repo, f.txRunner, f.eventStager, a, f.postCommit); err != nil {
		return evalerrors.Database(err, "保存测评失败")
	}
	return nil
}
