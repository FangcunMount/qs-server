package ruleset

import (
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func SBTIRuleSetSnapshot(model *modeltypology.SBTILegacyModel) (*domain.RuleSetSnapshot, error) {
	return aminfra.LegacySnapshotFromSBTI(model)
}

func MBTIRuleSetSnapshot(model *modeltypology.MBTILegacyModel) (*domain.RuleSetSnapshot, error) {
	return aminfra.LegacySnapshotFromMBTI(model)
}

func ScaleRuleSetSnapshot(model *scalesnapshot.ScaleSnapshot) (*domain.RuleSetSnapshot, error) {
	return aminfra.LegacySnapshotFromScale(model)
}

func RuleSetRefFromSnapshot(snapshot *domain.RuleSetSnapshot) port.Ref {
	return aminfra.RefFromSnapshot(snapshot)
}
