package statistics

import (
	"context"
	"strings"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
)

type questionnaireBatchQuery struct {
	readModel       StatisticsReadModel
	answerSheetRead surveyreadmodel.AnswerSheetReader
}

func (q *questionnaireBatchQuery) GetQuestionnaireBatchStatistics(ctx context.Context, orgID int64, codes []string) (*domainStatistics.QuestionnaireBatchStatisticsResponse, error) {
	cleanCodes := normalizeQuestionnaireCodes(codes)
	items := make([]*domainStatistics.QuestionnaireBatchStatisticsItem, 0, len(cleanCodes))
	if len(cleanCodes) == 0 {
		return &domainStatistics.QuestionnaireBatchStatisticsResponse{Items: items}, nil
	}

	totals, err := q.readModel.GetQuestionnaireBatchTotals(ctx, orgID, cleanCodes)
	if err != nil {
		return nil, err
	}

	resultByCode := make(map[string]*domainStatistics.QuestionnaireBatchStatisticsItem, len(cleanCodes))
	for _, codeValue := range cleanCodes {
		resultByCode[codeValue] = &domainStatistics.QuestionnaireBatchStatisticsItem{Code: codeValue}
	}
	for _, total := range totals {
		item := resultByCode[total.Code]
		if item == nil {
			item = &domainStatistics.QuestionnaireBatchStatisticsItem{Code: total.Code}
			resultByCode[total.Code] = item
		}
		item.TotalSubmissions = total.TotalSubmissions
		item.TotalCompletions = total.TotalCompletions
		if item.TotalSubmissions > 0 {
			item.CompletionRate = float64(item.TotalCompletions) / float64(item.TotalSubmissions) * 100
		}
	}

	for _, codeValue := range cleanCodes {
		items = append(items, resultByCode[codeValue])
	}

	if q.answerSheetRead != nil {
		for _, item := range items {
			if item.TotalSubmissions > 0 {
				continue
			}
			count, err := q.answerSheetRead.CountAnswerSheets(ctx, surveyreadmodel.AnswerSheetFilter{QuestionnaireCode: item.Code})
			if err != nil {
				return nil, err
			}
			if count <= 0 {
				continue
			}
			item.TotalSubmissions = count
			item.TotalCompletions = count
			item.CompletionRate = 100
		}
	}

	return &domainStatistics.QuestionnaireBatchStatisticsResponse{Items: items}, nil
}

func normalizeQuestionnaireCodes(codes []string) []string {
	cleanCodes := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, codeValue := range codes {
		codeValue = strings.TrimSpace(codeValue)
		if codeValue == "" {
			continue
		}
		if _, exists := seen[codeValue]; exists {
			continue
		}
		seen[codeValue] = struct{}{}
		cleanCodes = append(cleanCodes, codeValue)
	}
	return cleanCodes
}
