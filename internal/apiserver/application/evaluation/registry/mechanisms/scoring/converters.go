package scoring

import (
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
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
	if snapshot.HasCanonicalMeasure() {
		return modelFromCanonicalMeasure(snapshot)
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

func modelFromCanonicalMeasure(snapshot *scalesnapshot.ScaleSnapshot) calcscoring.Model {
	measure := snapshot.Measure
	interpretByCode := make(map[string][]calcscoring.InterpretRule, len(snapshot.Factors))
	for _, item := range snapshot.Factors {
		interpretByCode[item.Code] = interpretRulesFromSnapshot(item.InterpretRules)
	}
	scoringByFactor := make(map[string]factor.Scoring, len(measure.Scoring))
	for _, rule := range measure.Scoring {
		scoringByFactor[rule.FactorCode] = rule
	}
	factors := make([]calcscoring.Factor, 0, len(measure.Factors))
	for _, item := range measure.Factors {
		projected := calcscoring.Factor{
			Code:           item.Code,
			Title:          item.Title,
			IsTotalScore:   item.ResolvedRole() == factor.FactorRoleTotal,
			InterpretRules: interpretByCode[item.Code],
		}
		if rule, ok := scoringByFactor[item.Code]; ok {
			applyMeasureScoring(&projected, rule)
		}
		factors = append(factors, projected)
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

func applyMeasureScoring(projected *calcscoring.Factor, rule factor.Scoring) {
	projected.ScoringStrategy = string(rule.Strategy)
	projected.MaxScore = rule.MaxScore
	if rule.Params != nil {
		projected.ScoringParams = calcscoring.CntParams{
			CntOptionContents: append([]string(nil), rule.Params.CntOptionContents...),
		}
	}
	hasQuestion := false
	hasFactor := false
	for _, source := range rule.Sources {
		switch source.Kind {
		case factor.ScoringSourceQuestion:
			hasQuestion = true
		case factor.ScoringSourceFactor:
			hasFactor = true
		}
	}
	switch {
	case hasQuestion && !hasFactor:
		projected.QuestionCodes = make([]string, 0, len(rule.Sources))
		projected.Contributions = make([]calcscoring.QuestionContribution, 0, len(rule.Sources))
		for _, source := range rule.Sources {
			if source.Kind != factor.ScoringSourceQuestion {
				continue
			}
			projected.QuestionCodes = append(projected.QuestionCodes, source.Code)
			contrib := calcscoring.QuestionContribution{
				Code:        source.Code,
				Sign:        source.Sign,
				Weight:      source.Weight,
				ScoringMode: string(source.ScoringMode),
			}
			if len(source.OptionScores) > 0 {
				contrib.OptionScores = make(map[string]float64, len(source.OptionScores))
				for k, v := range source.OptionScores {
					contrib.OptionScores[k] = v
				}
			}
			projected.Contributions = append(projected.Contributions, contrib)
		}
	case hasFactor && !hasQuestion:
		projected.ChildCodes = make([]string, 0, len(rule.Sources))
		for _, source := range rule.Sources {
			if source.Kind != factor.ScoringSourceFactor {
				continue
			}
			projected.ChildCodes = append(projected.ChildCodes, source.Code)
		}
		if len(rule.Weights) > 0 {
			projected.ChildWeights = make(map[string]float64, len(rule.Weights))
			for k, v := range rule.Weights {
				projected.ChildWeights[k] = v
			}
		}
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
			Min:          rule.Min,
			Max:          rule.Max,
			MaxInclusive: rule.MaxInclusive,
			UnboundedMax: rule.UnboundedMax,
			RiskLevel:    rule.RiskLevel,
			Conclusion:   rule.Conclusion,
			Suggestion:   rule.Suggestion,
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
