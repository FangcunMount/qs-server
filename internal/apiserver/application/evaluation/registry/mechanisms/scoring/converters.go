package scoring

import (
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func calcInputFromSnapshot(snapshot *evaluationinput.InputSnapshot) calcscoring.Input {
	scaleSnapshot, _ := evaluationinput.ScalePayload(snapshot)
	return calcscoring.Input{
		Model:         modelFromSnapshot(scaleSnapshot),
		AnswerSheet:   scaleAnswerSheetFromDomain(answerSheetFromPort(snapshot.AnswerSheet)),
		Questionnaire: scaleQuestionnaireFromDomain(questionnaireFromPort(snapshot.Questionnaire)),
	}
}

// CloneInputWithScaleSnapshot clones input 快照 使用 scale 载荷 substituted。
func CloneInputWithScaleSnapshot(input *evaluationinput.InputSnapshot, scaleSnapshot *scalesnapshot.ScaleSnapshot) *evaluationinput.InputSnapshot {
	if input == nil {
		return nil
	}
	cloned := *input
	if scaleSnapshot != nil {
		cloned.ModelPayload = evaluationinput.ScaleModelPayload{Scale: scaleSnapshot}
		if cloned.Model != nil {
			model := *cloned.Model
			model.Payload = evaluationinput.ScaleModelPayload{Scale: scaleSnapshot}
			cloned.Model = &model
		}
	}
	return &cloned
}

func answerSheetFromPort(snapshot *evaluationinput.AnswerSheetSnapshot) *evalinput.AnswerSheet {
	if snapshot == nil {
		return nil
	}
	answers := make([]evalinput.Answer, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, evalinput.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evalinput.AnswerSheet{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func questionnaireFromPort(snapshot *evaluationinput.QuestionnaireSnapshot) *evalinput.Questionnaire {
	if snapshot == nil {
		return nil
	}
	questions := make([]evalinput.Question, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]evalinput.Option, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, evalinput.Option{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, evalinput.Question{
			Code:    question.Code,
			Type:    question.Type,
			Options: options,
		})
	}
	return &evalinput.Questionnaire{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Title:     snapshot.Title,
		Questions: questions,
	}
}

func modelFromSnapshot(snapshot *scalesnapshot.ScaleSnapshot) calcscoring.Model {
	if snapshot == nil {
		return calcscoring.Model{}
	}
	factors := make([]calcscoring.Factor, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, factorFromSnapshot(factor))
	}
	return calcscoring.Model{
		Code:                 snapshot.Code,
		ScaleVersion:         snapshot.ScaleVersion,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               snapshot.Status,
		Factors:              factors,
	}
}

func factorFromSnapshot(factor scalesnapshot.FactorSnapshot) calcscoring.Factor {
	return calcscoring.Factor{
		Code:            factor.Code,
		Title:           factor.Title,
		ScoringStrategy: factor.ScoringStrategy,
		ScoringParams: calcscoring.CntParams{
			CntOptionContents: append([]string(nil), factor.ScoringParams.CntOptionContents...),
		},
		QuestionCodes:  append([]string(nil), factor.QuestionCodes...),
		MaxScore:       factor.MaxScore,
		IsTotalScore:   factor.IsTotalScore,
		InterpretRules: interpretRulesFromSnapshot(factor.InterpretRules),
	}
}

func interpretRulesFromSnapshot(rules []scalesnapshot.InterpretRuleSnapshot) []calcscoring.InterpretRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]calcscoring.InterpretRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, calcscoring.InterpretRule{
			Min:        rule.Min,
			Max:        rule.Max,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
	}
	return out
}

func scaleAnswerSheetFromDomain(snapshot *evalinput.AnswerSheet) *calcscoring.AnswerSheet {
	if snapshot == nil {
		return nil
	}
	answers := make([]calcscoring.Answer, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, calcscoring.Answer{
			QuestionCode: meta.NewCode(answer.QuestionCode),
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &calcscoring.AnswerSheet{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func scaleQuestionnaireFromDomain(snapshot *evalinput.Questionnaire) *calcscoring.Questionnaire {
	if snapshot == nil {
		return nil
	}
	questions := make([]calcscoring.Question, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]calcscoring.Option, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, calcscoring.Option{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, calcscoring.Question{
			Code:    meta.NewCode(question.Code),
			Options: options,
		})
	}
	return &calcscoring.Questionnaire{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Questions: questions,
	}
}
