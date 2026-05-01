package answersheet

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

func buildAnswerScoreTasks(sheet *answersheet.AnswerSheet, qnr *questionnaire.Questionnaire) []ruleengine.AnswerScoreTask {
	questionMap := buildScoringQuestionMap(qnr.GetQuestions())
	tasks := make([]ruleengine.AnswerScoreTask, 0, len(sheet.Answers()))
	for _, ans := range sheet.Answers() {
		question, found := questionMap[ans.QuestionCode()]
		if !found {
			continue
		}
		tasks = append(tasks, ruleengine.AnswerScoreTask{
			ID:           ans.QuestionCode(),
			Value:        answersheet.NewScorableValue(ans.Value()),
			OptionScores: buildScoringOptionScoreMap(question.GetOptions()),
		})
	}
	return tasks
}

func scoredAnswerSheetFromResults(sheet *answersheet.AnswerSheet, results []ruleengine.AnswerScoreResult) *answersheet.ScoredAnswerSheet {
	resultMap := make(map[string]ruleengine.AnswerScoreResult, len(results))
	for _, result := range results {
		resultMap[result.ID] = result
	}

	scoredAnswers := make([]answersheet.ScoredAnswer, 0, len(sheet.Answers()))
	var totalScore float64
	for _, ans := range sheet.Answers() {
		result, found := resultMap[ans.QuestionCode()]
		if !found {
			continue
		}
		scoredAnswers = append(scoredAnswers, answersheet.ScoredAnswer{
			QuestionCode: ans.QuestionCode(),
			Score:        result.Score,
			MaxScore:     result.MaxScore,
		})
		totalScore += result.Score
	}
	return &answersheet.ScoredAnswerSheet{
		AnswerSheetID: sheet.ID().Uint64(),
		TotalScore:    totalScore,
		ScoredAnswers: scoredAnswers,
	}
}

func buildScoringQuestionMap(questions []questionnaire.Question) map[string]questionnaire.Question {
	questionMap := make(map[string]questionnaire.Question, len(questions))
	for _, question := range questions {
		questionMap[question.GetCode().Value()] = question
	}
	return questionMap
}

func buildScoringOptionScoreMap(options []questionnaire.Option) map[string]float64 {
	optionScoreMap := make(map[string]float64, len(options))
	for _, opt := range options {
		optionScoreMap[opt.GetCode().Value()] = opt.GetScore()
	}
	return optionScoreMap
}
