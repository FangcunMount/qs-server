package ruleset

import (
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/assessmentmodel"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
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

func RuleSetRefFromSnapshot(snapshot *domain.RuleSetSnapshot) port.RuleSetRef {
	return aminfra.RefFromSnapshot(snapshot)
}
