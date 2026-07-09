package assessmentstore

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

// AddFactorSnapshot adds a validated factor to the scale definition envelope.
func AddFactorSnapshot(model *domain.AssessmentModel, factor *scaledefinition.Factor) error {
	return MutateScaleSnapshot(model, func(snapshot *scalesnapshot.ScaleSnapshot) error {
		return addFactorSnapshot(snapshot, factor)
	})
}

// UpdateFactorSnapshot updates an existing factor in the scale definition envelope.
func UpdateFactorSnapshot(model *domain.AssessmentModel, factor *scaledefinition.Factor) error {
	return MutateScaleSnapshot(model, func(snapshot *scalesnapshot.ScaleSnapshot) error {
		return updateFactorSnapshot(snapshot, factor)
	})
}

// RemoveFactorSnapshot removes a factor from the scale definition envelope.
func RemoveFactorSnapshot(model *domain.AssessmentModel, factorCode string) error {
	return MutateScaleSnapshot(model, func(snapshot *scalesnapshot.ScaleSnapshot) error {
		return removeFactorSnapshot(snapshot, factorCode)
	})
}

// ReplaceFactorSnapshots replaces all factors in the scale definition envelope.
func ReplaceFactorSnapshots(model *domain.AssessmentModel, factors []*scaledefinition.Factor) error {
	return MutateScaleSnapshot(model, func(snapshot *scalesnapshot.ScaleSnapshot) error {
		return replaceFactorSnapshots(snapshot, factors)
	})
}

// UpdateFactorInterpretRulesSnapshot replaces interpret rules for one factor.
func UpdateFactorInterpretRulesSnapshot(model *domain.AssessmentModel, factorCode string, rules []scaledefinition.InterpretationRule) error {
	return MutateScaleSnapshot(model, func(snapshot *scalesnapshot.ScaleSnapshot) error {
		return updateFactorInterpretRulesSnapshot(snapshot, factorCode, rules)
	})
}
