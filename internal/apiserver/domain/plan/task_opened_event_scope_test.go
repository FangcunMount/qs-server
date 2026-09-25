package plan

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestTaskOpenedEventCarriesOrganizationScope(t *testing.T) {
	openedAt := time.Date(2026, 9, 25, 10, 0, 0, 0, taskBusinessLocation)
	task := NewAssessmentTask(NewAssessmentPlanID(), 1, 501, testee.NewID(21), "scale", openedAt)
	if err := NewTaskLifecycle().OpenAt(t.Context(), task, "token", "https://example.test/task", openedAt); err != nil {
		t.Fatal(err)
	}
	if len(task.Events()) != 1 {
		t.Fatalf("events=%d want=1", len(task.Events()))
	}
	opened, ok := task.Events()[0].(TaskOpenedEvent)
	if !ok {
		t.Fatalf("event type=%T want TaskOpenedEvent", task.Events()[0])
	}
	if opened.Data.OrgID != 501 {
		t.Fatalf("org_id=%d want=501", opened.Data.OrgID)
	}
	reminder := NewTaskOpenedReminderRequestedEvent(opened, task.GetScheduleRevision())
	if reminder.EventID() != opened.EventID() || reminder.OccurredAt() != opened.OccurredAt() || reminder.EventType() != EventTypeTaskOpenedReminderRequested || opened.EventType() != EventTypeTaskOpened {
		t.Fatalf("reminder must preserve the original opening identity without changing the legacy event: original=%+v reminder=%+v", opened.BaseEvent, reminder.BaseEvent)
	}
	if reminder.Data.OrgID != 501 || reminder.Data.ScheduleRevision != 1 {
		t.Fatalf("reminder must retain scope and opening revision: %+v", reminder.Data)
	}
	encoded, err := json.Marshal(opened)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Data struct {
			OrgID int64 `json:"org_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Data.OrgID != 501 {
		t.Fatalf("serialized org_id=%d want=501", wire.Data.OrgID)
	}
}
