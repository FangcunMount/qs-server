// Package snapshot is a compatibility seam; canonical home is taskperformance/snapshot.
package snapshot

import taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/taskperformance/snapshot"

type (
	Snapshot              = taskperfsnapshot.Snapshot
	FactorSnapshot        = taskperfsnapshot.FactorSnapshot
	InterpretRuleSnapshot = taskperfsnapshot.InterpretRuleSnapshot
)

var (
	ParseDefinitionPayload = taskperfsnapshot.ParseDefinitionPayload
	ParsePublishedPayload  = taskperfsnapshot.ParsePublishedPayload
)
