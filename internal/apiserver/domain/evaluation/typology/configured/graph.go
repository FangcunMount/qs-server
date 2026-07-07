package configured

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/trait"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

func buildGraphAndDecision(payload *modeltypology.Payload, spec *modeltypology.RuntimeSpec) (trait.FactorGraph, trait.DecisionSpec, error) {
	if payload == nil || spec == nil {
		return trait.FactorGraph{}, trait.DecisionSpec{}, fmt.Errorf("payload and runtime spec are required")
	}
	graph, err := buildFactorGraph(spec.FactorGraph, spec.Decision.Kind)
	if err != nil {
		return trait.FactorGraph{}, trait.DecisionSpec{}, err
	}
	decision, err := buildDecisionSpec(payload, spec)
	if err != nil {
		return trait.FactorGraph{}, trait.DecisionSpec{}, err
	}
	return graph, decision, nil
}

func buildFactorGraph(fg modeltypology.FactorGraphSpec, kind modelcatalog.DecisionKind) (trait.FactorGraph, error) {
	if fg.HasExplicitFactorGraph() {
		return buildExplicitFactorGraph(fg, kind)
	}
	return buildLegacyFlatFactorGraph(fg, kind)
}

func buildExplicitFactorGraph(fg modeltypology.FactorGraphSpec, kind modelcatalog.DecisionKind) (trait.FactorGraph, error) {
	factors := make(map[trait.FactorID]trait.PersonalityFactor, len(fg.Factors))
	leafSpecs := make(map[trait.FactorID]trait.LeafScoringSpec)
	for id, spec := range fg.Factors {
		factorID := trait.FactorID(firstNonEmpty(spec.ID, id))
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
				return trait.FactorGraph{}, fmt.Errorf("leaf factor %s must not have children", factorID)
			}
			factors[factorID] = trait.PersonalityFactor{
				ID:   factorID,
				Code: code,
				Name: name,
				Kind: trait.FactorKindLeaf,
			}
			leafSpecs[factorID] = leafScoringSpecFromFactorSpec(spec, kind)
		case modeltypology.FactorSpecKindComposite:
			children := childFactorIDs(spec.Children)
			if len(children) == 0 {
				return trait.FactorGraph{}, fmt.Errorf("composite factor %s requires children", factorID)
			}
			weights := make(map[trait.FactorID]float64, len(spec.Weights))
			for childID, weight := range spec.Weights {
				weights[trait.FactorID(childID)] = weight
			}
			factors[factorID] = trait.PersonalityFactor{
				ID:          factorID,
				Code:        code,
				Name:        name,
				Kind:        trait.FactorKindComposite,
				Children:    children,
				Aggregation: profileAggregation(spec.Aggregation),
				Weights:     weights,
			}
		default:
			return trait.FactorGraph{}, fmt.Errorf("factor %s has unsupported kind %s", factorID, spec.Kind)
		}
	}
	roots := make([]trait.FactorID, 0, len(fg.Roots))
	for _, rootID := range fg.Roots {
		factorID := trait.FactorID(rootID)
		if _, ok := factors[factorID]; !ok {
			return trait.FactorGraph{}, fmt.Errorf("root factor %s is not defined", rootID)
		}
		roots = append(roots, factorID)
	}
	graph := trait.FactorGraph{Factors: factors, LeafSpecs: leafSpecs, Roots: roots}
	if err := graph.Validate(); err != nil {
		return trait.FactorGraph{}, err
	}
	return graph, nil
}

func buildLegacyFlatFactorGraph(fg modeltypology.FactorGraphSpec, kind modelcatalog.DecisionKind) (trait.FactorGraph, error) {
	if len(fg.DimensionOrder) == 0 {
		return trait.FactorGraph{}, fmt.Errorf("dimension order is required")
	}
	mappingsByDimension := groupMappings(fg.QuestionMappings)
	factors := make(map[trait.FactorID]trait.PersonalityFactor, len(fg.DimensionOrder))
	leafSpecs := make(map[trait.FactorID]trait.LeafScoringSpec, len(fg.DimensionOrder))
	roots := make([]trait.FactorID, 0, len(fg.DimensionOrder))

	for _, dimCode := range fg.DimensionOrder {
		meta, ok := fg.Dimensions[dimCode]
		if !ok {
			return trait.FactorGraph{}, fmt.Errorf("dimension %s is not defined", dimCode)
		}
		mappings := mappingsByDimension[dimCode]
		if kind == modelcatalog.DecisionKindNearestPattern && len(mappings) == 0 {
			return trait.FactorGraph{}, fmt.Errorf("dimension %s has no mapped answers", dimCode)
		}
		contributions := make([]trait.AnswerContribution, 0, len(mappings))
		optionScoring := trait.OptionScoringStrict
		if kind == modelcatalog.DecisionKindNearestPattern {
			optionScoring = trait.OptionScoringCompat
		}
		for _, mapping := range mappings {
			contribution := trait.AnswerContribution{
				QuestionCode: mapping.QuestionCode,
				Sign:         mapping.Sign,
			}
			if len(mapping.OptionScores) > 0 {
				contribution.OptionScores = cloneOptionScores(mapping.OptionScores)
			}
			contributions = append(contributions, contribution)
		}
		factorID := trait.FactorID(dimCode)
		factors[factorID] = trait.PersonalityFactor{
			ID:   factorID,
			Code: meta.Code,
			Name: meta.Name,
			Kind: trait.FactorKindLeaf,
		}
		leafSpecs[factorID] = trait.LeafScoringSpec{
			Constant:      meta.Constant,
			Contributions: contributions,
			OptionScoring: optionScoring,
		}
		roots = append(roots, factorID)
	}

	graph := trait.FactorGraph{Factors: factors, LeafSpecs: leafSpecs, Roots: roots}
	if err := graph.Validate(); err != nil {
		return trait.FactorGraph{}, err
	}
	return graph, nil
}

