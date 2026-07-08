package configured

import (
	"fmt"
	"strings"

	calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func buildGraphAndDecision(payload *modeltypology.Payload, spec *modeltypology.RuntimeSpec) (calcclassification.FactorGraph, calcclassification.DecisionSpec, error) {
	if payload == nil || spec == nil {
		return calcclassification.FactorGraph{}, calcclassification.DecisionSpec{}, fmt.Errorf("payload and runtime spec are required")
	}
	graph, err := buildFactorGraph(spec.FactorGraph, spec.Decision.Kind)
	if err != nil {
		return calcclassification.FactorGraph{}, calcclassification.DecisionSpec{}, err
	}
	decision, err := buildDecisionSpec(payload, spec)
	if err != nil {
		return calcclassification.FactorGraph{}, calcclassification.DecisionSpec{}, err
	}
	return graph, decision, nil
}

func buildFactorGraph(fg modeltypology.FactorGraphSpec, kind modelcatalog.DecisionKind) (calcclassification.FactorGraph, error) {
	if fg.HasExplicitFactorGraph() {
		return buildExplicitFactorGraph(fg, kind)
	}
	return buildLegacyFlatFactorGraph(fg, kind)
}

func buildExplicitFactorGraph(fg modeltypology.FactorGraphSpec, kind modelcatalog.DecisionKind) (calcclassification.FactorGraph, error) {
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
			leafSpecs[factorID] = leafScoringSpecFromFactorSpec(spec, kind)
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

func buildLegacyFlatFactorGraph(fg modeltypology.FactorGraphSpec, kind modelcatalog.DecisionKind) (calcclassification.FactorGraph, error) {
	if len(fg.DimensionOrder) == 0 {
		return calcclassification.FactorGraph{}, fmt.Errorf("dimension order is required")
	}
	mappingsByDimension := groupMappings(fg.QuestionMappings)
	factors := make(map[calcclassification.FactorID]calcclassification.PersonalityFactor, len(fg.DimensionOrder))
	leafSpecs := make(map[calcclassification.FactorID]calcclassification.LeafScoringSpec, len(fg.DimensionOrder))
	roots := make([]calcclassification.FactorID, 0, len(fg.DimensionOrder))

	for _, dimCode := range fg.DimensionOrder {
		meta, ok := fg.Dimensions[dimCode]
		if !ok {
			return calcclassification.FactorGraph{}, fmt.Errorf("dimension %s is not defined", dimCode)
		}
		mappings := mappingsByDimension[dimCode]
		if kind == modelcatalog.DecisionKindNearestPattern && len(mappings) == 0 {
			return calcclassification.FactorGraph{}, fmt.Errorf("dimension %s has no mapped answers", dimCode)
		}
		contributions := make([]calcclassification.AnswerContribution, 0, len(mappings))
		optionScoring := calcclassification.OptionScoringStrict
		if kind == modelcatalog.DecisionKindNearestPattern {
			optionScoring = calcclassification.OptionScoringCompat
		}
		for _, mapping := range mappings {
			contribution := calcclassification.AnswerContribution{
				QuestionCode: mapping.QuestionCode,
				Sign:         mapping.Sign,
			}
			if len(mapping.OptionScores) > 0 {
				contribution.OptionScores = cloneOptionScores(mapping.OptionScores)
			}
			contributions = append(contributions, contribution)
		}
		factorID := calcclassification.FactorID(dimCode)
		factors[factorID] = calcclassification.PersonalityFactor{
			ID:   factorID,
			Code: meta.Code,
			Name: meta.Name,
			Kind: calcclassification.FactorKindLeaf,
		}
		leafSpecs[factorID] = calcclassification.LeafScoringSpec{
			Constant:      meta.Constant,
			Contributions: contributions,
			OptionScoring: optionScoring,
		}
		roots = append(roots, factorID)
	}

	graph := calcclassification.FactorGraph{Factors: factors, LeafSpecs: leafSpecs, Roots: roots}
	if err := graph.Validate(); err != nil {
		return calcclassification.FactorGraph{}, err
	}
	return graph, nil
}

func leafScoringSpecFromFactorSpec(spec modeltypology.FactorSpec, kind modelcatalog.DecisionKind) calcclassification.LeafScoringSpec {
	optionScoring := calcclassification.OptionScoringStrict
	if spec.OptionScoring == modeltypology.FactorOptionScoringCompat ||
		(spec.OptionScoring == "" && kind == modelcatalog.DecisionKindNearestPattern) {
		optionScoring = calcclassification.OptionScoringCompat
	}
	contributions := make([]calcclassification.AnswerContribution, 0, len(spec.Contributions))
	for _, contribution := range spec.Contributions {
		item := calcclassification.AnswerContribution{
			QuestionCode: contribution.QuestionCode,
			Sign:         contribution.Sign,
		}
		if len(contribution.OptionScores) > 0 {
			item.OptionScores = cloneOptionScores(contribution.OptionScores)
		}
		contributions = append(contributions, item)
	}
	return calcclassification.LeafScoringSpec{
		Constant:      spec.Constant,
		Contributions: contributions,
		OptionScoring: optionScoring,
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
		mappings := mappingsForFactor(fg, factorID)
		poles = append(poles, calcclassification.PoleSpec{
			FactorID:     calcclassification.FactorID(factorID),
			LeftPole:     meta.LeftPole,
			RightPole:    meta.RightPole,
			Threshold:    meta.Threshold,
			MaxDeviation: dimensionMaxDeviation(meta, mappings),
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

func mappingsForFactor(fg modeltypology.FactorGraphSpec, factorID string) []modeltypology.QuestionMapping {
	if fg.HasExplicitFactorGraph() {
		spec, ok := fg.Factors[factorID]
		if !ok {
			return nil
		}
		mappings := make([]modeltypology.QuestionMapping, 0, len(spec.Contributions))
		for _, contribution := range spec.Contributions {
			mappings = append(mappings, modeltypology.QuestionMapping{
				QuestionCode: contribution.QuestionCode,
				Dimension:    factorID,
				Sign:         contribution.Sign,
				OptionScores: contribution.OptionScores,
			})
		}
		return mappings
	}
	mappings := make([]modeltypology.QuestionMapping, 0)
	for _, mapping := range fg.QuestionMappings {
		if mapping.Dimension == factorID {
			mappings = append(mappings, mapping)
		}
	}
	return mappings
}

func groupMappings(mappings []modeltypology.QuestionMapping) map[string][]modeltypology.QuestionMapping {
	grouped := make(map[string][]modeltypology.QuestionMapping)
	for _, mapping := range mappings {
		grouped[mapping.Dimension] = append(grouped[mapping.Dimension], mapping)
	}
	return grouped
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

func dimensionMaxDeviation(meta modeltypology.Dimension, mappings []modeltypology.QuestionMapping) float64 {
	minScore := meta.Constant
	maxScore := meta.Constant
	for _, mapping := range mappings {
		if mapping.Dimension != meta.Code && mapping.Dimension != "" {
			continue
		}
		if len(mapping.OptionScores) > 0 {
			var localMin, localMax float64
			first := true
			for _, score := range mapping.OptionScores {
				if first {
					localMin, localMax = score, score
					first = false
					continue
				}
				if score < localMin {
					localMin = score
				}
				if score > localMax {
					localMax = score
				}
			}
			minScore += localMin
			maxScore += localMax
			continue
		}
		if mapping.Sign > 0 {
			minScore += mapping.Sign * 1
			maxScore += mapping.Sign * 5
		} else {
			minScore += mapping.Sign * 5
			maxScore += mapping.Sign * 1
		}
	}
	threshold := meta.Threshold
	if threshold == 0 {
		threshold = 24
	}
	left := threshold - minScore
	right := maxScore - threshold
	if left > right {
		return left
	}
	return right
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
