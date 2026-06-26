package typology

import (
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func resolveReportBuildContext(
	runner algorithmRunner,
	outcome evaluationresult.Outcome,
) (modeltypology.ReportSpec, modeltypology.OutcomeMappingSpec, assessmentmodel.DecisionKind) {
	var spec modeltypology.ReportSpec
	mapping := modeltypology.OutcomeMappingSpec{}
	decisionKind := assessmentmodel.DecisionKind("")
	if outcome.Input != nil {
		if payload, ok := port.TypologyPayload(outcome.Input); ok && payload != nil {
			if runtimeSpec, err := payload.ToRuntimeSpec(); err == nil {
				return runtimeSpec.Report, runtimeSpec.OutcomeMapping, runtimeSpec.Decision.Kind
			}
		}
	}
	if runner.adapter != nil {
		algorithm := runner.adapter.Algorithm()
		spec = legacy.ReportSpecForAlgorithm(algorithm)
		mapping = legacy.OutcomeMappingForAlgorithm(algorithm)
	}
	return spec, mapping, decisionKind
}
