// Package snapshot is a compatibility facade for the scale runtime payload.
//
// Deprecated: use internal/apiserver/port/modelcatalog/payload/scale.
package snapshot

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

type (
	ScaleSnapshot         = scalepayload.ScaleSnapshot
	FactorSnapshot        = scalepayload.FactorSnapshot
	ScoringParamsSnapshot = scalepayload.ScoringParamsSnapshot
	InterpretRuleSnapshot = scalepayload.InterpretRuleSnapshot
	ExecutionEnvelope     = scalepayload.ExecutionEnvelope
)

func ParsePublishedPayload(payload []byte) (*ScaleSnapshot, error) {
	return scalepayload.ParsePublishedPayload(payload)
}

func InterpretRuleFromScoreRange(r factor.ScoreRangeRule) InterpretRuleSnapshot {
	return scalepayload.InterpretRuleFromScoreRange(r)
}

func ScoreRangeFromInterpretRule(rule InterpretRuleSnapshot) factor.ScoreRangeRule {
	return scalepayload.ScoreRangeFromInterpretRule(rule)
}

func FactorFromCanonical(f factor.FactorSnapshot) FactorSnapshot {
	return scalepayload.FactorFromCanonical(f)
}

func FactorFromLegacyFactor(f factor.LegacyFactor) FactorSnapshot {
	return scalepayload.FactorFromLegacyFactor(f)
}

func FactorSnapshotFromCanonical(f factor.FactorSnapshot) FactorSnapshot {
	return scalepayload.FactorSnapshotFromCanonical(f)
}

func FactorsFromCanonical(factors []factor.FactorSnapshot) []FactorSnapshot {
	return scalepayload.FactorsFromCanonical(factors)
}

func FactorsFromLegacy(factors []factor.LegacyFactor) []FactorSnapshot {
	return scalepayload.FactorsFromLegacy(factors)
}

func BuildFromLegacyFactors(env ExecutionEnvelope, factors []factor.LegacyFactor) *ScaleSnapshot {
	return scalepayload.BuildFromLegacyFactors(env, factors)
}

func BuildFromCanonicalFactors(env ExecutionEnvelope, factors []factor.FactorSnapshot) *ScaleSnapshot {
	return scalepayload.BuildFromCanonicalFactors(env, factors)
}

func DefinitionFromScaleSnapshot(snapshot *ScaleSnapshot) *definition.Definition {
	return scalepayload.DefinitionFromScaleSnapshot(snapshot)
}

func ScaleSnapshotFromDefinition(env ExecutionEnvelope, def *definition.Definition) *ScaleSnapshot {
	return scalepayload.ScaleSnapshotFromDefinition(env, def)
}
