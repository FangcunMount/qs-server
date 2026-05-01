package questionnaire

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/mongo"
)

type questionnaireReadModel struct {
	repo *Repository
}

// NewQuestionnaireReadModel adapts questionnaire Mongo repositories to the read-model port.
func NewQuestionnaireReadModel(repo *Repository) surveyreadmodel.QuestionnaireReader {
	return questionnaireReadModel{repo: repo}
}

func (r questionnaireReadModel) ListQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	pipeline := questionnaireHeadReadModelPipeline(filter, page)
	cursor, err := r.repo.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()
	return questionnaireRowsFromCursor(ctx, cursor)
}

func (r questionnaireReadModel) CountQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter) (int64, error) {
	return r.repo.CountDocuments(ctx, questionnaireHeadReadModelFilter(filter))
}

func (r questionnaireReadModel) ListPublishedQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	pipeline := questionnairePublishedReadModelPipeline(filter, page)
	cursor, err := r.repo.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()
	return questionnaireRowsFromCursor(ctx, cursor)
}

func (r questionnaireReadModel) CountPublishedQuestionnaires(ctx context.Context, filter surveyreadmodel.QuestionnaireFilter) (int64, error) {
	pipeline := questionnairePublishedReadModelCountPipeline(filter)
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

func questionnaireRowsFromCursor(ctx context.Context, cursor *mongo.Cursor) ([]surveyreadmodel.QuestionnaireSummaryRow, error) {
	var rows []surveyreadmodel.QuestionnaireSummaryRow
	for cursor.Next(ctx) {
		var po QuestionnairePO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		rows = append(rows, questionnaireRowFromPO(&po))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func questionnaireRowsFromPO(items []QuestionnairePO) []surveyreadmodel.QuestionnaireSummaryRow {
	rows := make([]surveyreadmodel.QuestionnaireSummaryRow, 0, len(items))
	for i := range items {
		rows = append(rows, questionnaireRowFromPO(&items[i]))
	}
	return rows
}

func questionnaireRowFromPO(item *QuestionnairePO) surveyreadmodel.QuestionnaireSummaryRow {
	if item == nil {
		return surveyreadmodel.QuestionnaireSummaryRow{}
	}
	return surveyreadmodel.QuestionnaireSummaryRow{
		Code:          item.Code,
		Version:       item.Version,
		Title:         item.Title,
		Description:   item.Description,
		ImgURL:        item.ImgUrl,
		Status:        item.Status,
		Type:          item.Type,
		QuestionCount: item.QuestionCount,
		CreatedBy:     meta.FromUint64(item.CreatedBy),
		CreatedAt:     item.CreatedAt,
		UpdatedBy:     meta.FromUint64(item.UpdatedBy),
		UpdatedAt:     item.UpdatedAt,
	}
}
