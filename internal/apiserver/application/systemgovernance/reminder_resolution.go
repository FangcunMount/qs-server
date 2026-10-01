package systemgovernance

import "time"

// ReminderResolutionRequest records a human finding, never authorizes a send.
// Reviewed is distinct from platform Confirmed; unknown findings stay unknown.
type ReminderResolutionRequest struct {
	RequestID         string    `json:"request_id"`
	DeliveryID        uint64    `json:"delivery_id"`
	TaskID            string    `json:"task_id"`
	OpeningEventID    string    `json:"opening_event_id"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	Finding           string    `json:"finding"`
	EvidenceReference string    `json:"evidence_reference"`
	Reason            string    `json:"reason"`
	Confirm           bool      `json:"confirm"`
}
