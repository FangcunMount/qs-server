package questionnaire

import (
	"context"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
)

type questionnaireRepositoryReadModel struct {
	repo domainQuestionnaire.Repository
}

func (r questionnaireRepositoryReadModel) ListQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	items, err := r.repo.FindBaseList(ctx, page.Page, page.PageSize, questionnaireFilterToConditions(filter))
	if err != nil {
		return nil, err
	}
	return questionnaireRowsFromDomain(items), nil
}

func (r questionnaireRepositoryReadModel) CountQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter) (int64, error) {
	return r.repo.CountWithConditions(ctx, questionnaireFilterToConditions(filter))
}

func (r questionnaireRepositoryReadModel) ListPublishedQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	items, err := r.repo.FindBasePublishedList(ctx, page.Page, page.PageSize, questionnaireFilterToConditions(filter))
	if err != nil {
		return nil, err
	}
	return questionnaireRowsFromDomain(items), nil
}

func (r questionnaireRepositoryReadModel) CountPublishedQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter) (int64, error) {
	return r.repo.CountPublishedWithConditions(ctx, questionnaireFilterToConditions(filter))
}

func questionnaireFilterToConditions(filter surveyreadmodel.QuestionnaireFilter) map[string]interface{} {
	conditions := make(map[string]interface{})
	if filter.Status != "" {
		conditions["status"] = filter.Status
	}
	if filter.Title != "" {
		conditions["title"] = filter.Title
	}
	if filter.Type != "" {
		conditions["type"] = filter.Type
	}
	return conditions
}

func questionnaireRowsFromDomain(items []*domainQuestionnaire.Questionnaire) []surveyreadmodel.QuestionnaireSummaryRow {
	rows := make([]surveyreadmodel.QuestionnaireSummaryRow, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		rows = append(rows, surveyreadmodel.QuestionnaireSummaryRow{
			Code:          item.GetCode().String(),
			Version:       item.GetVersion().String(),
			Title:         item.GetTitle(),
			Description:   item.GetDescription(),
			ImgURL:        item.GetImgUrl(),
			Status:        item.GetStatus().String(),
			Type:          item.GetType().String(),
			QuestionCount: item.GetQuestionCnt(),
			CreatedBy:     item.GetCreatedBy(),
			CreatedAt:     item.GetCreatedAt(),
			UpdatedBy:     item.GetUpdatedBy(),
			UpdatedAt:     item.GetUpdatedAt(),
		})
	}
	return rows
}
