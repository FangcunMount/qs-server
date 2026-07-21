package scoring

import (
	"sort"

	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func calcInputFromSnapshot(snapshot *evaluationinput.InputSnapshot) calcscoring.Input {
	if def, ok := evaluationinput.DefinitionV2FromSnapshot(snapshot); ok {
		if scale := scaleSnapshotFromDefinition(snapshot, def); scale != nil {
			return calcscoring.Input{
				Model:         modelFromSnapshot(scale),
				AnswerSheet:   scaleAnswerSheetFromDomain(answerSheetFromPort(snapshot.AnswerSheet)),
				Questionnaire: scaleQuestionnaireFromDomain(questionnaireFromPort(snapshot.Questionnaire)),
			}
		}
	}
	return calcscoring.Input{
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

func scaleSnapshotFromDefinition(input *evaluationinput.InputSnapshot, def *modeldefinition.Definition) *scalesnapshot.ScaleSnapshot {
	if input == nil || def == nil || len(def.Measure.Factors) == 0 {
		return nil
	}
	env := scalesnapshot.ExecutionEnvelope{Status: "published"}
	if input.Model != nil {
		env.Code = input.Model.Code
		env.ScaleVersion = input.Model.Version
		env.Title = input.Model.Title
	}
	if input.AnswerSheet != nil {
		env.QuestionnaireCode = input.AnswerSheet.QuestionnaireCode
		env.QuestionnaireVersion = input.AnswerSheet.QuestionnaireVersion
	}
	return scalesnapshot.ScaleSnapshotFromDefinition(env, def)
}

func modelFromSnapshot(snapshot *scalesnapshot.ScaleSnapshot) calcscoring.Model {
	if snapshot == nil || !snapshot.HasCanonicalMeasure() {
		return calcscoring.Model{}
	}
	return modelFromCanonicalMeasure(snapshot)
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
	for _, item := range orderedMeasureFactors(measure) {
		projected := calcscoring.Factor{
			Code:           item.Code,
			Title:          item.Title,
			SortOrder:      measure.FactorGraph.SortOrders[item.Code],
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

func orderedMeasureFactors(measure *modeldefinition.MeasureSpec) []factor.Factor {
	if measure == nil || len(measure.Factors) == 0 {
		return nil
	}
	ordered := append([]factor.Factor(nil), measure.Factors...)
	positions := make(map[string]int, len(measure.Factors))
	for index, item := range measure.Factors {
		positions[item.Code] = index
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftSet := measure.FactorGraph.SortOrders[ordered[i].Code]
		right, rightSet := measure.FactorGraph.SortOrders[ordered[j].Code]
		switch {
		case leftSet && rightSet && left != right:
			return left < right
		case leftSet != rightSet:
			return leftSet
		default:
			return positions[ordered[i].Code] < positions[ordered[j].Code]
		}
	})
	return ordered
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
