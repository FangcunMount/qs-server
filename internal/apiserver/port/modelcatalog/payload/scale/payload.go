// Package scale owns the runtime/published JSON DTO for scale-like execution.
package scale

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	oldscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

type (
	ScaleSnapshot         = oldscale.ScaleSnapshot
	FactorSnapshot        = oldscale.FactorSnapshot
	ScoringParamsSnapshot = oldscale.ScoringParamsSnapshot
	InterpretRuleSnapshot = oldscale.InterpretRuleSnapshot
	ExecutionEnvelope     = oldscale.ExecutionEnvelope
)

func ParsePublishedPayload(payload []byte) (*ScaleSnapshot, error) {
	return oldscale.ParsePublishedPayload(payload)
}

func InterpretRuleFromScoreRange(r factor.ScoreRangeRule) InterpretRuleSnapshot {
	return oldscale.InterpretRuleFromScoreRange(r)
}

func ScoreRangeFromInterpretRule(rule InterpretRuleSnapshot) factor.ScoreRangeRule {
	return oldscale.ScoreRangeFromInterpretRule(rule)
}

func FactorFromCanonical(f factor.FactorSnapshot) FactorSnapshot {
	return oldscale.FactorFromCanonical(f)
}

func FactorFromLegacyFactor(f factor.LegacyFactor) FactorSnapshot {
	return oldscale.FactorFromLegacyFactor(f)
}

func FactorSnapshotFromCanonical(f factor.FactorSnapshot) FactorSnapshot {
	return oldscale.FactorSnapshotFromCanonical(f)
}

func FactorsFromCanonical(factors []factor.FactorSnapshot) []FactorSnapshot {
	return oldscale.FactorsFromCanonical(factors)
}

func FactorsFromLegacy(factors []factor.LegacyFactor) []FactorSnapshot {
	return oldscale.FactorsFromLegacy(factors)
}

func BuildFromLegacyFactors(env ExecutionEnvelope, factors []factor.LegacyFactor) *ScaleSnapshot {
	return oldscale.BuildFromLegacyFactors(env, factors)
}

func BuildFromCanonicalFactors(env ExecutionEnvelope, factors []factor.FactorSnapshot) *ScaleSnapshot {
	return oldscale.BuildFromCanonicalFactors(env, factors)
}

func DefinitionFromScaleSnapshot(snapshot *ScaleSnapshot) *definition.Definition {
	return oldscale.DefinitionFromScaleSnapshot(snapshot)
}

func ScaleSnapshotFromDefinition(env ExecutionEnvelope, def *definition.Definition) *ScaleSnapshot {
	return oldscale.ScaleSnapshotFromDefinition(env, def)
}
