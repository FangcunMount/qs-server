// Package snapshot is a compatibility seam; canonical home is scoring/snapshot.
package snapshot

import scoringsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"

type (
	ScaleSnapshot         = scoringsnapshot.ScaleSnapshot
	FactorSnapshot        = scoringsnapshot.FactorSnapshot
	ScoringParamsSnapshot = scoringsnapshot.ScoringParamsSnapshot
	InterpretRuleSnapshot = scoringsnapshot.InterpretRuleSnapshot
	ExecutionEnvelope     = scoringsnapshot.ExecutionEnvelope
)

var (
	ParsePublishedPayload       = scoringsnapshot.ParsePublishedPayload
	InterpretRuleFromScoreRange = scoringsnapshot.InterpretRuleFromScoreRange
	ScoreRangeFromInterpretRule = scoringsnapshot.ScoreRangeFromInterpretRule
	FactorFromCanonical         = scoringsnapshot.FactorFromCanonical
	FactorSnapshotFromCanonical = scoringsnapshot.FactorSnapshotFromCanonical
	FactorsFromCanonical        = scoringsnapshot.FactorsFromCanonical
	BuildFromModelFactors       = scoringsnapshot.BuildFromModelFactors
	BuildFromCanonicalFactors   = scoringsnapshot.BuildFromCanonicalFactors
)
