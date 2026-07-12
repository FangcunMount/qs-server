package runtime

import (
	mechanismnorming "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	mechanismscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	mechanismtask "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	mechanismtypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	portruleengine "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// WiringDeps groups dependencies used to attach native descriptor pipelines.
type WiringDeps struct {
	ScaleScorer portruleengine.ScaleFactorScorer
}

// MaterializeFactorScoringPipelineComponents builds native factor_scoring pipeline triple from wiring deps.
func MaterializeFactorScoringPipelineComponents(deps WiringDeps) mechanismscoring.PipelineComponents {
	return mechanismscoring.NewPipelineComponents(deps.ScaleScorer)
}

// MaterializeFactorNormPipelineComponents builds native factor_norm pipeline triple from wiring deps.
func MaterializeFactorNormPipelineComponents(deps WiringDeps) mechanismnorming.PipelineComponents {
	return mechanismnorming.NewPipelineComponents(deps.ScaleScorer)
}

// MaterializeTaskPerformancePipelineComponents builds native task_performance pipeline triple from wiring deps.
func MaterializeTaskPerformancePipelineComponents(deps WiringDeps) mechanismtask.PipelineComponents {
	return mechanismtask.NewPipelineComponents(deps.ScaleScorer)
}

// MaterializeFactorClassificationPipelineComponents builds native factor_classification pipeline triple from wiring deps.
func MaterializeFactorClassificationPipelineComponents(deps WiringDeps) mechanismtypology.PipelineComponents {
	return mechanismtypology.NewPipelineComponents()
}
