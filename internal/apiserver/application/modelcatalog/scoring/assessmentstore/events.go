package assessmentstore

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ScaleChangedEvent builds a scale lifecycle event from an AssessmentModel snapshot envelope.
func ScaleChangedEvent(model *domain.AssessmentModel, action eventpayload.ScaleChangeAction) (event.Event[eventpayload.ScaleChangedData], bool) {
	if model == nil {
		return event.Event[eventpayload.ScaleChangedData]{}, false
	}
	snapshot, err := legacyadapter.ScaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		return event.Event[eventpayload.ScaleChangedData]{}, false
	}
	return event.New(eventcatalog.ScaleChanged, "MedicalScale", "0", eventpayload.ScaleChangedData{
		ScaleID:   0,
		Code:      model.Code,
		Version:   snapshot.ScaleVersion,
		Name:      model.Title,
		Action:    action,
		ChangedAt: time.Now().UTC(),
	}), true
}
