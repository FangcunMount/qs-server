package main

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

type traitItem struct {
	Code    string
	Factor  string
	Reverse bool
	Title   string
}

type traitFactor struct {
	Code string
	Name string
}

func normalScoreMap() map[string]float64 {
	return map[string]float64{"1": 1, "2": 2, "3": 3, "4": 4, "5": 5}
}

func reverseScoreMap() map[string]float64 {
	return map[string]float64{"1": 5, "2": 4, "3": 3, "4": 2, "5": 1}
}

func optionScoresForItem(reverse bool) map[string]float64 {
	if reverse {
		return reverseScoreMap()
	}
	return normalScoreMap()
}

func buildTraitQuestionMappings(items []traitItem) []modeltypology.QuestionMapping {
	mappings := make([]modeltypology.QuestionMapping, 0, len(items))
	for _, item := range items {
		mappings = append(mappings, modeltypology.QuestionMapping{
			QuestionCode: item.Code,
			Dimension:    item.Factor,
			Sign:         1,
			OptionScores: optionScoresForItem(item.Reverse),
		})
	}
	return mappings
}

func buildTraitDimensions(factors []traitFactor) map[string]modeltypology.Dimension {
	dimensions := make(map[string]modeltypology.Dimension, len(factors))
	for _, factor := range factors {
		dimensions[factor.Code] = modeltypology.Dimension{
			Code: factor.Code,
			Name: factor.Name,
		}
	}
	return dimensions
}

type traitProfileSeedInput struct {
	Code                 string
	Version              string
	Title                string
	Description          string
	Algorithm            domain.Algorithm
	FactorOrder          []string
	Factors              []traitFactor
	Items                []traitItem
	Source               modeltypology.Source
	ReportCategoryLabel  string
	DetailAdapterKey     modeltypology.DetailAdapterKey
	ReportAdapterKey     modeltypology.ReportAdapterKey
}

func buildTraitProfilePayload(input traitProfileSeedInput) (*modeltypology.Payload, error) {
	factorOrder := append([]string(nil), input.FactorOrder...)
	payload := &modeltypology.Payload{
		Code:                 input.Code,
		Version:              input.Version,
		Title:                input.Title,
		QuestionnaireCode:    input.Code,
		QuestionnaireVersion: input.Version,
		Status:               "published",
		Source:               input.Source,
		Algorithm:            input.Algorithm,
		DimensionOrder:       factorOrder,
		Dimensions:           buildTraitDimensions(input.Factors),
		QuestionMappings:     buildTraitQuestionMappings(input.Items),
		MatchingSpec: modeltypology.MatchingSpec{
			Kind: domain.DecisionKindTraitProfile,
		},
	}
	return enrichTraitProfileRuntime(payload, input.ReportCategoryLabel, input.DetailAdapterKey, input.ReportAdapterKey)
}

func enrichTraitProfileRuntime(
	payload *modeltypology.Payload,
	categoryLabel string,
	detailAdapter modeltypology.DetailAdapterKey,
	reportAdapter modeltypology.ReportAdapterKey,
) (*modeltypology.Payload, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}
	runtime, err := payload.ToRuntimeSpec()
	if err != nil {
		return nil, fmt.Errorf("derive runtime spec: %w", err)
	}

	factors := make(map[string]modeltypology.FactorSpec, len(payload.DimensionOrder))
	for _, dimCode := range payload.DimensionOrder {
		dim := payload.Dimensions[dimCode]
		factors[dimCode] = modeltypology.FactorSpec{
			ID:            dimCode,
			Code:          dimCode,
			Name:          dim.Name,
			Kind:          modeltypology.FactorSpecKindLeaf,
			OptionScoring: modeltypology.FactorOptionScoringStrict,
			Contributions: projectContributions(payload.QuestionMappings, dimCode),
		}
	}

	runtime.FactorGraph.Factors = factors
	runtime.FactorGraph.Roots = append([]string(nil), payload.DimensionOrder...)
	runtime.FactorGraph.DimensionOrder = append([]string(nil), payload.DimensionOrder...)
	runtime.FactorGraph.Dimensions = cloneDimensions(payload.Dimensions)
	runtime.FactorGraph.QuestionMappings = cloneQuestionMappings(payload.QuestionMappings)
	runtime.OutcomeMapping = modeltypology.OutcomeMappingSpec{
		DetailKind:       modeltypology.OutcomeDetailTraitProfile,
		DetailAdapterKey: detailAdapter,
	}
	runtime.Report = modeltypology.ReportSpec{
		Kind:          modeltypology.ReportKindTraitProfile,
		AdapterKey:    reportAdapter,
		CategoryLabel: categoryLabel,
	}

	payload.Runtime = runtime
	return payload, nil
}
