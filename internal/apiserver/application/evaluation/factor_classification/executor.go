package factor_classification

import (
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// Executor runs factor-classification evaluations via the configured typology runtime.
type Executor = typologyEvaluation.Executor

// ModuleRegistry resolves typology modules for factor-classification execution.
type ModuleRegistry = typologyEvaluation.ModuleRegistry

// MaterializeEvaluator builds or reuses the configured typology executor for a descriptor.
func MaterializeEvaluator(
	desc evaldomain.ModelDescriptor,
	registry ModuleRegistry,
	shared **Executor,
) (evaluationexecute.Evaluator, error) {
	return typologyEvaluation.MaterializeTypologyEvaluator(desc, registry, shared)
}

// MaterializeReportBuilder builds or reuses the configured typology report builder for a descriptor.
func MaterializeReportBuilder(
	desc evaldomain.ModelDescriptor,
	registry ModuleRegistry,
	shared *typologyEvaluation.ReportBuilder,
) (interpretationreporting.ReportBuilder, error) {
	if shared == nil {
		return nil, fmt.Errorf("shared typology report builder holder is required")
	}
	return typologyEvaluation.MaterializeTypologyReportBuilder(desc, registry, shared)
}
