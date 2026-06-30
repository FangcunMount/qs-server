package main

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
)

const mbti93QuestionnairePath = "scripts/oneoff/seed_personality_typology/data/mbti_fc_93_questionnaire.json"

func buildMBTI93Payload() (*modeltypology.Payload, error) {
	seed, err := loadQuestionnaireSeed(mbti93QuestionnairePath)
	if err != nil {
		return nil, err
	}
	outcomes, err := mbti16OutcomesFromLegacy()
	if err != nil {
		return nil, err
	}
	payload := &modeltypology.Payload{
		Code:                 seed.Code,
		Version:              seed.Version,
		Title:                seed.Title,
		QuestionnaireCode:    seed.Code,
		QuestionnaireVersion: seed.Version,
		Status:               "published",
		Algorithm:            domain.AlgorithmMBTI,
		DimensionOrder:       factorOrderFromSeed(seed),
		Dimensions:           mbtiDimensionsFromSeed(seed),
		QuestionMappings:     questionMappingsFromSeed(seed),
		Outcomes:             outcomes,
		MatchingSpec: modeltypology.MatchingSpec{
			Kind: domain.DecisionKindPoleComposition,
		},
		Source: modeltypology.Source{
			Attribution:   "原创强迫选择题库，兼容 MBTI 四维结构",
			License:       "Internal / Self-exploration Use",
			NonCommercial: true,
		},
	}
	return enrichPayloadWithExplicitRuntime(payload)
}

func mbti16OutcomesFromLegacy() ([]modeltypology.Outcome, error) {
	legacy, err := rulesetInfra.LoadDefaultMBTILegacyModel()
	if err != nil {
		return nil, fmt.Errorf("load mbti legacy outcomes: %w", err)
	}
	payload := modeltypology.FromMBTI(legacy)
	if payload == nil {
		return nil, fmt.Errorf("mbti legacy payload is nil")
	}
	return append([]modeltypology.Outcome(nil), payload.Outcomes...), nil
}
