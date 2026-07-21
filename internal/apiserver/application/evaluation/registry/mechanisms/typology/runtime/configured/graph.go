package configured

import (
	"fmt"
	"strings"

	calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func buildGraphAndDecision(payload *modeltypology.Payload, spec *modeltypology.RuntimeSpec) (calcclassification.FactorGraph, calcclassification.DecisionSpec, error) {
	if payload == nil || spec == nil {
		return calcclassification.FactorGraph{}, calcclassification.DecisionSpec{}, fmt.Errorf("payload and runtime spec are required")
	}
	graph, err := buildFactorGraph(spec.FactorGraph)
	if err != nil {
		return calcclassification.FactorGraph{}, calcclassification.DecisionSpec{}, err
	}
	decision, err := buildDecisionSpec(payload, spec)
	if err != nil {
		return calcclassification.FactorGraph{}, calcclassification.DecisionSpec{}, err
	}
	return graph, decision, nil
}

func buildFactorGraph(fg modeltypology.FactorGraphSpec) (calcclassification.FactorGraph, error) {
	if !fg.HasExplicitFactorGraph() {
		return calcclassification.FactorGraph{}, fmt.Errorf("explicit factor graph is required")
	}
	return buildExplicitFactorGraph(fg)
}

func buildExplicitFactorGraph(fg modeltypology.FactorGraphSpec) (calcclassification.FactorGraph, error) {
	factors := make(map[calcclassification.FactorID]calcclassification.PersonalityFactor, len(fg.Factors))
	leafSpecs := make(map[calcclassification.FactorID]calcclassification.LeafScoringSpec)
	for id, spec := range fg.Factors {
		factorID := calcclassification.FactorID(firstNonEmpty(spec.ID, id))
		if spec.ID == "" {
			spec.ID = id
		}
		code := firstNonEmpty(spec.Code, spec.ID)
		name := spec.Name
		if meta, ok := fg.Dimensions[spec.ID]; ok && name == "" {
			name = meta.Name
		}
		switch spec.Kind {
		case modeltypology.FactorSpecKindLeaf, "":
			children := childFactorIDs(spec.Children)
			if len(children) > 0 {
				return calcclassification.FactorGraph{}, fmt.Errorf("leaf factor %s must not have children", factorID)
			}
			factors[factorID] = calcclassification.PersonalityFactor{
				ID:   factorID,
				Code: code,
				Name: name,
				Kind: calcclassification.FactorKindLeaf,
			}
			leafSpecs[factorID] = leafScoringSpecFromFactorSpec(spec)
		case modeltypology.FactorSpecKindComposite:
			children := childFactorIDs(spec.Children)
			if len(children) == 0 {
				return calcclassification.FactorGraph{}, fmt.Errorf("composite factor %s requires children", factorID)
			}
			weights := make(map[calcclassification.FactorID]float64, len(spec.Weights))
			for childID, weight := range spec.Weights {
				weights[calcclassification.FactorID(childID)] = weight
			}
			factors[factorID] = calcclassification.PersonalityFactor{
				ID:          factorID,
				Code:        code,
				Name:        name,
				Kind:        calcclassification.FactorKindComposite,
				Children:    children,
				Aggregation: profileAggregation(spec.Aggregation),
				Weights:     weights,
			}
		default:
			return calcclassification.FactorGraph{}, fmt.Errorf("factor %s has unsupported kind %s", factorID, spec.Kind)
		}
	}
	roots := make([]calcclassification.FactorID, 0, len(fg.Roots))
	for _, rootID := range fg.Roots {
		factorID := calcclassification.FactorID(rootID)
		if _, ok := factors[factorID]; !ok {
			return calcclassification.FactorGraph{}, fmt.Errorf("root factor %s is not defined", rootID)
		}
		roots = append(roots, factorID)
	}
	graph := calcclassification.FactorGraph{Factors: factors, LeafSpecs: leafSpecs, Roots: roots}
	if err := graph.Validate(); err != nil {
		return calcclassification.FactorGraph{}, err
	}
	return graph, nil
}

func leafScoringSpecFromFactorSpec(spec modeltypology.FactorSpec) calcclassification.LeafScoringSpec {
	contributions := make([]calcclassification.AnswerContribution, 0, len(spec.Contributions))
	for _, contribution := range spec.Contributions {
		item := calcclassification.AnswerContribution{
			QuestionCode: contribution.QuestionCode,
			ScoringMode:  calcclassification.QuestionScoringMode(contribution.ScoringMode),
			Sign:         contribution.Sign,
			Weight:       contribution.Weight,
		}
		if len(contribution.OptionScores) > 0 {
			item.OptionScores = cloneOptionScores(contribution.OptionScores)
		}
		contributions = append(contributions, item)
	}
	return calcclassification.LeafScoringSpec{
		Constant:      spec.Constant,
		Contributions: contributions,
	}
}

func profileAggregation(aggregation modeltypology.FactorAggregation) calcclassification.AggregationMethod {
	switch aggregation {
	case modeltypology.FactorAggregationAvg:
		return calcclassification.AggregationAvg
	case modeltypology.FactorAggregationWeightedAvg:
		return calcclassification.AggregationWeightedAvg
	default:
		return calcclassification.AggregationSum
	}
}

func childFactorIDs(children []string) []calcclassification.FactorID {
	if len(children) == 0 {
		return nil
	}
	out := make([]calcclassification.FactorID, 0, len(children))
	for _, childID := range children {
		out = append(out, calcclassification.FactorID(childID))
	}
	return out
}

func buildDecisionSpec(payload *modeltypology.Payload, spec *modeltypology.RuntimeSpec) (calcclassification.DecisionSpec, error) {
	switch spec.Decision.Kind {
	case modelcatalog.DecisionKindPoleComposition, "":
		return buildPoleDecision(spec.FactorGraph)
	case modelcatalog.DecisionKindNearestPattern:
		return buildPatternDecision(payload, spec)
	case modelcatalog.DecisionKindTraitProfile:
		return calcclassification.DecisionSpec{Kind: calcclassification.DecisionKindTraitProfile}, nil
	case modelcatalog.DecisionKindDominantFactor:
		factorOrder := make([]calcclassification.FactorID, 0, len(spec.FactorGraph.DecisionFactorOrder()))
		for _, factorID := range spec.FactorGraph.DecisionFactorOrder() {
			factorOrder = append(factorOrder, calcclassification.FactorID(factorID))
		}
		return calcclassification.DecisionSpec{Kind: calcclassification.DecisionKindDominantFactor, FactorOrder: factorOrder, TopK: spec.Decision.TopK}, nil
	default:
		return calcclassification.DecisionSpec{}, fmt.Errorf("unsupported decision kind %s", spec.Decision.Kind)
	}
}

func buildPoleDecision(fg modeltypology.FactorGraphSpec) (calcclassification.DecisionSpec, error) {
	poles := make([]calcclassification.PoleSpec, 0, len(fg.DecisionFactorOrder()))
	for _, factorID := range fg.DecisionFactorOrder() {
		meta, ok := dimensionMetaForFactor(fg, factorID)
		if !ok {
			return calcclassification.DecisionSpec{}, fmt.Errorf("pole metadata for factor %s is not defined", factorID)
		}
		contributions := contributionsForFactor(fg, factorID)
		poles = append(poles, calcclassification.PoleSpec{
			FactorID:     calcclassification.FactorID(factorID),
			LeftPole:     meta.LeftPole,
			RightPole:    meta.RightPole,
			Threshold:    meta.Threshold,
			MaxDeviation: calcclassification.PoleMaxDeviation(meta.Constant, meta.Threshold, contributions),
		})
	}
	return calcclassification.DecisionSpec{Kind: calcclassification.DecisionKindPoleComposition, Poles: poles}, nil
}

func buildPatternDecision(payload *modeltypology.Payload, spec *modeltypology.RuntimeSpec) (calcclassification.DecisionSpec, error) {
	patternOrder := make([]calcclassification.FactorID, 0, len(spec.FactorGraph.DecisionFactorOrder()))
	for _, factorID := range spec.FactorGraph.DecisionFactorOrder() {
		patternOrder = append(patternOrder, calcclassification.FactorID(factorID))
	}
	patterns := make([]calcclassification.PatternCandidate, 0)
	for _, outcome := range payload.Outcomes {
		if outcome.IsSpecial || outcome.Pattern == "" {
			continue
		}
		patterns = append(patterns, calcclassification.PatternCandidate{
			Code:    outcome.Code,
			Label:   outcome.Name,
			Pattern: patternLevelsByOrder(outcome.Pattern, patternOrder),
		})
	}
	levelRule := calcclassification.LevelRule{LowMax: 3, HighMin: 5}
	if spec.Decision.LevelRule != nil {
		levelRule = calcclassification.LevelRule{
			LowMax:  spec.Decision.LevelRule.LowMax,
			HighMin: spec.Decision.LevelRule.HighMin,
		}
	}
	threshold := spec.Decision.FallbackSimilarityThreshold
	if threshold <= 0 {
		threshold = 0.6
	}
	fallbackCode := spec.Decision.FallbackCode
	return calcclassification.DecisionSpec{
		Kind:              calcclassification.DecisionKindNearestPattern,
		PatternOrder:      patternOrder,
		Patterns:          patterns,
		LevelRule:         levelRule,
		FallbackThreshold: threshold,
		FallbackCode:      fallbackCode,
	}, nil
}

func dimensionMetaForFactor(fg modeltypology.FactorGraphSpec, factorID string) (modeltypology.Dimension, bool) {
	if meta, ok := fg.Dimensions[factorID]; ok {
		return meta, true
	}
	if spec, ok := fg.Factors[factorID]; ok {
		meta := modeltypology.Dimension{Code: firstNonEmpty(spec.Code, spec.ID), Name: spec.Name}
		if stored, ok := fg.Dimensions[meta.Code]; ok {
			return stored, true
		}
		return meta, meta.Code != ""
	}
	return modeltypology.Dimension{}, false
}

func contributionsForFactor(fg modeltypology.FactorGraphSpec, factorID string) []calcclassification.AnswerContribution {
	spec, ok := fg.Factors[factorID]
	if !ok {
		return nil
	}
	out := make([]calcclassification.AnswerContribution, 0, len(spec.Contributions))
	for _, contribution := range spec.Contributions {
		out = append(out, calcclassification.AnswerContribution{
			QuestionCode: contribution.QuestionCode,
			ScoringMode:  calcclassification.QuestionScoringMode(contribution.ScoringMode),
			Sign:         contribution.Sign,
			Weight:       contribution.Weight,
			OptionScores: cloneOptionScores(contribution.OptionScores),
		})
	}
	return out
}

func patternLevelsByOrder(pattern string, order []calcclassification.FactorID) map[calcclassification.FactorID]string {
	compact := strings.ReplaceAll(pattern, "-", "")
	levels := make(map[calcclassification.FactorID]string, len(order))
	for i, factorID := range order {
		if i >= len(compact) {
			break
		}
		levels[factorID] = strings.ToUpper(string(compact[i]))
	}
	return levels
}

func cloneOptionScores(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	cloned := make(map[string]float64, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
