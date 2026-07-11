package definition

import (
	"encoding/json"
	"fmt"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// InitializeScaleDefinition creates the canonical empty scale definition and
// refreshes its compatibility payload projection.
func InitializeScaleDefinition(model *domain.AssessmentModel, now time.Time) error {
	if model == nil || model.Kind != domain.KindScale {
		return nil
	}
	model.DefinitionV2 = &domain.Definition{}
	return RefreshScaleDraftProjectionAt(model, now)
}

// RefreshScaleDraftProjection updates the draft wire projection from the
// canonical DefinitionV2. The payload is not an authoring source of truth.
func RefreshScaleDraftProjection(model *domain.AssessmentModel) error {
	return RefreshScaleDraftProjectionAt(model, time.Now().UTC())
}

// RefreshScaleDraftProjectionAt is the clock-injectable variant used by
// lifecycle application services.
func RefreshScaleDraftProjectionAt(model *domain.AssessmentModel, now time.Time) error {
	if model == nil || model.Kind != domain.KindScale || model.DefinitionV2 == nil {
		return nil
	}

	snapshot := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{
		Code:                 model.Code,
		ScaleVersion:         "1.0.0",
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(model.Status),
	}, model.DefinitionV2)
	if snapshot == nil {
		return fmt.Errorf("scale definition projection is empty")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	model.Definition = domain.DefinitionPayload{
		Format: domain.PayloadFormatAssessmentScaleV1,
		Data:   payload,
	}
	model.UpdatedAt = now
	return nil
}
