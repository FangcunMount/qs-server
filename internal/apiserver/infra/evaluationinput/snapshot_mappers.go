package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func answerSheetToSnapshot(sheet *answersheet.AnswerSheet) *port.AnswerSheetSnapshot {
	if sheet == nil {
		return nil
	}
	code, version, title := sheet.QuestionnaireInfo()
	answers := make([]port.AnswerSnapshot, 0, len(sheet.Answers()))
	for _, answer := range sheet.Answers() {
		var raw any
		if answer.Value() != nil {
			raw = answer.Value().Raw()
		}
		answers = append(answers, port.AnswerSnapshot{
			QuestionCode: answer.QuestionCode(),
			Score:        answer.Score(),
			Value:        raw,
		})
	}
	return &port.AnswerSheetSnapshot{
		ID:                   sheet.ID().Uint64(),
		QuestionnaireCode:    code,
		QuestionnaireVersion: version,
		QuestionnaireTitle:   title,
		Answers:              answers,
	}
}

func questionnaireToSnapshot(qnr *questionnaire.Questionnaire) *port.QuestionnaireSnapshot {
	if qnr == nil {
		return nil
	}
	questions := make([]port.QuestionSnapshot, 0, len(qnr.GetQuestions()))
	for _, q := range qnr.GetQuestions() {
		options := make([]port.OptionSnapshot, 0, len(q.GetOptions()))
		for _, opt := range q.GetOptions() {
			options = append(options, port.OptionSnapshot{
				Code:    opt.GetCode().String(),
				Content: opt.GetContent(),
				Score:   opt.GetScore(),
			})
		}
		questions = append(questions, port.QuestionSnapshot{
			Code:    q.GetCode().String(),
			Type:    q.GetType().Value(),
			Options: options,
		})
	}
	return &port.QuestionnaireSnapshot{
		Code:      qnr.GetCode().String(),
		Version:   qnr.GetVersion().String(),
		Title:     qnr.GetTitle(),
		Questions: questions,
	}
}
