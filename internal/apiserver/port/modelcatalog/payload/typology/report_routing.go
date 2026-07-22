package typology

import (
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// ReportRoutingSource classifies how typology report TemplateID/AdapterKey were obtained.
type ReportRoutingSource string

const (
	ReportRoutingDefinitionV2 ReportRoutingSource = "definition_v2"
)

// ErrRuntimeSpecInvalid marks a declared runtime that fails ToRuntimeSpec validation.
// Callers must fail closed; do not fall back to historical derivation.
var ErrRuntimeSpecInvalid = errors.New("runtime_spec_invalid")

// TypologyReportRouting is the pure report-template routing result shared by
// production Outcome adapters and Preview adapters.
type TypologyReportRouting struct {
	Source          ReportRoutingSource
	TemplateID      string
	TemplateVersion string
	AdapterKey      ReportAdapterKey
	DecisionKind    binding.DecisionKind
	Spec            *RuntimeSpec
}

// ResolveTypologyReportRouting resolves report routing from a DefinitionV2-materialized DTO.
func ResolveTypologyReportRouting(payload *Payload) (TypologyReportRouting, error) {
	if payload == nil || !payload.HasExplicitRuntime() {
		return TypologyReportRouting{}, fmt.Errorf("%w: definition_v2 runtime is required", ErrRuntimeSpecInvalid)
	}

	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		return TypologyReportRouting{}, fmt.Errorf("%w: %v", ErrRuntimeSpecInvalid, err)
	}
	return routingFromSpec(ReportRoutingDefinitionV2, spec), nil
}

func routingFromSpec(source ReportRoutingSource, spec *RuntimeSpec) TypologyReportRouting {
	if spec == nil {
		return TypologyReportRouting{Source: source}
	}
	return TypologyReportRouting{
		Source:          source,
		TemplateID:      spec.Report.TemplateID,
		TemplateVersion: spec.Report.TemplateVersion,
		AdapterKey:      spec.Report.ResolvedAdapterKey(spec.OutcomeMapping, spec.Decision.Kind),
		DecisionKind:    spec.Decision.Kind,
		Spec:            spec,
	}
}

// IsRegisteredReportTemplateID reports whether templateID is known to the
// interpretation template registry. Empty IDs are not registered.
func IsRegisteredReportTemplateID(templateID string) bool {
	switch templateID {
	case "mbti", "sbti", "bigfive", "enneagram":
		return true
	default:
		return false
	}
}
