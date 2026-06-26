package profile

import (
	"fmt"

	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"strings"
)

// FactorScore is the scored value of one factor.
type FactorScore struct {
	FactorID FactorID
	Code     string
	Raw      float64
}

// ProfileVector is the scored factor profile before outcome selection.
type ProfileVector struct {
	Scores  map[FactorID]FactorScore
	Ordered []FactorScore
}

// ScoreGraph scores all factors in topological order from an answer sheet.
func ScoreGraph(g FactorGraph, sheet *evaluationinput.AnswerSheet) (ProfileVector, error) {
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

func indexAnswers(sheet *evaluationinput.AnswerSheet) map[string]evaluationinput.Answer {
	answers := make(map[string]evaluationinput.Answer, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers[answer.QuestionCode] = answer
	}
	return answers
}

func scoreLeaf(spec LeafScoringSpec, answers map[string]evaluationinput.Answer) (float64, error) {
	total := spec.Constant
	for _, contribution := range spec.Contributions {
		answer, ok := answers[contribution.QuestionCode]
		if !ok {
			return 0, fmt.Errorf("missing answer for question %s", contribution.QuestionCode)
		}
		value, err := contributionScore(spec.OptionScoring, contribution, answer)
		if err != nil {
			return 0, err
		}
		total += value
	}
	return total, nil
}

func contributionScore(policy OptionScoringPolicy, contribution AnswerContribution, answer evaluationinput.Answer) (float64, error) {
	if len(contribution.OptionScores) > 0 {
		return scoreOptionAnswer(policy, contribution.OptionScores, answer)
	}
	value, err := likertValue(answer)
	if err != nil {
		return 0, err
	}
	return contribution.Sign * value, nil
}

func scoreOptionAnswer(policy OptionScoringPolicy, optionScores map[string]float64, answer evaluationinput.Answer) (float64, error) {
	value := evaluationinput.AnswerValueKey(answer.Value)
	if value != "" {
		if score, ok := optionScores[value]; ok {
			return score, nil
		}
		if score, ok := optionScores[strings.ToUpper(value)]; ok {
			return score, nil
		}
	}
	if policy == OptionScoringCompat && answer.Score > 0 {
		return answer.Score, nil
	}
	return 0, fmt.Errorf("invalid answer for question %s: %v", answer.QuestionCode, answer.Value)
}

func likertValue(answer evaluationinput.Answer) (float64, error) {
	if answer.Score >= 1 && answer.Score <= 5 {
		return answer.Score, nil
	}
	value := evaluationinput.AnswerValueKey(answer.Value)
	if value == "" {
		return 0, fmt.Errorf("invalid answer for question %s", answer.QuestionCode)
	}
	switch value {
	case "1", "2", "3", "4", "5":
		return float64(value[0] - '0'), nil
	default:
		return 0, fmt.Errorf("invalid likert value for question %s: %s", answer.QuestionCode, value)
	}
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
