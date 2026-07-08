package personality

import (
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

type editorDefinitionPayload struct {
	FactorGraph    editorFactorGraphSpec                 `json:"factor_graph"`
	Decision       modeltypology.PersonalityDecisionSpec `json:"decision"`
	SpecialRules   []modeltypology.SpecialRuleSpec       `json:"special_rules,omitempty"`
	OutcomeMapping editorOutcomeMappingSpec              `json:"outcome_mapping"`
	Report         modeltypology.ReportSpec              `json:"report"`
}

type editorFactorGraphSpec struct {
	DimensionOrder   []string                            `json:"dimension_order,omitempty"`
	Dimensions       map[string]modeltypology.Dimension  `json:"dimensions,omitempty"`
	QuestionMappings []editorQuestionMapping             `json:"question_mappings,omitempty"`
	Factors          map[string]modeltypology.FactorSpec `json:"factors,omitempty"`
	Roots            []string                            `json:"roots,omitempty"`
}

type editorQuestionMapping struct {
	QuestionCode string             `json:"question_code"`
	FactorCode   string             `json:"factor_code"`
	Sign         float64            `json:"sign,omitempty"`
	OptionScores map[string]float64 `json:"option_scores,omitempty"`
}

type editorOutcomeMappingSpec struct {
	DetailKind       modeltypology.OutcomeDetailKind `json:"detail_kind,omitempty"`
	DetailAdapterKey modeltypology.DetailAdapterKey  `json:"detail_adapter_key,omitempty"`
	Algorithm        domain.Algorithm                `json:"algorithm,omitempty"`
	Outcomes         []modeltypology.Outcome         `json:"outcomes"`
}

type draftDefinitionEnvelope struct {
	Algorithm domain.Algorithm           `json:"algorithm,omitempty"`
	Outcomes  []modeltypology.Outcome    `json:"outcomes,omitempty"`
	Runtime   *modeltypology.RuntimeSpec `json:"runtime,omitempty"`
}

func buildEditorDefinitionPayload(model *domain.AssessmentModel, payload *modeltypology.Payload, runtime *modeltypology.RuntimeSpec) ([]byte, error) {
	if runtime == nil {
		return nil, nil
	}
	outcomes := resolveEditorOutcomes(runtime, outcomesFromPayload(payload))
	algo := resolvePayloadAlgorithm(model, payload, runtime)
	decision := runtime.Decision
	decision.Kind = normalizeDecisionKind(decision.Kind, algo)
	editor := editorDefinitionPayload{
		FactorGraph: editorFactorGraphSpec{
			DimensionOrder:   append([]string(nil), runtime.FactorGraph.DimensionOrder...),
			Dimensions:       runtime.FactorGraph.Dimensions,
			QuestionMappings: buildEditorQuestionMappings(runtime.FactorGraph, payload),
			Factors:          runtime.FactorGraph.Factors,
			Roots:            append([]string(nil), runtime.FactorGraph.Roots...),
		},
		Decision:     decision,
		SpecialRules: append([]modeltypology.SpecialRuleSpec(nil), runtime.SpecialRules...),
		OutcomeMapping: editorOutcomeMappingSpec{
			DetailKind:       runtime.OutcomeMapping.DetailKind,
			DetailAdapterKey: runtime.OutcomeMapping.DetailAdapterKey,
			Algorithm:        runtime.OutcomeMapping.Algorithm,
			Outcomes:         outcomes,
		},
		Report: runtime.Report,
	}
	if editor.OutcomeMapping.Algorithm == "" && model != nil {
		editor.OutcomeMapping.Algorithm = model.Algorithm
	}
	if editor.OutcomeMapping.Algorithm == "" {
		editor.OutcomeMapping.Algorithm = algo
	}
	return json.Marshal(editor)
}

func normalizeDefinitionPayloadForStorage(data []byte, algorithm domain.Algorithm) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return data, nil
	}
	payload, err := decodeDefinitionPayload(data, algorithm)
	if err != nil {
		return data, nil
	}
	runtime, err := payload.ToRuntimeSpec()
	if err != nil {
		return data, nil
	}
	payload.Runtime = runtime
	algo := firstNonEmptyAlgorithm(payload.Algorithm, algorithm)
	runtime.Decision.Kind = normalizeDecisionKind(runtime.Decision.Kind, algo)
	envelope := draftDefinitionEnvelope{
		Algorithm: firstNonEmptyAlgorithm(payload.Algorithm, algorithm),
		Outcomes:  resolveEditorOutcomes(runtime, append([]modeltypology.Outcome(nil), payload.Outcomes...)),
		Runtime:   runtime,
	}
	return json.Marshal(envelope)
}

