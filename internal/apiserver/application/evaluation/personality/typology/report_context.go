package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology/legacy"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
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
	algorithm := assessmentmodel.Algorithm("")
	if runner.adapter != nil {
		algorithm = runner.adapter.Algorithm()
	}
	if algorithm == "" || algorithm == assessmentmodel.AlgorithmPersonalityTypology {
		algorithm = legacyAlgorithmFromOutcome(outcome)
	}
	if algorithm != "" {
		spec = legacy.ReportSpecForAlgorithm(algorithm)
		mapping = legacy.OutcomeMappingForAlgorithm(algorithm)
	}
	return spec, mapping, decisionKind
}

func legacyAlgorithmFromOutcome(outcome evaluationresult.Outcome) assessmentmodel.Algorithm {
	if outcome.Assessment != nil {
		if ref := outcome.Assessment.EvaluationModelRef(); ref != nil {
			if algorithm := ref.Algorithm(); algorithm != "" && algorithm != assessmentmodel.AlgorithmPersonalityTypology {
				return algorithm
			}
		}
	}
	if outcome.Execution != nil {
		if algorithm := outcome.Execution.ModelRef.Algorithm(); algorithm != "" && algorithm != assessmentmodel.AlgorithmPersonalityTypology {
			return algorithm
		}
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		if algorithm := assessmentmodel.Algorithm(outcome.Input.Model.ModelRef().Algorithm); algorithm != "" && algorithm != assessmentmodel.AlgorithmPersonalityTypology {
			return algorithm
		}
	}
	return ""
}