func leafScoringSpecFromFactorSpec(spec modeltypology.FactorSpec, kind modelcatalog.DecisionKind) trait.LeafScoringSpec {
	optionScoring := trait.OptionScoringStrict
	if spec.OptionScoring == modeltypology.FactorOptionScoringCompat ||
		(spec.OptionScoring == "" && kind == modelcatalog.DecisionKindNearestPattern) {
		optionScoring = trait.OptionScoringCompat
	}
	contributions := make([]trait.AnswerContribution, 0, len(spec.Contributions))
	for _, contribution := range spec.Contributions {
		item := trait.AnswerContribution{
			QuestionCode: contribution.QuestionCode,
			Sign:         contribution.Sign,
		}
		if len(contribution.OptionScores) > 0 {
			item.OptionScores = cloneOptionScores(contribution.OptionScores)
		}
		contributions = append(contributions, item)
	}
	return trait.LeafScoringSpec{
		Constant:      spec.Constant,
		Contributions: contributions,
		OptionScoring: optionScoring,
	}
}

func profileAggregation(aggregation modeltypology.FactorAggregation) trait.AggregationMethod {
	switch aggregation {
	case modeltypology.FactorAggregationAvg:
		return trait.AggregationAvg
	case modeltypology.FactorAggregationWeightedAvg:
		return trait.AggregationWeightedAvg
	default:
		return trait.AggregationSum
	}
}

func childFactorIDs(children []string) []trait.FactorID {
	if len(children) == 0 {
		return nil
	}
	out := make([]trait.FactorID, 0, len(children))
	for _, childID := range children {
		out = append(out, trait.FactorID(childID))
	}
	return out
}

func buildDecisionSpec(payload *modeltypology.Payload, spec *modeltypology.RuntimeSpec) (trait.DecisionSpec, error) {
	switch spec.Decision.Kind {
	case modelcatalog.DecisionKindPoleComposition, "":
		return buildPoleDecision(spec.FactorGraph)
	case modelcatalog.DecisionKindNearestPattern:
		return buildPatternDecision(payload, spec)
	case modelcatalog.DecisionKindTraitProfile:
		return trait.DecisionSpec{Kind: trait.DecisionKindTraitProfile}, nil
	default:
		return trait.DecisionSpec{}, fmt.Errorf("unsupported decision kind %s", spec.Decision.Kind)
	}
}

func buildPoleDecision(fg modeltypology.FactorGraphSpec) (trait.DecisionSpec, error) {
	poles := make([]trait.PoleSpec, 0, len(fg.DecisionFactorOrder()))
	for _, factorID := range fg.DecisionFactorOrder() {
		meta, ok := dimensionMetaForFactor(fg, factorID)
		if !ok {
			return trait.DecisionSpec{}, fmt.Errorf("pole metadata for factor %s is not defined", factorID)
		}
		mappings := mappingsForFactor(fg, factorID)
		poles = append(poles, trait.PoleSpec{
			FactorID:     trait.FactorID(factorID),
			LeftPole:     meta.LeftPole,
			RightPole:    meta.RightPole,
			Threshold:    meta.Threshold,
			MaxDeviation: dimensionMaxDeviation(meta, mappings),
		})
	}
	return trait.DecisionSpec{Kind: trait.DecisionKindPoleComposition, Poles: poles}, nil
}

func buildPatternDecision(payload *modeltypology.Payload, spec *modeltypology.RuntimeSpec) (trait.DecisionSpec, error) {
	patternOrder := make([]trait.FactorID, 0, len(spec.FactorGraph.DecisionFactorOrder()))
	for _, factorID := range spec.FactorGraph.DecisionFactorOrder() {
		patternOrder = append(patternOrder, trait.FactorID(factorID))
	}
	patterns := make([]trait.PatternCandidate, 0)
	for _, outcome := range payload.Outcomes {
		if outcome.IsSpecial || outcome.Pattern == "" {
			continue
		}
		patterns = append(patterns, trait.PatternCandidate{
			Code:    outcome.Code,
			Label:   outcome.Name,
			Pattern: patternLevelsByOrder(outcome.Pattern, patternOrder),
		})
	}
	levelRule := trait.LevelRule{LowMax: 3, HighMin: 5}
	if spec.Decision.LevelRule != nil {
		levelRule = trait.LevelRule{
			LowMax:  spec.Decision.LevelRule.LowMax,
			HighMin: spec.Decision.LevelRule.HighMin,
		}
	}
	threshold := spec.Decision.FallbackSimilarityThreshold
	if threshold <= 0 {
		threshold = 0.6
	}
	fallbackCode := spec.Decision.FallbackCode
	return trait.DecisionSpec{
		Kind:              trait.DecisionKindNearestPattern,
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

func patternLevelsByOrder(pattern string, order []trait.FactorID) map[trait.FactorID]string {
	compact := strings.ReplaceAll(pattern, "-", "")
	levels := make(map[trait.FactorID]string, len(order))
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
