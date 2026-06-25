package evaluation

import (
	"testing"

	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
)

func TestResolveMBTIPreference_tieAtThresholdPrefersLeftPole(t *testing.T) {
	meta := rulesetmbti.DimensionSnapshot{
		Code:      "EI",
		LeftPole:  "I",
		RightPole: "E",
		Constant:  30,
		Threshold: 24,
	}
	mappings := []rulesetmbti.QuestionMappingSnapshot{
		{QuestionCode: "MBTI_Q03", Dimension: "EI", Sign: -1},
	}

	preference, strength := resolveMBTIPreference(meta, 24, mappings)
	if preference != "I" {
		t.Fatalf("preference = %s, want I", preference)
	}
	if strength != 0 {
		t.Fatalf("strength = %.2f, want 0", strength)
	}
}

func TestResolveMBTIPreference_aboveThresholdPrefersRightPole(t *testing.T) {
	meta := rulesetmbti.DimensionSnapshot{
		Code:      "EI",
		LeftPole:  "I",
		RightPole: "E",
		Constant:  30,
		Threshold: 24,
	}
	mappings := []rulesetmbti.QuestionMappingSnapshot{
		{QuestionCode: "MBTI_Q03", Dimension: "EI", Sign: -1},
	}

	preference, strength := resolveMBTIPreference(meta, 25, mappings)
	if preference != "E" {
		t.Fatalf("preference = %s, want E", preference)
	}
	if strength <= 0 {
		t.Fatalf("strength = %.2f, want > 0", strength)
	}
}

func TestResolveMBTIPreference_belowThresholdPrefersLeftPole(t *testing.T) {
	meta := rulesetmbti.DimensionSnapshot{
		Code:      "JP",
		LeftPole:  "J",
		RightPole: "P",
		Constant:  18,
		Threshold: 24,
	}
	mappings := []rulesetmbti.QuestionMappingSnapshot{
		{QuestionCode: "MBTI_Q01", Dimension: "JP", Sign: 1},
	}

	preference, _ := resolveMBTIPreference(meta, 20, mappings)
	if preference != "J" {
		t.Fatalf("preference = %s, want J", preference)
	}
}

func TestMBTIAnswerLikertValue_prefersScore(t *testing.T) {
	value, err := mbtiAnswerLikertValue(Answer{
		QuestionCode: "MBTI_Q01",
		Value:        "1",
		Score:        4,
	})
	if err != nil {
		t.Fatalf("mbtiAnswerLikertValue: %v", err)
	}
	if value != 4 {
		t.Fatalf("value = %.0f, want 4", value)
	}
}

func TestMBTIAnswerLikertValue_fromOptionCode(t *testing.T) {
	value, err := mbtiAnswerLikertValue(Answer{
		QuestionCode: "MBTI_Q01",
		Value:        "5",
	})
	if err != nil {
		t.Fatalf("mbtiAnswerLikertValue: %v", err)
	}
	if value != 5 {
		t.Fatalf("value = %.0f, want 5", value)
	}
}
