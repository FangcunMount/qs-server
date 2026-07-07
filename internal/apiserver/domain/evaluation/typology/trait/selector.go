package trait

import (
	"fmt"
	"math"
	"strings"
)

// SelectOutcome 应用decision spec 到 scored 画像 vector。
func SelectOutcome(vector ProfileVector, spec DecisionSpec) (OutcomeCandidate, error) {
	switch spec.Kind {
	case DecisionKindPoleComposition:
		return selectPoleComposition(vector, spec)
	case DecisionKindTraitProfile:
		return selectTraitProfile(vector)
	case DecisionKindNearestPattern:
		return selectNearestPattern(vector, spec)
	default:
		return OutcomeCandidate{}, fmt.Errorf("unsupported decision kind %s", spec.Kind)
	}
}

func selectPoleComposition(vector ProfileVector, spec DecisionSpec) (OutcomeCandidate, error) {
	if len(spec.Poles) == 0 {
		return OutcomeCandidate{}, fmt.Errorf("pole composition requires pole specs")
	}
	letters := make([]string, 0, len(spec.Poles))
	var strengthSum float64
	for _, pole := range spec.Poles {
		score, ok := vector.Scores[pole.FactorID]
		if !ok {
			return OutcomeCandidate{}, fmt.Errorf("missing factor score for %s", pole.FactorID)
		}
		threshold := pole.Threshold
		if threshold == 0 {
			threshold = 24
		}
		preference := pole.LeftPole
		if score.Raw > threshold {
			preference = pole.RightPole
		}
		letters = append(letters, preference)
		strengthSum += PoleStrength(score.Raw, pole)
	}
	code := strings.Join(letters, "")
	match := 0.0
	if len(spec.Poles) > 0 {
		match = strengthSum / float64(len(spec.Poles))
	}
	return OutcomeCandidate{
		Code:       code,
		MatchScore: match,
	}, nil
}

// ResolvePole 映射原始 因子 score 到 pole preference 和 strength。
func ResolvePole(pole PoleSpec, raw float64) (preference string, strength float64) {
	threshold := pole.Threshold
	if threshold == 0 {
		threshold = 24
	}
	preference = pole.LeftPole
	if raw > threshold {
		preference = pole.RightPole
	}
	return preference, PoleStrength(raw, pole)
}

// PoleStrength 计算preference strength using pole deviation 元数据。
func PoleStrength(raw float64, pole PoleSpec) float64 {
	threshold := pole.Threshold
	if threshold == 0 {
		threshold = 24
	}
	maxDeviation := pole.MaxDeviation
	if maxDeviation <= 0 {
		maxDeviation = threshold
	}
	if maxDeviation <= 0 {
		return 0
	}
	strength := math.Abs(raw-threshold) / maxDeviation * 100
	if strength > 100 {
		return 100
	}
	return strength
}

// LevelForScore 映射原始分 到 L/M/H using 配置化 等级 rule。
func LevelForScore(raw float64, rule LevelRule) string {
	lowMax := rule.LowMax
	if lowMax == 0 {
		lowMax = 3
	}
	highMin := rule.HighMin
	if highMin == 0 {
		highMin = 5
	}
	switch {
	case raw <= lowMax:
		return "L"
	case raw >= highMin:
		return "H"
	default:
		return "M"
	}
}

func selectNearestPattern(vector ProfileVector, spec DecisionSpec) (OutcomeCandidate, error) {
	if len(spec.PatternOrder) == 0 {
		return OutcomeCandidate{}, fmt.Errorf("nearest_pattern requires pattern order")
	}
	if len(spec.Patterns) == 0 {
		return OutcomeCandidate{}, fmt.Errorf("nearest_pattern requires pattern candidates")
	}
	actual := make([]string, 0, len(spec.PatternOrder))
	for _, factorID := range spec.PatternOrder {
		score, ok := vector.Scores[factorID]
		if !ok {
			return OutcomeCandidate{}, fmt.Errorf("missing factor score for %s", factorID)
		}
		actual = append(actual, LevelForScore(score.Raw, spec.LevelRule))
	}

	var (
		best        PatternCandidate
		bestScore   = math.Inf(-1)
		hasBest     bool
		maxDistance = float64(len(actual) * 2)
	)
	for _, candidate := range spec.Patterns {
		expected := patternLevels(candidate, spec.PatternOrder)
		if len(expected) != len(actual) {
			continue
		}
		distance := 0
		for i := range actual {
			distance += absLevelDelta(actual[i], expected[i])
		}
		similarity := 1 - float64(distance)/maxDistance
		if !hasBest || similarity > bestScore {
			best = candidate
			bestScore = similarity
			hasBest = true
		}
	}
	if !hasBest {
		return OutcomeCandidate{}, fmt.Errorf("no valid pattern candidates configured")
	}
	return OutcomeCandidate{
		Code:       best.Code,
		Label:      best.Label,
		MatchScore: bestScore,
	}, nil
}

func patternLevels(candidate PatternCandidate, order []FactorID) []string {
	levels := make([]string, 0, len(order))
	for _, factorID := range order {
		levels = append(levels, strings.ToUpper(strings.TrimSpace(candidate.Pattern[factorID])))
	}
	return levels
}

func absLevelDelta(actual, expected string) int {
	return absInt(levelValue(actual) - levelValue(expected))
}

func levelValue(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "L":
		return 0
	case "M":
		return 1
	case "H":
		return 2
	default:
		return 1
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func selectTraitProfile(vector ProfileVector) (OutcomeCandidate, error) {
	traits := make(map[FactorID]float64, len(vector.Scores))
	for id, score := range vector.Scores {
		traits[id] = score.Raw
	}
	return OutcomeCandidate{
		TraitScores: traits,
	}, nil
}
