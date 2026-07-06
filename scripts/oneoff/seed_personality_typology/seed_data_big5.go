package main

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

const big5QuestionnairePath = "scripts/oneoff/seed_personality_typology/data/big5_ipip_50_questionnaire.json"

func buildBig5Payload() (*modeltypology.Payload, error) {
	seed, err := loadQuestionnaireSeed(big5QuestionnairePath)
	if err != nil {
		return nil, err
	}
	return buildTraitProfilePayload(traitProfileSeedInput{
		Code:                seed.Code,
		Version:             seed.Version,
		Title:               seed.Title,
		Algorithm:           domain.AlgorithmBigFive,
		FactorOrder:         factorOrderFromSeed(seed),
		Factors:             traitFactorsFromSeed(seed),
		Items:               traitItemsFromSeed(seed),
		ReportCategoryLabel: "大五人格",
		DetailAdapterKey:    modeltypology.DetailAdapterBigFive,
		ReportAdapterKey:    modeltypology.ReportAdapterBigFive,
		Source: modeltypology.Source{
			SourceSite:    "https://ipip.ori.org/",
			Attribution:   "IPIP (International Personality Item Pool)",
			License:       "Public Domain",
			NonCommercial: false,
		},
	})
}
