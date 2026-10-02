//go:build integration && m4_07_new_chain

package answersheet

import (
	"fmt"
	"time"
)

// SetM407PendingEventIdentity fixes generated metadata only in the isolated comparison build.
func (a *AnswerSheet) SetM407PendingEventIdentity(id string, occurredAt time.Time) error {
	if id == "" || occurredAt.IsZero() || len(a.events) != 1 {
		return fmt.Errorf("M4-07 proof requires one pending event, an ID and occurrence time")
	}
	submitted, ok := a.events[0].(AnswerSheetSubmittedEvent)
	if !ok {
		return fmt.Errorf("M4-07 proof requires an answer sheet submitted event")
	}
	submitted.BaseEvent.ID = id
	submitted.BaseEvent.OccurredAtValue = occurredAt
	a.events[0] = submitted
	return nil
}
