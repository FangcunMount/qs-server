package shared

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// DefinitionBodyFromDefinition projects the common legacy wire body from the
// canonical measure and risk-conclusion layers. Family extensions are added by
// their owning payload adapter.
func DefinitionBodyFromDefinition(def *definition.Definition) DefinitionBody {
	if def == nil || len(def.Measure.Factors) == 0 {
		return DefinitionBody{}
	}
	scoringByFactor := make(map[string]factor.Scoring, len(def.Measure.Scoring))
	for _, item := range def.Measure.Scoring {
		scoringByFactor[item.FactorCode] = item
	}
	levels := def.Measure.FactorGraph.Levels()
	dimensions := make([]DimensionRule, 0, len(def.Measure.Factors))
	for _, item := range def.Measure.Factors {
		dimension := DimensionRule{
			Code:         item.Code,
			Title:        item.Title,
			Role:         string(item.ResolvedRole()),
			ParentCode:   def.Measure.FactorGraph.ParentCode(item.Code),
			SortOrder:    def.Measure.FactorGraph.SortOrders[item.Code],
			Level:        levels[item.Code],
			IsTotalScore: item.ResolvedRole() == factor.FactorRoleTotal,
			IsShow:       true,
		}
		if scoring, ok := scoringByFactor[item.Code]; ok {
			dimension.ScoringStrategy = string(scoring.Strategy)
			dimension.MaxScore = cloneFloat64(scoring.MaxScore)
			if scoring.Params != nil {
				dimension.ScoringParams = &ScoringParamsPayload{CntOptionContents: append([]string(nil), scoring.Params.CntOptionContents...)}
			}
			for _, source := range scoring.Sources {
				switch source.Kind {
				case factor.ScoringSourceQuestion:
					dimension.QuestionCodes = append(dimension.QuestionCodes, source.Code)
				case factor.ScoringSourceFactor:
					if dimension.ChildrenPolicy == nil {
						dimension.ChildrenPolicy = &ChildrenPolicyPayload{Strategy: string(scoring.Strategy), Weights: cloneWeights(scoring.Weights)}
					}
					dimension.ChildrenPolicy.Children = append(dimension.ChildrenPolicy.Children, source.Code)
				}
			}
		}
		dimensions = append(dimensions, dimension)
	}
	return DefinitionBody{Dimensions: dimensions, InterpretRules: riskRulesFromDefinition(def.Conclusions)}
}

func riskRulesFromDefinition(items []conclusion.Conclusion) []InterpretRule {
	rules := make([]InterpretRule, 0)
	for _, item := range items {
		risk, ok := item.(conclusion.RiskConclusion)
		if !ok || risk.FactorCode == "" {
			continue
		}
		rule := InterpretRule{DimensionCode: risk.FactorCode, Ranges: make([]ScoreRangeRule, 0, len(risk.Rules))}
		for _, value := range risk.Rules {
			rule.Ranges = append(rule.Ranges, ScoreRangeRule{MinScore: value.MinScore, MaxScore: value.MaxScore, Level: value.Level, Conclusion: value.Summary, Suggestion: value.Description})
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return nil
	}
	return rules
}
