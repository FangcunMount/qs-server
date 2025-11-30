package scale

// ScalePublishedEventDTO 量表发布事件 DTO
type ScalePublishedEventDTO struct {
	EventID     string `json:"event_id"`
	EventType   string `json:"event_type"`
	ScaleID     uint64 `json:"scale_id"`
	Code        string `json:"code"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
}

// ScaleUnpublishedEventDTO 量表下架事件 DTO
type ScaleUnpublishedEventDTO struct {
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	ScaleID       uint64 `json:"scale_id"`
	Code          string `json:"code"`
	Version       string `json:"version"`
	UnpublishedAt string `json:"unpublished_at"`
}

// ScaleArchivedEventDTO 量表归档事件 DTO
type ScaleArchivedEventDTO struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	ScaleID    uint64 `json:"scale_id"`
	Code       string `json:"code"`
	Version    string `json:"version"`
	ArchivedAt string `json:"archived_at"`
}

// ScaleUpdatedEventDTO 量表更新事件 DTO
type ScaleUpdatedEventDTO struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	ScaleID   uint64 `json:"scale_id"`
	Code      string `json:"code"`
	Version   string `json:"version"`
	UpdatedAt string `json:"updated_at"`
}
