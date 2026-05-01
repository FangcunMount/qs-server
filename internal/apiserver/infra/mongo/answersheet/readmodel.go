package answersheet

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type answerSheetReadModel struct {
	repo *Repository
}

// NewAnswerSheetReadModel adapts answer-sheet Mongo repositories to the read-model port.
func NewAnswerSheetReadModel(repo *Repository) surveyreadmodel.AnswerSheetReader {
	return answerSheetReadModel{repo: repo}
}

func (r answerSheetReadModel) ListAnswerSheets(ctx context.Context, filter surveyreadmodel.AnswerSheetFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.AnswerSheetSummaryRow, error) {
	if page.PageSize <= 0 {
		return []surveyreadmodel.AnswerSheetSummaryRow{}, nil
	}

	pipeline, err := answerSheetListPipeline(filter, page)
	if err != nil {
		return nil, err
	}
	cursor, err := r.repo.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	return decodeSummaryRows(ctx, cursor)
}

func (r answerSheetReadModel) CountAnswerSheets(ctx context.Context, filter surveyreadmodel.AnswerSheetFilter) (int64, error) {
	return r.repo.Collection().CountDocuments(ctx, answerSheetFilterToBSON(filter))
}

func answerSheetListPipeline(filter surveyreadmodel.AnswerSheetFilter, page surveyreadmodel.PageRequest) ([]bson.M, error) {
	query, err := answerSheetListFilterToBSON(filter)
	if err != nil {
		return nil, err
	}
	return []bson.M{
		{"$match": query},
		{"$sort": bson.M{"filled_at": -1}},
		{"$skip": answerSheetPaginationSkip(page.Page, page.PageSize)},
		{"$limit": answerSheetPaginationLimit(page.Page, page.PageSize)},
		{"$project": answerSheetSummaryProjection()},
	}, nil
}

func answerSheetListFilterToBSON(filter surveyreadmodel.AnswerSheetFilter) (bson.M, error) {
	query := bson.M{
		"questionnaire_code": filter.QuestionnaireCode,
		"deleted_at":         nil,
	}
	if filter.FillerID != nil && *filter.FillerID > 0 {
		fillerID, err := safeconv.Uint64ToInt64(*filter.FillerID)
		if err != nil {
			return nil, err
		}
		query = bson.M{
			"filler_id":  fillerID,
			"deleted_at": nil,
		}
	}
	return query, nil
}

func answerSheetSummaryProjection() bson.M {
	return bson.M{
		"domain_id":           1,
		"questionnaire_code":  1,
		"questionnaire_title": 1,
		"filler_id":           1,
		"filler_type":         1,
		"total_score":         1,
		"filled_at":           1,
		"answer_count":        bson.M{"$size": bson.M{"$ifNull": []interface{}{"$answers", []interface{}{}}}},
	}
}

func answerSheetFilterToBSON(filter surveyreadmodel.AnswerSheetFilter) bson.M {
	query := bson.M{}
	if filter.QuestionnaireCode != "" {
		query["questionnaire_code"] = filter.QuestionnaireCode
	}
	if filter.FillerID != nil && *filter.FillerID > 0 {
		// Preserve the historical read-model count behavior. List queries
		// normalize filler_id to int64 for Mongo, while the legacy count path
		// passed the typed filter value through as-is.
		query["filler_id"] = *filter.FillerID
	}
	if filter.StartTime != nil {
		query["start_time"] = filter.StartTime
	}
	if filter.EndTime != nil {
		query["end_time"] = filter.EndTime
	}
	query["deleted_at"] = nil
	return query
}

func answerSheetPaginationLimit(page, pageSize int) int64 {
	if page <= 0 || pageSize <= 0 {
		return 0
	}
	return int64(pageSize)
}

func answerSheetPaginationSkip(page, pageSize int) int64 {
	if page <= 1 || pageSize <= 0 {
		return 0
	}
	return int64((page - 1) * pageSize)
}

func decodeSummaryRows(ctx context.Context, cursor *mongo.Cursor) ([]surveyreadmodel.AnswerSheetSummaryRow, error) {
	var rows []surveyreadmodel.AnswerSheetSummaryRow
	for cursor.Next(ctx) {
		var po AnswerSheetSummaryPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		row, err := answerSheetRowFromPO(&po)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func answerSheetRowFromPO(po *AnswerSheetSummaryPO) (surveyreadmodel.AnswerSheetSummaryRow, error) {
	id, err := safeconv.Uint64ToMetaID(po.DomainID)
	if err != nil {
		return surveyreadmodel.AnswerSheetSummaryRow{}, err
	}
	fillerID, err := safeconv.Int64ToUint64(po.FillerID)
	if err != nil {
		return surveyreadmodel.AnswerSheetSummaryRow{}, err
	}

	row := surveyreadmodel.AnswerSheetSummaryRow{
		ID:                 id,
		QuestionnaireCode:  po.QuestionnaireCode,
		QuestionnaireTitle: po.QuestionnaireTitle,
		FillerID:           fillerID,
		FillerType:         po.FillerType,
		TotalScore:         po.TotalScore,
		AnswerCount:        po.AnswerCount,
	}
	if po.FilledAt != nil {
		row.FilledAt = *po.FilledAt
	}
	return row, nil
}
