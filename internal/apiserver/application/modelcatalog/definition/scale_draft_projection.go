package definition

import (
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// InitializeScaleDefinition creates the canonical empty scale definition.
func InitializeScaleDefinition(model *domain.AssessmentModel, now time.Time) error {
	if model == nil || model.Kind != domain.KindScale {
		return nil
	}
	model.DefinitionV2 = &domain.Definition{}
	return RefreshScaleDraftProjectionAt(model, now)
}

// RefreshScaleDraftProjection validates the draft DefinitionV2 runtime materialization.
func RefreshScaleDraftProjection(model *domain.AssessmentModel) error {
	return RefreshScaleDraftProjectionAt(model, time.Now().UTC())
}

// RefreshScaleDraftProjectionAt 刷新草稿线投影
func RefreshScaleDraftProjectionAt(model *domain.AssessmentModel, now time.Time) error {
	if model == nil || model.Kind != domain.KindScale || model.DefinitionV2 == nil {
		return nil
	}

	if _, err := (RuntimeMaterializer{}).MaterializeScale(model); err != nil {
		return err
	}
	model.UpdatedAt = now
	return nil
}
