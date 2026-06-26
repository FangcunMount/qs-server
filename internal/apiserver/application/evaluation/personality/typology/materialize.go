package typology

import (
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// MaterializeTypologyEvaluator builds or reuses the configured typology executor for a descriptor.
func MaterializeTypologyEvaluator(
	desc evaldomain.ModelDescriptor,
	registry ModuleRegistry,
	shared **Executor,
) (evaluationexecute.Evaluator, error) {
	if shared == nil {
		return nil, fmt.Errorf("shared typology executor holder is required")
	}
	if desc.Key != evaldomain.EvaluatorKeyPersonalityTypology {
		return nil, fmt.Errorf("unsupported typology descriptor key: %s", desc.Key)
	}
	if *shared == nil {
		executor, err := NewConfiguredTypologyExecutorWithRegistry(registry)
		if err != nil {
			return nil, err
		}
		*shared = executor
	}
	return *shared, nil
}

// MaterializeTypologyReportBuilder builds or reuses the configured typology report builder for a descriptor.
func MaterializeTypologyReportBuilder(
	desc evaldomain.ModelDescriptor,
	registry ModuleRegistry,
	shared *ReportBuilder,
) (evaluationresult.ReportBuilder, error) {
	if shared == nil {
		return nil, fmt.Errorf("shared typology report builder holder is required")
	}
	if desc.Key != evaldomain.EvaluatorKeyPersonalityTypology {
		return nil, fmt.Errorf("unsupported typology descriptor key: %s", desc.Key)
	}
	if shared.runner == nil {
		builder, err := NewConfiguredReportBuilderWithRegistry(registry)
		if err != nil {
			return nil, err
		}
		*shared = builder
	}
	return *shared, nil
}
