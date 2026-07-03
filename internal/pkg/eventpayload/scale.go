package eventpayload

import "time"

// ScaleChangeAction is a scale lifecycle action.
type ScaleChangeAction string

const (
	ScaleChangeActionPublished   ScaleChangeAction = "published"
	ScaleChangeActionUnpublished ScaleChangeAction = "unpublished"
	ScaleChangeActionUpdated     ScaleChangeAction = "updated"
	ScaleChangeActionArchived    ScaleChangeAction = "archived"
)

// ScaleChangedData is the scale lifecycle changed event body.
type ScaleChangedData struct {
	ScaleID   uint64            `json:"scale_id"`
	Code      string            `json:"code"`
	Version   string            `json:"version"`
	Name      string            `json:"name"`
	Action    ScaleChangeAction `json:"action"`
	ChangedAt time.Time         `json:"changed_at"`
}
