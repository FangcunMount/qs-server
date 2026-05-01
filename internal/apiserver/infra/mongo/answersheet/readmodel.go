package answersheet

import (
	"context"

	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
)

type answerSheetReadModel struct {
	repo domainanswersheet.Repository
}

// NewAnswerSheetReadModel adapts answer-sheet Mongo repositories to the read-model port.
func NewAnswerSheetReadModel(repo domainanswersheet.Repository) surveyreadmodel.AnswerSheetReader {
	return answerSheetReadModel{repo: repo}
}

func (r answerSheetReadModel) ListAnswerSheets(ctx context.Context, filter surveyreadmodel.AnswerSheetFilter, page surveyreadmodel.PageRequest) ([]surveyreadmodel.AnswerSheetSummaryRow, error) {
	var (
		items []*domainanswersheet.AnswerSheetSummary
		err   error
	)
	switch {
	case filter.FillerID != nil && *filter.FillerID > 0:
		items, err = r.repo.FindSummaryListByFiller(ctx, *filter.FillerID, page.Page, page.PageSize)
	default:
		items, err = r.repo.FindSummaryListByQuestionnaire(ctx, filter.QuestionnaireCode, page.Page, page.PageSize)
	}
	if err != nil {
		return nil, err
	}
	return answerSheetRowsFromDomain(items), nil
}

func (r answerSheetReadModel) CountAnswerSheets(ctx context.Context, filter surveyreadmodel.AnswerSheetFilter) (int64, error) {
	return r.repo.CountWithConditions(ctx, answerSheetFilterToConditions(filter))
}

func answerSheetFilterToConditions(filter surveyreadmodel.AnswerSheetFilter) map[string]interface{} {
	conditions := make(map[string]interface{})
	if filter.QuestionnaireCode != "" {
		conditions["questionnaire_code"] = filter.QuestionnaireCode
	}
	if filter.FillerID != nil && *filter.FillerID > 0 {
		conditions["filler_id"] = *filter.FillerID
	}
	if filter.StartTime != nil {
		conditions["start_time"] = filter.StartTime
	}
	if filter.EndTime != nil {
		conditions["end_time"] = filter.EndTime
	}
	return conditions
}

func answerSheetRowsFromDomain(items []*domainanswersheet.AnswerSheetSummary) []surveyreadmodel.AnswerSheetSummaryRow {
	rows := make([]surveyreadmodel.AnswerSheetSummaryRow, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		rows = append(rows, surveyreadmodel.AnswerSheetSummaryRow{
			ID:                   item.ID,
			QuestionnaireCode:    item.QuestionnaireCode,
			QuestionnaireVersion: item.QuestionnaireVersion,
			QuestionnaireTitle:   item.QuestionnaireTitle,
			FillerID:             item.FillerID,
			FillerType:           item.FillerType,
			TotalScore:           item.TotalScore,
			AnswerCount:          item.AnswerCount,
			FilledAt:             item.FilledAt,
		})
	}
	return rows
}
