package questionnaire

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
)

type questionnaireReadModel struct {
	repo *Repository
}

// NewQuestionnaireReadModel adapts questionnaire Mongo repositories to the read-model port.
func NewQuestionnaireReadModel(repo *Repository) surveyreadmodel.QuestionnaireReader {
	return questionnaireReadModel{repo: repo}
}

func (r questionnaireReadModel) ListQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	pipeline := buildHeadBasePipeline(
		buildHeadListFilter(questionnaireFilterToConditions(filter)),
		paginationSkip(page.Page, page.PageSize),
		paginationLimit(page.Page, page.PageSize),
	)
	items, err := r.repo.aggregateList(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	return questionnaireRowsFromDomain(items), nil
}

func (r questionnaireReadModel) CountQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter) (int64, error) {
	return r.repo.CountDocuments(ctx, buildHeadListFilter(questionnaireFilterToConditions(filter)))
}

func (r questionnaireReadModel) ListPublishedQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	pipeline := buildPublishedBasePipeline(
		buildPublishedListFilter(questionnaireFilterToConditions(filter)),
		paginationSkip(page.Page, page.PageSize),
		paginationLimit(page.Page, page.PageSize),
	)
	items, err := r.repo.aggregateList(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	return questionnaireRowsFromDomain(items), nil
}

func (r questionnaireReadModel) CountPublishedQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter) (int64, error) {
	pipeline := []bson.M{
		{"$match": buildPublishedListFilter(questionnaireFilterToConditions(filter))},
		{"$addFields": bson.M{"published_priority": publishedPriorityExpr()}},
		{"$sort": bson.M{"code": 1, "published_priority": -1, "updated_at": -1}},
		{"$group": bson.M{"_id": "$code"}},
		{"$count": "total"},
	}

	cursor, err := r.repo.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}

	var result struct {
		Total int64 `bson:"total"`
	}
	if err := cursor.Decode(&result); err != nil {
		return 0, err
	}
	return result.Total, nil
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
