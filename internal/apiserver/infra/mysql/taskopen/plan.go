package taskopen

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// The full snapshot is private protocol data. Only the original reminder
// Branch.Wire reaches consumers; it does not expose the entry URL.
type frozenOpening struct {
	Version   int
	Task      repository.AssessmentTaskPO
	Reminder  []byte
	UpdatedAt time.Time
	UpdatedBy uint64
}

func openingKey(id uint64, revision uint32) string {
	return fmt.Sprintf("task-open/%d/%d", id, revision)
}

func freeze(task *domain.AssessmentTask, actor uint64) (eventport.Request, error) {
	if task == nil || task.GetID().IsZero() || task.GetStatus() != domain.TaskStatusOpened || len(task.Events()) != 1 {
		return eventport.Request{}, transaction.ErrInvalidPlan
	}
	opened, ok := task.Events()[0].(domain.TaskOpenedEvent)
	if !ok || opened.EventType() != domain.EventTypeTaskOpened || opened.AggregateType() != domain.AggregateTypeTask || opened.AggregateID() != task.GetID().String() ||
		opened.Data.OrgID != task.GetOrgID() || opened.Data.PlanID != task.GetPlanID().String() || opened.Data.TesteeID != task.GetTesteeID().String() || opened.Data.EntryURL != task.GetEntryURL() || task.GetOpenAt() == nil || !opened.Data.OpenAt.Equal(*task.GetOpenAt()) {
		return eventport.Request{}, transaction.ErrBinding
	}
	reminder := domain.NewTaskOpenedReminderRequestedEvent(opened, task.GetScheduleRevision())
	body, err := json.Marshal(reminder.Data)
	if err != nil {
		return eventport.Request{}, err
	}
	po := repository.NewTaskMapper().ToPO(task)
	// MySQL's task business clocks are DATETIME(3). Keep full wire precision
	// independently, while freezing the exact values used by the database CAS.
	po.OpenAt = mysqlClockPtr(po.OpenAt)
	po.ExpireAt = mysqlClockPtr(po.ExpireAt)
	f := frozenOpening{Version: 1, Task: *po, Reminder: body, UpdatedAt: *po.OpenAt, UpdatedBy: actor}
	if err := f.validate(); err != nil {
		return eventport.Request{}, err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return eventport.Request{}, err
	}
	hash, err := f.inputHash()
	if err != nil {
		return eventport.Request{}, err
	}
	return eventport.Request{Key: openingKey(po.ID.Uint64(), po.ScheduleRevision), InputHash: hash, Artifact: raw, Events: []event.DomainEvent{reminder}}, nil
}

func mysqlClockPtr(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	copy := at.UTC().Round(time.Microsecond).Round(time.Millisecond)
	return &copy
}

func (f frozenOpening) inputHash() ([32]byte, error) {
	// Competing openings may independently generate a new entry, event ID,
	// opening clock or actor. These are winner artifacts, not business input.
	stable := struct {
		ID, Plan, Enrollment, Testee uint64
		Org                          int64
		Seq                          int
		Revision                     uint32
		Scale                        string
		Planned                      time.Time
		Due, Defined                 *time.Time
	}{f.Task.ID.Uint64(), f.Task.PlanID, f.Task.EnrollmentID, f.Task.TesteeID, f.Task.OrgID, f.Task.Seq, f.Task.ScheduleRevision, f.Task.ScaleCode, f.Task.PlannedAt.UTC().Round(time.Microsecond).Round(time.Millisecond), mysqlClockPtr(f.Task.DueAt), mysqlClockPtr(f.Task.ScheduleDefinedAt)}
	raw, err := json.Marshal(stable)
	return sha256.Sum256(raw), err
}

func (f frozenOpening) validate() error {
	p := f.Task
	if f.Version != 1 || p.ID.IsZero() || p.OrgID <= 0 || p.PlanID == 0 || p.TesteeID == 0 || p.Seq < 1 || p.ScaleCode == "" || p.ScheduleRevision == 0 || p.Status != domain.TaskStatusOpened.String() ||
		p.OpenAt == nil || p.OpenAt.IsZero() || p.EntryURL == "" || len(p.EntryURL) > 500 || p.CompletedAt != nil || p.ExpiredAt != nil || p.CanceledAt != nil || p.AssessmentID != nil || p.PlannedAt.IsZero() || !f.UpdatedAt.Equal(*p.OpenAt) {
		return transaction.ErrInvalidPlan
	}
	var body eventpayload.TaskOpenedReminderRequestedData
	if json.Unmarshal(f.Reminder, &body) != nil || body.TaskID != p.ID.String() || body.PlanID != fmt.Sprint(p.PlanID) || body.OrgID != p.OrgID || body.TesteeID != fmt.Sprint(p.TesteeID) || body.ScheduleRevision != p.ScheduleRevision ||
		!body.OpenAt.UTC().Round(time.Microsecond).Round(time.Millisecond).Equal(*p.OpenAt) {
		return transaction.ErrBinding
	}
	return nil
}

func read(raw []byte) (frozenOpening, error) {
	var f frozenOpening
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, err
	}
	canonical, err := json.Marshal(f)
	if err != nil || !bytes.Equal(canonical, raw) {
		return f, transaction.ErrInvalidPlan
	}
	return f, f.validate()
}
