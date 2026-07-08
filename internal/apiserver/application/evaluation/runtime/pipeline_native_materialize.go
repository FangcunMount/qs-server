package runtime

import (
	mechanismscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
)

// MaterializeFactorScoringPipelineComponents builds native factor_scoring pipeline triple from wiring deps.
func MaterializeFactorScoringPipelineComponents(deps WiringDeps) mechanismscoring.PipelineComponents {
	return mechanismscoring.NewPipelineComponents(deps.ScaleScorer)
}
