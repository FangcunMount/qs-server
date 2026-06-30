package main

import (
	"encoding/json"
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
)

func buildMBTIPayload() (*modeltypology.Payload, error) {
	legacy, err := rulesetInfra.LoadDefaultMBTILegacyModel()
	if err != nil {
		return nil, fmt.Errorf("load mbti seed: %w", err)
	}
	payload := modeltypology.FromMBTI(legacy)
	if payload == nil {
		return nil, fmt.Errorf("mbti payload is nil")
	}
	return enrichPayloadWithExplicitRuntime(payload)
}

func buildSBTIPayload() (*modeltypology.Payload, error) {
	legacy, err := rulesetInfra.LoadDefaultSBTILegacyModel()
	if err != nil {
		return nil, fmt.Errorf("load sbti seed: %w", err)
	}
	payload := modeltypology.FromSBTI(legacy)
	if payload == nil {
		return nil, fmt.Errorf("sbti payload is nil")
	}
	return enrichPayloadWithExplicitRuntime(payload)
}

func enrichPayloadWithExplicitRuntime(payload *modeltypology.Payload) (*modeltypology.Payload, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}
	runtime, err := payload.ToRuntimeSpec()
	if err != nil {
		return nil, fmt.Errorf("derive runtime spec: %w", err)
	}

	optionScoring := modeltypology.FactorOptionScoringStrict
	switch payload.Algorithm {
	case domain.AlgorithmSBTI:
		optionScoring = modeltypology.FactorOptionScoringCompat
	}

	factors := make(map[string]modeltypology.FactorSpec, len(payload.DimensionOrder))
	for _, dimCode := range payload.DimensionOrder {
		dim := payload.Dimensions[dimCode]
		factors[dimCode] = modeltypology.FactorSpec{
			ID:            dimCode,
			Code:          dimCode,
			Name:          dim.Name,
			Kind:          modeltypology.FactorSpecKindLeaf,
			OptionScoring: optionScoring,
			Constant:      dim.Constant,
			Contributions: projectContributions(payload.QuestionMappings, dimCode),
		}
	}

	runtime.FactorGraph.Factors = factors
	runtime.FactorGraph.Roots = append([]string(nil), payload.DimensionOrder...)
	runtime.FactorGraph.DimensionOrder = append([]string(nil), payload.DimensionOrder...)
	runtime.FactorGraph.Dimensions = cloneDimensions(payload.Dimensions)
	runtime.FactorGraph.QuestionMappings = cloneQuestionMappings(payload.QuestionMappings)

	switch payload.Algorithm {
	case domain.AlgorithmMBTI:
		runtime.OutcomeMapping = modeltypology.OutcomeMappingSpec{
			DetailKind:       modeltypology.OutcomeDetailPersonalityType,
			DetailAdapterKey: modeltypology.DetailAdapterMBTI,
		}
		runtime.Report = modeltypology.ReportSpec{
			Kind:          modeltypology.ReportKindPersonalityType,
			AdapterKey:    modeltypology.ReportAdapterMBTI,
			CategoryLabel: "MBTI",
		}
	case domain.AlgorithmSBTI:
		runtime.OutcomeMapping = modeltypology.OutcomeMappingSpec{
			DetailKind:       modeltypology.OutcomeDetailPersonalityType,
			DetailAdapterKey: modeltypology.DetailAdapterSBTI,
		}
		runtime.Report = modeltypology.ReportSpec{
			Kind:          modeltypology.ReportKindPersonalityType,
			AdapterKey:    modeltypology.ReportAdapterSBTI,
			CategoryLabel: "SBTI",
		}
	}

	payload.Runtime = runtime
	return payload, nil
}

func validatePayloadAgainstQuestionnaire(payload *modeltypology.Payload, seed questionnaireSeedFile) error {
	runtime, err := payload.ToRuntimeSpec()
	if err != nil {
		return fmt.Errorf("validate runtime spec: %w", err)
	}
	questionnaire := modeltypology.QuestionnaireSnapshot{
		Code:      seed.Code,
		Version:   seed.Version,
		Questions: questionnaireQuestionsFromSeed(seed),
	}
	issues := modeltypology.ValidateRuntimeSpecForPublishWithContext(runtime, questionnaire, modeltypology.RuntimeSpecValidationContext{
		Algorithm: payload.Algorithm,
		Outcomes:  payload.Outcomes,
	})
	if len(issues) > 0 {
		return fmt.Errorf("publish validation failed: %s", issues[0].Message)
	}
	return nil
}

func questionnaireQuestionsFromSeed(seed questionnaireSeedFile) []modeltypology.QuestionSnapshot {
	questions := make([]modeltypology.QuestionSnapshot, 0, len(seed.Questions))
	for _, item := range seed.Questions {
		optionCodes := make([]string, 0, len(item.Options))
		for _, opt := range item.Options {
			optionCodes = append(optionCodes, opt.Code)
		}
		questions = append(questions, modeltypology.QuestionSnapshot{
			Code:        item.Code,
			OptionCodes: optionCodes,
		})
	}
	return questions
}

func projectContributions(mappings []modeltypology.QuestionMapping, dimCode string) []modeltypology.FactorContributionSpec {
	contributions := make([]modeltypology.FactorContributionSpec, 0)
	for _, mapping := range mappings {
		if mapping.Dimension != dimCode {
			continue
		}
		contributions = append(contributions, modeltypology.FactorContributionSpec{
			QuestionCode: mapping.QuestionCode,
			Sign:         mapping.Sign,
			OptionScores: normalizeOptionScores(mapping.OptionScores),
		})
	}
	return contributions
}

func cloneDimensions(source map[string]modeltypology.Dimension) map[string]modeltypology.Dimension {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]modeltypology.Dimension, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneQuestionMappings(source []modeltypology.QuestionMapping) []modeltypology.QuestionMapping {
	if len(source) == 0 {
		return nil
	}
	cloned := make([]modeltypology.QuestionMapping, len(source))
	for i, mapping := range source {
		cloned[i] = modeltypology.QuestionMapping{
			QuestionCode: mapping.QuestionCode,
			Dimension:    mapping.Dimension,
			Sign:         mapping.Sign,
			OptionScores: normalizeOptionScores(mapping.OptionScores),
		}
	}
	return cloned
}

func normalizeOptionScores(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	numeric := map[string]float64{}
	for _, code := range []string{"1", "2", "3", "4", "5"} {
		if value, ok := source[code]; ok {
			numeric[code] = value
		}
	}
	if len(numeric) > 0 {
		return numeric
	}
	return cloneOptionScores(source)
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

func payloadDefinitionBytes(payload *modeltypology.Payload) ([]byte, error) {
	runtime, err := payload.ToRuntimeSpec()
	if err != nil {
		return nil, fmt.Errorf("derive runtime spec: %w", err)
	}
	data, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime spec: %w", err)
	}
	return data, nil
}

func fullPayloadDefinitionBytes(payload *modeltypology.Payload) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return data, nil
}
