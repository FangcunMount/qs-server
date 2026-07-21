package typology

import (
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// ReportRoutingSource classifies how typology report TemplateID/AdapterKey were obtained.
type ReportRoutingSource string

const (
	// ReportRoutingHistoricalLegacy means runtime was absent; legacy derivation is allowed.
	ReportRoutingHistoricalLegacy ReportRoutingSource = "historical_legacy"
	// ReportRoutingExplicitRuntime means an author-declared runtime section was present and valid.
	ReportRoutingExplicitRuntime ReportRoutingSource = "explicit_runtime"
)

// ErrRuntimeSpecInvalid marks a declared runtime that fails ToRuntimeSpec validation.
// Callers must fail closed; do not fall back to historical derivation.
var ErrRuntimeSpecInvalid = errors.New("runtime_spec_invalid")

// TypologyReportRouting is the pure report-template routing result shared by
// production Outcome adapters and Preview adapters.
type TypologyReportRouting struct {
	Source       ReportRoutingSource
	TemplateID   string
	AdapterKey   ReportAdapterKey
	DecisionKind binding.DecisionKind
	Spec         *RuntimeSpec
}

// ResolveTypologyReportRouting resolves report TemplateID/AdapterKey from a
// typology payload without silently treating invalid explicit runtime as legacy.
//
//	!HasExplicitRuntime → historical_legacy (derive allowed; derive failure leaves fields empty)
//	HasExplicitRuntime && ToRuntimeSpec err → runtime_spec_invalid (fail closed)
//	HasExplicitRuntime && success → explicit_runtime
func ResolveTypologyReportRouting(payload *Payload) (TypologyReportRouting, error) {
	if payload == nil || !payload.HasExplicitRuntime() {
		routing := TypologyReportRouting{Source: ReportRoutingHistoricalLegacy}
		if payload == nil {
			return routing, nil
		}
		spec, err := payload.ToRuntimeSpec()
		if err != nil {
			return routing, nil
		}
		return routingFromSpec(ReportRoutingHistoricalLegacy, spec), nil
	}

	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		return TypologyReportRouting{}, fmt.Errorf("%w: %v", ErrRuntimeSpecInvalid, err)
	}
	return routingFromSpec(ReportRoutingExplicitRuntime, spec), nil
}

func routingFromSpec(source ReportRoutingSource, spec *RuntimeSpec) TypologyReportRouting {
	if spec == nil {
		return TypologyReportRouting{Source: source}
	}
	return TypologyReportRouting{
		Source:       source,
		TemplateID:   spec.Report.TemplateID,
		AdapterKey:   spec.Report.ResolvedAdapterKey(spec.OutcomeMapping, spec.Decision.Kind),
		DecisionKind: spec.Decision.Kind,
		Spec:         spec,
	}
}

// IsRegisteredReportTemplateID reports whether templateID is known to the
// interpretation template registry. Empty IDs are not registered.
func IsRegisteredReportTemplateID(templateID string) bool {
	switch templateID {
	case "mbti", "sbti", "bigfive":
		return true
	default:
		return false
	}
}
