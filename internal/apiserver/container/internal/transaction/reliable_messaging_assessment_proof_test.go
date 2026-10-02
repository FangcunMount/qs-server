//go:build reliable_messaging

package transaction

import (
	"context"

	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"

	intake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

type proofModelValidator struct{}

func (proofModelValidator) ValidateEvaluationModel(context.Context, domain.EvaluationModelRef, domain.QuestionnaireRef, intake.ModelValidationMode) error {
	return nil
}

type proofPendingBarrier struct {
	domain.Repository
	arrived chan struct{}
	release chan struct{}
}

func (r *proofPendingBarrier) FindByID(ctx context.Context, id domain.ID) (*domain.Assessment, error) {
	item, err := r.Repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	select {
	case r.arrived <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-r.release:
		return item, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *proofPendingBarrier) SavePendingSubmission(ctx context.Context, a *domain.Assessment) error {
	return r.Repository.(domain.PendingSubmissionRepository).SavePendingSubmission(ctx, a)
}

type proofSubmissionReader struct{ value *sheet.AnswerSheet }

func (r proofSubmissionReader) FindByID(context.Context, meta.ID) (*sheet.AnswerSheet, error) {
	return r.value, nil
}
