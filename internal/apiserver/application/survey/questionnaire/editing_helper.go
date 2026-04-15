package questionnaire

import (
	"context"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

func ensureEditableHead(ctx context.Context, repo domainQuestionnaire.Repository, q *domainQuestionnaire.Questionnaire) error {
	if q == nil || !q.IsPublished() {
		return nil
	}

	if err := repo.CreatePublishedSnapshot(ctx, q, true); err != nil {
		return err
	}

	versioning := domainQuestionnaire.Versioning{}
	return versioning.ForkDraftFromPublished(q)
}

func cloneQuestionnaireAsHead(q *domainQuestionnaire.Questionnaire) (*domainQuestionnaire.Questionnaire, error) {
	if q == nil {
		return nil, nil
	}

	questions := make([]domainQuestionnaire.Question, 0, len(q.GetQuestions()))
	questions = append(questions, q.GetQuestions()...)

	return domainQuestionnaire.NewQuestionnaire(
		q.GetCode(),
		q.GetTitle(),
		domainQuestionnaire.WithDesc(q.GetDescription()),
		domainQuestionnaire.WithImgUrl(q.GetImgUrl()),
		domainQuestionnaire.WithVersion(q.GetVersion()),
		domainQuestionnaire.WithStatus(q.GetStatus()),
		domainQuestionnaire.WithType(q.GetType()),
		domainQuestionnaire.WithQuestions(questions),
		domainQuestionnaire.WithCreatedBy(q.GetCreatedBy()),
		domainQuestionnaire.WithCreatedAt(q.GetCreatedAt()),
		domainQuestionnaire.WithUpdatedBy(q.GetUpdatedBy()),
		domainQuestionnaire.WithUpdatedAt(q.GetUpdatedAt()),
		domainQuestionnaire.WithRecordRole(domainQuestionnaire.RecordRoleHead),
		domainQuestionnaire.WithActivePublished(false),
	)
}
