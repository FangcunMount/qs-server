package assessmentstore

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

// SyncScaleMetadataInModel projects editable metadata onto the scale definition envelope.
func SyncScaleMetadataInModel(model *domain.AssessmentModel) error {
	return legacyadapter.SyncScaleMetadataInModel(model)
}

// SyncSnapshotStatus updates the scale snapshot status inside the definition envelope.
func SyncSnapshotStatus(model *domain.AssessmentModel, status string) error {
	if model == nil {
		return fmt.Errorf("assessment model is nil")
	}
	snapshot, err := legacyadapter.ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		return err
	}
	snapshot.Status = status
	return applyScaleSnapshotEnvelope(model, snapshot)
}

// MutateScaleSnapshot applies an in-place mutation to the scale definition envelope.
func MutateScaleSnapshot(model *domain.AssessmentModel, mutate func(*scalesnapshot.ScaleSnapshot) error) error {
	if model == nil {
		return fmt.Errorf("assessment model is nil")
	}
	snapshot, err := legacyadapter.ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		return err
	}
	if err := mutate(snapshot); err != nil {
		return err
	}
	return UpdateScaleDefinition(model, snapshot, time.Now().UTC())
}

// UpdateScaleDefinition persists a mutated snapshot onto the model with DefinitionV2 materialization.
func UpdateScaleDefinition(model *domain.AssessmentModel, snapshot *scalesnapshot.ScaleSnapshot, now time.Time) error {
	if model == nil || snapshot == nil {
		return fmt.Errorf("assessment model or scale snapshot is nil")
	}
	payload, err := legacyadapter.DefinitionPayloadFromScaleSnapshot(snapshot)
	if err != nil {
		return err
	}
	return model.UpdateDefinitionWithV2(payload, scalesnapshot.DefinitionFromScaleSnapshot(snapshot), now)
}

func applyScaleSnapshotEnvelope(model *domain.AssessmentModel, snapshot *scalesnapshot.ScaleSnapshot) error {
	payload, err := legacyadapter.DefinitionPayloadFromScaleSnapshot(snapshot)
	if err != nil {
		return err
	}
	model.Definition = payload
	model.DefinitionV2 = scalesnapshot.DefinitionFromScaleSnapshot(snapshot)
	return nil
}

// ForkDraftFromPublished forks a published scale head into a draft working version.
func ForkDraftFromPublished(model *domain.AssessmentModel, now time.Time) error {
	return legacyadapter.ForkAssessmentModelDraftFromPublished(model, now)
}

// DefaultScaleVersion returns the default scale semantic version.
func DefaultScaleVersion() string {
	return scaledefinition.DefaultScaleVersion
}
