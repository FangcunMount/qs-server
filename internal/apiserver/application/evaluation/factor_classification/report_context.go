package factor_classification

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_classification/legacy"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func resolveReportBuildContext(
	runner algorithmRunner,
	outcome evaloutcome.Outcome,
) (modeltypology.ReportSpec, modeltypology.OutcomeMappingSpec, modelcatalog.DecisionKind) {
	var spec modeltypology.ReportSpec
	mapping := modeltypology.OutcomeMappingSpec{}
	decisionKind := modelcatalog.DecisionKind("")
	if outcome.Input != nil {
		if payload, ok := port.TypologyPayload(outcome.Input); ok && payload != nil {
			if runtimeSpec, err := payload.ToRuntimeSpec(); err == nil {
				return runtimeSpec.Report, runtimeSpec.OutcomeMapping, runtimeSpec.Decision.Kind
			}
		}
	}
	algorithm := modelcatalog.Algorithm("")
	if runner.adapter != nil {
		algorithm = runner.adapter.Algorithm()
	}
	if algorithm == "" || algorithm == modelcatalog.AlgorithmPersonalityTypology {
		algorithm = legacyAlgorithmFromOutcome(outcome)
	}
	if algorithm != "" {
		spec = legacy.ReportSpecForAlgorithm(algorithm)
		mapping = legacy.OutcomeMappingForAlgorithm(algorithm)
	}
	return spec, mapping, decisionKind
}

func legacyAlgorithmFromOutcome(outcome evaloutcome.Outcome) modelcatalog.Algorithm {
	if outcome.Assessment != nil {
		if ref := outcome.Assessment.EvaluationModelRef(); ref != nil {
			if algorithm := ref.Algorithm(); algorithm != "" && algorithm != modelcatalog.AlgorithmPersonalityTypology {
				return algorithm
			}
		}
	}
	if outcome.Execution != nil {
		if algorithm := outcome.Execution.ModelRef.Algorithm(); algorithm != "" && algorithm != modelcatalog.AlgorithmPersonalityTypology {
			return algorithm
		}
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		if algorithm := modelcatalog.Algorithm(outcome.Input.Model.ModelRef().Algorithm); algorithm != "" && algorithm != modelcatalog.AlgorithmPersonalityTypology {
			return algorithm
		}
	}
	return ""
}
