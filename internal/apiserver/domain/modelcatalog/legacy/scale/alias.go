package scale

import (
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

// MedicalScale is the legacy medical-scale authoring aggregate.
type MedicalScale = scaledefinition.MedicalScale

// Snapshot is the legacy published scale execution payload.
type Snapshot = scalesnapshot.ScaleSnapshot