func decodeDefinitionPayload(data []byte, algorithm domain.Algorithm) (*modeltypology.Payload, error) {
	var payload modeltypology.Payload
	if err := json.Unmarshal(data, &payload); err == nil {
		if payload.HasExplicitRuntime() || payload.Algorithm != "" || len(payload.Dimensions) > 0 || len(payload.Outcomes) > 0 {
			if payload.Algorithm == "" {
				payload.Algorithm = algorithm
			}
			return &payload, nil
		}
	}
	var editor editorDefinitionPayload
	if err := json.Unmarshal(data, &editor); err == nil {
		if editor.Decision.Kind != "" || len(editor.FactorGraph.Factors) > 0 || len(editor.FactorGraph.QuestionMappings) > 0 {
			return editorPayloadToDomain(&editor, algorithm), nil
		}
	}
	model := &domain.AssessmentModel{
		Algorithm:  algorithm,
		Definition: domain.DefinitionPayload{Data: data},
	}
	decoded, runtime, err := publishing.PersonalityPayloadAndRuntimeSpecFromModel(model)
	if err != nil {
		return nil, err
	}
	if decoded != nil {
		if decoded.Algorithm == "" {
			decoded.Algorithm = algorithm
		}
		if len(decoded.Outcomes) == 0 {
			decoded.Outcomes = outcomesFromEditorOutcomeMapping(data)
		}
		if len(decoded.Outcomes) == 0 && decoded.Runtime != nil {
			decoded.Outcomes = resolveEditorOutcomes(decoded.Runtime, nil)
		}
		return decoded, nil
	}
	return &modeltypology.Payload{Algorithm: algorithm, Runtime: runtime}, nil
}

func editorPayloadToDomain(editor *editorDefinitionPayload, algorithm domain.Algorithm) *modeltypology.Payload {
	if editor == nil {
		return &modeltypology.Payload{Algorithm: algorithm}
	}
	algo := firstNonEmptyAlgorithm(editor.OutcomeMapping.Algorithm, algorithm)
	editor.Decision.Kind = normalizeDecisionKind(editor.Decision.Kind, algo)
	mappings := make([]modeltypology.QuestionMapping, 0, len(editor.FactorGraph.QuestionMappings))
	for _, mapping := range editor.FactorGraph.QuestionMappings {
		mappings = append(mappings, modeltypology.QuestionMapping{
			QuestionCode: mapping.QuestionCode,
			Dimension:    mapping.FactorCode,
			Sign:         mapping.Sign,
			OptionScores: cloneOptionScores(mapping.OptionScores),
		})
	}
	runtime := &modeltypology.RuntimeSpec{
		FactorGraph: modeltypology.FactorGraphSpec{
			DimensionOrder:   append([]string(nil), editor.FactorGraph.DimensionOrder...),
			Dimensions:       editor.FactorGraph.Dimensions,
			QuestionMappings: mappings,
			Factors:          editor.FactorGraph.Factors,
			Roots:            append([]string(nil), editor.FactorGraph.Roots...),
		},
		Decision:     editor.Decision,
		SpecialRules: append([]modeltypology.SpecialRuleSpec(nil), editor.SpecialRules...),
		OutcomeMapping: modeltypology.OutcomeMappingSpec{
			DetailKind:       editor.OutcomeMapping.DetailKind,
			DetailAdapterKey: editor.OutcomeMapping.DetailAdapterKey,
			Algorithm:        editor.OutcomeMapping.Algorithm,
		},
		Report: editor.Report,
	}
	return &modeltypology.Payload{
		Algorithm: firstNonEmptyAlgorithm(editor.OutcomeMapping.Algorithm, algorithm),
		Outcomes:  resolveEditorOutcomes(runtime, append([]modeltypology.Outcome(nil), editor.OutcomeMapping.Outcomes...)),
		Runtime:   runtime,
	}
}

func buildEditorQuestionMappings(factorGraph modeltypology.FactorGraphSpec, payload *modeltypology.Payload) []editorQuestionMapping {
	source := factorGraph.QuestionMappings
	if len(source) == 0 && payload != nil {
		source = payload.QuestionMappings
	}
	if len(source) == 0 {
		source = questionMappingsFromContributions(factorGraph)
	}
	mappings := make([]editorQuestionMapping, 0, len(source))
	for _, mapping := range source {
		factorCode := mapping.Dimension
		if factorCode == "" {
			factorCode = factorCodeForQuestion(factorGraph, mapping.QuestionCode)
		}
		optionScores := cloneOptionScores(mapping.OptionScores)
		if len(optionScores) == 0 {
			optionScores = optionScoresForQuestion(factorGraph, mapping.QuestionCode)
		}
		if len(optionScores) == 0 && mapping.Sign != 0 {
			optionScores = defaultLikertOptionScores()
		}
		mappings = append(mappings, editorQuestionMapping{
			QuestionCode: mapping.QuestionCode,
			FactorCode:   factorCode,
			Sign:         mapping.Sign,
			OptionScores: optionScores,
		})
	}
	return mappings
}

func questionMappingsFromContributions(factorGraph modeltypology.FactorGraphSpec) []modeltypology.QuestionMapping {
	if !factorGraph.HasExplicitFactorGraph() {
		return nil
	}
	mappings := make([]modeltypology.QuestionMapping, 0)
	for factorCode, factor := range factorGraph.Factors {
		for _, contribution := range factor.Contributions {
			mappings = append(mappings, modeltypology.QuestionMapping{
				QuestionCode: contribution.QuestionCode,
				Dimension:    factorCode,
				Sign:         contribution.Sign,
				OptionScores: cloneOptionScores(contribution.OptionScores),
			})
		}
	}
	return mappings
}

