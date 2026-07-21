package classification

import (
	"fmt"
	"math"
	"strings"
)

// Answer is a neutral answer value for classification scoring.
type Answer struct {
	QuestionCode string
	Score        float64
	Value        any
}

// AnswerSheet is a neutral answer sheet for classification scoring.
type AnswerSheet struct {
	Answers []Answer
}

// FactorScore is a scored value of a factor.
type FactorScore struct {
	FactorID FactorID
	Code     string
	Raw      float64
}

// ProfileVector is a scored factor profile before outcome selection.
type ProfileVector struct {
	Scores  map[FactorID]FactorScore
	Ordered []FactorScore
}

// ScoreGraph computes all factors in topological order from answers.
func ScoreGraph(g FactorGraph, sheet *AnswerSheet) (ProfileVector, error) {
	if sheet == nil {
		return ProfileVector{}, fmt.Errorf("answer sheet is required")
	}
	order, err := g.TopologicalOrder()
	if err != nil {
		return ProfileVector{}, err
	}
	answers := indexAnswers(sheet)
	scores := make(map[FactorID]FactorScore, len(order))
	ordered := make([]FactorScore, 0, len(order))
	for _, factorID := range order {
		factor := g.Factors[factorID]
		var raw float64
		switch factor.Kind {
		case FactorKindLeaf:
			spec := g.LeafSpecs[factorID]
			raw, err = scoreLeaf(spec, answers)
			if err != nil {
				return ProfileVector{}, fmt.Errorf("score leaf %s: %w", factorID, err)
			}
		case FactorKindComposite:
			raw, err = aggregateChildren(factor, scores)
			if err != nil {
				return ProfileVector{}, fmt.Errorf("aggregate %s: %w", factorID, err)
			}
		default:
			return ProfileVector{}, fmt.Errorf("unsupported factor kind %s", factor.Kind)
		}
		score := FactorScore{FactorID: factorID, Code: factor.Code, Raw: raw}
		scores[factorID] = score
		ordered = append(ordered, score)
	}
	return ProfileVector{Scores: scores, Ordered: ordered}, nil
}

func indexAnswers(sheet *AnswerSheet) map[string]Answer {
	answers := make(map[string]Answer, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers[answer.QuestionCode] = answer
	}
	return answers
}

func scoreLeaf(spec LeafScoringSpec, answers map[string]Answer) (float64, error) {
	// MissingAnswerPolicyFor(typology, typology_leaf) == fail.
	total := spec.Constant
	for _, contribution := range spec.Contributions {
		answer, ok := answers[contribution.QuestionCode]
		if !ok {
			return 0, fmt.Errorf("missing answer for question %s", contribution.QuestionCode)
		}
		value, err := CalculateQuestionContribution(contribution, answer)
		if err != nil {
			return 0, err
		}
		total += value
	}
	return total, nil
}

// CalculateQuestionContribution calculates one explicitly configured question contribution.
func CalculateQuestionContribution(contribution AnswerContribution, answer Answer) (float64, error) {
	switch contribution.ScoringMode {
	case "":
		return 0, fmt.Errorf("scoring mode is required for question %s", contribution.QuestionCode)
	case QuestionScoringModeQuestionScore:
		return explicitContributionScore(contribution, answer.Score)
	case QuestionScoringModeOptionOverride:
		if len(contribution.OptionScores) == 0 {
			return 0, fmt.Errorf("option scores are required for question %s", contribution.QuestionCode)
		}
		base, err := scoreOptionAnswer(contribution.OptionScores, answer)
		if err != nil {
			return 0, err
		}
		return explicitContributionScore(contribution, base)
	default:
		return 0, fmt.Errorf("unsupported scoring mode %s for question %s", contribution.ScoringMode, contribution.QuestionCode)
	}
}

func explicitContributionScore(contribution AnswerContribution, base float64) (float64, error) {
	if math.IsNaN(base) || math.IsInf(base, 0) {
		return 0, fmt.Errorf("answer score for question %s must be finite", contribution.QuestionCode)
	}
	sign := contribution.Sign
	if sign == 0 {
		sign = 1
	}
	if sign != 1 && sign != -1 {
		return 0, fmt.Errorf("sign for question %s must be 1 or -1", contribution.QuestionCode)
	}
	weight := contribution.Weight
	if weight == 0 {
		weight = 1
	}
	if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
		return 0, fmt.Errorf("weight for question %s must be finite and greater than zero", contribution.QuestionCode)
	}
	return base * sign * weight, nil
}

func scoreOptionAnswer(optionScores map[string]float64, answer Answer) (float64, error) {
	value := answerValueKey(answer.Value)
	if value != "" {
		if score, ok := optionScores[value]; ok {
			return score, nil
		}
		if score, ok := optionScores[strings.ToUpper(value)]; ok {
			return score, nil
		}
	}
	return 0, fmt.Errorf("invalid answer for question %s: %v", answer.QuestionCode, answer.Value)
}

func aggregateChildren(factor PersonalityFactor, scores map[FactorID]FactorScore) (float64, error) {
	if len(factor.Children) == 0 {
		return 0, fmt.Errorf("composite factor %s has no children", factor.ID)
	}
	childValues := make([]float64, 0, len(factor.Children))
	for _, childID := range factor.Children {
		childScore, ok := scores[childID]
		if !ok {
			return 0, fmt.Errorf("missing child score for %s", childID)
		}
		childValues = append(childValues, childScore.Raw)
	}
	switch factor.Aggregation {
	case AggregationSum, "":
		var total float64
		for _, value := range childValues {
			total += value
		}
		return total, nil
	case AggregationAvg:
		var total float64
		for _, value := range childValues {
			total += value
		}
		return total / float64(len(childValues)), nil
	case AggregationWeightedAvg:
		var weighted float64
		var weightSum float64
		for _, childID := range factor.Children {
			weight, ok := factor.Weights[childID]
			if !ok {
				return 0, fmt.Errorf("missing weight for child %s", childID)
			}
			childScore, ok := scores[childID]
			if !ok {
				return 0, fmt.Errorf("missing weighted child score for %s", childID)
			}
			weighted += childScore.Raw * weight
			weightSum += weight
		}
		if weightSum == 0 {
			return 0, fmt.Errorf("weighted factor %s has zero total weight", factor.ID)
		}
		return weighted / weightSum, nil
	default:
		return 0, fmt.Errorf("unsupported aggregation %s", factor.Aggregation)
	}
}

func answerValueKey(value any) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	if arr, ok := value.([]string); ok && len(arr) > 0 {
		return strings.TrimSpace(arr[0])
	}
	return ""
}
