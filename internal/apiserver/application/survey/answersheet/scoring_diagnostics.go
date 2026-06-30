package answersheet

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

func logZeroScoreDetails(l *logger.RequestLogger, sheet *answersheet.AnswerSheet, qnr *questionnaire.Questionnaire, scoredSheet *answersheet.ScoredAnswerSheet) {
	if scoredSheet.TotalScore != 0 || len(scoredSheet.ScoredAnswers) == 0 {
		return
	}
	for _, scoredAns := range scoredSheet.ScoredAnswers {
		if scoredAns.Score != 0 {
			continue
		}
		answerValue := findAnswerRawValue(sheet, scoredAns.QuestionCode)
		optionScores := findQuestionOptionScores(qnr, scoredAns.QuestionCode)
		selection, hasSelection := answervalue.NormalizeSingleOption(answerValue)
		matched := false
		if hasSelection {
			_, matched = optionScores[selection]
		}

		l.Warnw("答案分数为0的详情",
			"question_code", scoredAns.QuestionCode,
			"answer_value", answerValue,
			"normalized_selection", selection,
			"answer_value_type", fmt.Sprintf("%T", answerValue),
			"option_scores", optionScores,
			"option_score_count", len(optionScores),
			"all_option_scores_zero", allOptionScoresZero(optionScores),
			"matched", matched,
			"score", scoredAns.Score,
		)
	}
}

func findAnswerRawValue(sheet *answersheet.AnswerSheet, questionCode string) interface{} {
	for _, ans := range sheet.Answers() {
		if ans.QuestionCode() == questionCode {
			return ans.Value().Raw()
		}
	}
	return nil
}

func findQuestionOptionScores(qnr *questionnaire.Questionnaire, questionCode string) map[string]float64 {
	for _, q := range qnr.GetQuestions() {
		if q.GetCode().Value() != questionCode {
			continue
		}
		options := q.GetOptions()
		optionScores := make(map[string]float64, len(options))
		for _, opt := range options {
			optionScores[opt.GetCode().Value()] = opt.GetScore()
		}
		return optionScores
	}
	return nil
}

func allOptionScoresZero(optionScores map[string]float64) bool {
	if len(optionScores) == 0 {
		return true
	}
	for _, score := range optionScores {
		if score != 0 {
			return false
		}
	}
	return true
}
