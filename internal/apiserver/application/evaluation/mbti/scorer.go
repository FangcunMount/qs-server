package mbti

import (
	"fmt"
	"math"
	"strings"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Scorer struct{}

func NewScorer() Scorer {
	return Scorer{}
}

func (Scorer) Score(model *port.MBTIModelSnapshot, answerSheet *port.AnswerSheetSnapshot) (ResultDetail, error) {
	if model == nil {
		return ResultDetail{}, fmt.Errorf("mbti model is required")
	}
	if answerSheet == nil {
		return ResultDetail{}, fmt.Errorf("answer sheet is required")
	}

	answerByQuestion := make(map[string]port.AnswerSnapshot, len(answerSheet.Answers))
	for _, answer := range answerSheet.Answers {
		answerByQuestion[answer.QuestionCode] = answer
	}

	dimensionScores := make(map[string]float64, len(model.DimensionOrder))
	for _, dimCode := range model.DimensionOrder {
		meta := model.Dimensions[dimCode]
		dimensionScores[dimCode] = meta.Constant
	}

	for _, mapping := range model.QuestionMappings {
		answer, ok := answerByQuestion[mapping.QuestionCode]
		if !ok {
			return ResultDetail{}, fmt.Errorf("missing mbti answer for question %s", mapping.QuestionCode)
		}
		value, err := answerLikertValue(answer)
		if err != nil {
			return ResultDetail{}, err
		}
		dimensionScores[mapping.Dimension] += mapping.Sign * value
	}

	dimensions := make([]DimensionResult, 0, len(model.DimensionOrder))
	typeLetters := make([]string, 0, len(model.DimensionOrder))
	var strengthSum float64

	for _, dimCode := range model.DimensionOrder {
		meta := model.Dimensions[dimCode]
		raw := dimensionScores[dimCode]
		preference, strength := resolvePreference(meta, raw, model.QuestionMappings)
		dimensions = append(dimensions, DimensionResult{
			Code:       dimCode,
			Name:       meta.Name,
			LeftPole:   meta.LeftPole,
			RightPole:  meta.RightPole,
			RawScore:   raw,
			Preference: preference,
			Strength:   strength,
		})
		typeLetters = append(typeLetters, preference)
		strengthSum += strength
	}

	typeCode := strings.Join(typeLetters, "")
	profile, ok := model.FindTypeProfile(typeCode)
	if !ok {
		return ResultDetail{}, fmt.Errorf("mbti type profile not found for %s", typeCode)
	}

	matchPercent := 0.0
	if len(dimensions) > 0 {
		matchPercent = strengthSum / float64(len(dimensions))
	}

	return ResultDetail{
		TypeCode:     typeCode,
		TypeName:     profile.TypeName,
		OneLiner:     profile.OneLiner,
		MatchPercent: matchPercent,
		ImageURL:     profile.ImageURL,
		Dimensions:   dimensions,
		Profile:      profile,
		Source:       model.Source,
	}, nil
}

func resolvePreference(
	meta port.MBTIDimensionSnapshot,
	raw float64,
	mappings []port.MBTIQuestionMappingSnapshot,
) (string, float64) {
	threshold := meta.Threshold
	if threshold == 0 {
		threshold = 24
	}
	preference := meta.LeftPole
	if raw > threshold {
		preference = meta.RightPole
	}
	maxDeviation := dimensionMaxDeviation(meta, mappings)
	strength := 0.0
	if maxDeviation > 0 {
		strength = math.Abs(raw-threshold) / maxDeviation * 100
		if strength > 100 {
			strength = 100
		}
	}
	return preference, strength
}

func dimensionMaxDeviation(meta port.MBTIDimensionSnapshot, mappings []port.MBTIQuestionMappingSnapshot) float64 {
	minScore := meta.Constant
	maxScore := meta.Constant
	for _, mapping := range mappings {
		if mapping.Dimension != meta.Code {
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
	return math.Max(threshold-minScore, maxScore-threshold)
}

func answerLikertValue(answer port.AnswerSnapshot) (float64, error) {
	if answer.Score >= 1 && answer.Score <= 5 {
		return answer.Score, nil
	}
	value := answerValueKey(answer.Value)
	if value == "" {
		return 0, fmt.Errorf("invalid mbti answer for question %s: %v", answer.QuestionCode, answer.Value)
	}
	switch value {
	case "1", "2", "3", "4", "5":
		return float64(value[0] - '0'), nil
	default:
		return 0, fmt.Errorf("invalid mbti likert value for question %s: %s", answer.QuestionCode, value)
	}
}

func answerValueKey(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	case []string:
		if len(value) == 0 {
			return ""
		}
		return strings.TrimSpace(value[0])
	case []any:
		if len(value) == 0 {
			return ""
		}
		return answerValueKey(value[0])
	default:
		if raw == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}
