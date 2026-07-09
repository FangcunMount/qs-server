package assessmentstore

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
)

// ScaleChangedEvent builds a scale lifecycle event from an AssessmentModel snapshot envelope.
func ScaleChangedEvent(model *domain.AssessmentModel, action scaledefinition.ChangeAction) (scaledefinition.ScaleChangedEvent, bool) {
	if model == nil {
		return scaledefinition.ScaleChangedEvent{}, false
	}
	snapshot, err := legacyadapter.ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		return scaledefinition.ScaleChangedEvent{}, false
	}
	return scaledefinition.NewScaleChangedEvent(
		0,
		model.Code,
		snapshot.ScaleVersion,
		model.Title,
		action,
		time.Now().UTC(),
	), true
}
