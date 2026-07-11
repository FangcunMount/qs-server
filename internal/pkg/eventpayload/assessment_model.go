package eventpayload

import "time"

// AssessmentModelChangeAction identifies a committed assessment-model
// lifecycle transition.
type AssessmentModelChangeAction string

const (
	AssessmentModelChangeActionPublished   AssessmentModelChangeAction = "published"
	AssessmentModelChangeActionUnpublished AssessmentModelChangeAction = "unpublished"
	AssessmentModelChangeActionArchived    AssessmentModelChangeAction = "archived"
)

// AssessmentModelChangedData is the model-agnostic lifecycle event body.
// Kind, code and version are the canonical immutable model identity; no legacy
// scale collection identifier is carried across the event boundary.
type AssessmentModelChangedData struct {
	Kind      string                      `json:"kind"`
	Code      string                      `json:"code"`
	Version   string                      `json:"version"`
	Title     string                      `json:"title"`
	Action    AssessmentModelChangeAction `json:"action"`
	ChangedAt time.Time                   `json:"changed_at"`
}
