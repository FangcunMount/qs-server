package main

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

const enneagramQuestionnairePath = "scripts/oneoff/seed_personality_typology/data/enneagram_45_questionnaire.json"

func buildEnneagramPayload() (*modeltypology.Payload, error) {
	seed, err := loadQuestionnaireSeed(enneagramQuestionnairePath)
	if err != nil {
		return nil, err
	}
	return buildTraitProfilePayload(traitProfileSeedInput{
		Code:                seed.Code,
		Version:             seed.Version,
		Title:               seed.Title,
		Algorithm:           domain.AlgorithmPersonalityTypology,
		FactorOrder:         factorOrderFromSeed(seed),
		Factors:             traitFactorsFromSeed(seed),
		Items:               traitItemsFromSeed(seed),
		ReportCategoryLabel: "九型人格",
		DetailAdapterKey:    modeltypology.DetailAdapterTraitProfile,
		ReportAdapterKey:    modeltypology.ReportAdapterTraitProfile,
		Source: modeltypology.Source{
			Attribution:   "原创自研题库，基于公开九型人格类型结构",
			License:       "Internal / Entertainment Use",
			NonCommercial: true,
		},
	})
}
