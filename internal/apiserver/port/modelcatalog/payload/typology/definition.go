package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

// DefinitionFromPayload materializes the target typology definition model from
// the legacy/runtime payload without changing the payload contract.
func DefinitionFromPayload(payload []byte, algorithm binding.Algorithm) (*definition.Definition, error) {
	decoded, runtime, err := PayloadAndRuntimeSpecFromDefinition(payload, algorithm)
	if err != nil {
		return nil, err
	}
	return DefinitionFromRuntime(decoded, runtime), nil
}

// DefinitionFromRuntime projects typology runtime configuration into the target
// modelcatalog definition layers.
func DefinitionFromRuntime(payload *Payload, runtime *RuntimeSpec) *definition.Definition {
	if runtime == nil {
		return &definition.Definition{}
	}
	measure := runtime.CanonicalMeasureSpec()
	outcomes := conclusionOutcomes(payload)
	return &definition.Definition{
		Measure: measure,
		Conclusions: []conclusion.Conclusion{
			conclusion.TypeConclusion{
				FactorCodes: factorCodes(measure),
				Outcomes:    outcomes,
			},
		},
		Outcomes:  outcomes,
		ReportMap: reportMapFromRuntime(runtime),
	}
}

func factorCodes(measure definition.MeasureSpec) []string {
	if len(measure.Factors) == 0 {
		return nil
	}
	out := make([]string, 0, len(measure.Factors))
	for _, item := range measure.Factors {
		if item.Code == "" {
			continue
		}
		out = append(out, item.Code)
	}
	return out
}

func conclusionOutcomes(payload *Payload) []conclusion.Outcome {
	if payload == nil || len(payload.Outcomes) == 0 {
		return nil
	}
	out := make([]conclusion.Outcome, 0, len(payload.Outcomes))
	for _, item := range payload.Outcomes {
		out = append(out, conclusion.Outcome{
			Code:        item.Code,
			Title:       item.Name,
			Summary:     item.Summary,
			Description: item.OneLiner,
		})
	}
	return out
}

func reportMapFromRuntime(runtime *RuntimeSpec) definition.ReportMap {
	if runtime == nil || runtime.Report.Kind == "" {
		return definition.ReportMap{}
	}
	section := definition.ReportSection{
		Code:       string(runtime.Report.Kind),
		Title:      runtime.Report.CategoryLabel,
		SourceRefs: []string{string(runtime.Report.ResolvedAdapterKey(runtime.OutcomeMapping, runtime.Decision.Kind))},
	}
	if section.Title == "" {
		section.Title = string(runtime.Report.Kind)
	}
	if section.SourceRefs[0] == "" {
		section.SourceRefs = nil
	}
	return definition.ReportMap{Sections: []definition.ReportSection{section}}
}
