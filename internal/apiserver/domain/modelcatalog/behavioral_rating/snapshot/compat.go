// Package snapshot is a compatibility seam; canonical home is norming/snapshot.
package snapshot

import normingsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"

type (
	Snapshot              = normingsnapshot.Snapshot
	NormingProfile        = normingsnapshot.NormingProfile
	FactorSnapshot        = normingsnapshot.FactorSnapshot
	InterpretRuleSnapshot = normingsnapshot.InterpretRuleSnapshot
)

var (
	ParseDefinitionPayload = normingsnapshot.ParseDefinitionPayload
	ParsePublishedPayload  = normingsnapshot.ParsePublishedPayload
)