func factorCodeForQuestion(factorGraph modeltypology.FactorGraphSpec, questionCode string) string {
	for factorCode, factor := range factorGraph.Factors {
		for _, contribution := range factor.Contributions {
			if contribution.QuestionCode == questionCode {
				return firstNonEmpty(factor.Code, factor.ID, factorCode)
			}
		}
	}
	return ""
}

func optionScoresForQuestion(factorGraph modeltypology.FactorGraphSpec, questionCode string) map[string]float64 {
	for _, factor := range factorGraph.Factors {
		for _, contribution := range factor.Contributions {
			if contribution.QuestionCode == questionCode && len(contribution.OptionScores) > 0 {
				return cloneOptionScores(contribution.OptionScores)
			}
		}
	}
	return nil
}

func outcomesFromPayload(payload *modeltypology.Payload) []modeltypology.Outcome {
	if payload == nil || len(payload.Outcomes) == 0 {
		return nil
	}
	return append([]modeltypology.Outcome(nil), payload.Outcomes...)
}

func resolveEditorOutcomes(runtime *modeltypology.RuntimeSpec, outcomes []modeltypology.Outcome) []modeltypology.Outcome {
	if len(outcomes) > 0 {
		return outcomes
	}
	if runtime == nil || !isTraitProfileDecision(runtime.Decision.Kind) {
		return nil
	}
	return synthesizeTraitProfileOutcomes(runtime.FactorGraph)
}

func synthesizeTraitProfileOutcomes(graph modeltypology.FactorGraphSpec) []modeltypology.Outcome {
	order := append([]string(nil), graph.DimensionOrder...)
	if len(order) == 0 {
		order = append(order, graph.Roots...)
	}
	outcomes := make([]modeltypology.Outcome, 0, len(order))
	seen := make(map[string]struct{}, len(order))
	for _, code := range order {
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		outcomes = append(outcomes, modeltypology.Outcome{
			Code: code,
			Name: traitProfileOutcomeName(code, graph),
		})
	}
	if len(outcomes) > 0 {
		return outcomes
	}
	for code, factor := range graph.Factors {
		outcomeCode := firstNonEmpty(factor.Code, factor.ID, code)
		if outcomeCode == "" {
			continue
		}
		if _, exists := seen[outcomeCode]; exists {
			continue
		}
		seen[outcomeCode] = struct{}{}
		outcomes = append(outcomes, modeltypology.Outcome{
			Code: outcomeCode,
			Name: firstNonEmpty(factor.Name, factor.Code, factor.ID, outcomeCode),
		})
	}
	return outcomes
}

func traitProfileOutcomeName(code string, graph modeltypology.FactorGraphSpec) string {
	if dim, ok := graph.Dimensions[code]; ok && dim.Name != "" {
		return dim.Name
	}
	if factor, ok := graph.Factors[code]; ok {
		return firstNonEmpty(factor.Name, factor.Code, factor.ID, code)
	}
	return code
}

func isTraitProfileDecision(kind domain.DecisionKind) bool {
	return kind == domain.DecisionKindTraitProfile
}

func outcomesFromEditorOutcomeMapping(data []byte) []modeltypology.Outcome {
	var editor editorDefinitionPayload
	if err := json.Unmarshal(data, &editor); err != nil {
		return nil
	}
	if len(editor.OutcomeMapping.Outcomes) == 0 {
		return nil
	}
	return append([]modeltypology.Outcome(nil), editor.OutcomeMapping.Outcomes...)
}

func defaultLikertOptionScores() map[string]float64 {
	return map[string]float64{"1": 1, "2": 2, "3": 3, "4": 4, "5": 5}
}

func cloneOptionScores(source map[string]float64) map[string]float64 {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]float64, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func firstNonEmptyAlgorithm(values ...domain.Algorithm) domain.Algorithm {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resolvePayloadAlgorithm(model *domain.AssessmentModel, payload *modeltypology.Payload, runtime *modeltypology.RuntimeSpec) domain.Algorithm {
	if payload != nil && payload.Algorithm != "" {
		return payload.Algorithm
	}
	if runtime != nil && runtime.OutcomeMapping.Algorithm != "" {
		return runtime.OutcomeMapping.Algorithm
	}
	if model != nil {
		return model.Algorithm
	}
	return ""
}

func normalizeDecisionKind(kind domain.DecisionKind, algorithm domain.Algorithm) domain.DecisionKind {
	if isEditorDecisionKind(kind) {
		return kind
	}
	return kind
}

func isEditorDecisionKind(kind domain.DecisionKind) bool {
	switch kind {
	case domain.DecisionKindPoleComposition,
		domain.DecisionKindNearestPattern,
		domain.DecisionKindTraitProfile:
		return true
	default:
		return false
	}
}
