package taskopen

import (
	"bytes"
	"encoding/json"
	"fmt"
	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	appplan "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type taskRoutes struct{}

func (taskRoutes) GetTopicForEvent(string) (string, bool) { return "qs.plan.tasks", true }

func openedTask(t *testing.T) *domain.AssessmentTask {
	t.Helper()
	at := time.Date(2026, 10, 20, 13, 40, 0, 123456789, time.UTC)
	task := domain.NewAssessmentTaskAt(domain.NewAssessmentPlanID(), 1, 7, testee.NewID(9), "scale", at.Add(-time.Minute), at.Add(-time.Hour))
	require.NoError(t, domain.NewTaskLifecycle().OpenAt(t.Context(), task, "https://entry.example.test/first-private-entry", at))
	return task
}

func TestOpeningPlanPreservesFirstEntryWithoutExposingItInReminderWire(t *testing.T) {
	task := openedTask(t)
	q, err := freeze(task, 42)
	require.NoError(t, err)
	f, err := read(q.Artifact)
	require.NoError(t, err)
	require.Equal(t, task.GetEntryURL(), f.Task.EntryURL)
	require.Equal(t, uint64(42), f.UpdatedBy)
	require.True(t, task.GetOpenAt().UTC().Truncate(time.Millisecond).Equal(*f.Task.OpenAt))
	plan, err := shared.Freeze(q, taskRoutes{}, "qs.mysql", "m3-1")
	require.NoError(t, err)
	require.Len(t, plan.Input().Branches, 1)
	wire, err := standard.EncodeWire(q.Events[0], "api-server")
	require.NoError(t, err)
	require.Equal(t, wire, plan.Input().Branches[0].Wire)
	require.NotContains(t, string(wire), task.GetEntryURL())
	encoded, err := transaction.EncodePlan(plan)
	require.NoError(t, err)
	decoded, err := transaction.DecodePlan(encoded)
	require.NoError(t, err)
	raw, err := shared.Artifact(decoded, "qs.mysql", "m3-1")
	require.NoError(t, err)
	require.Equal(t, q.Artifact, raw)
	require.Equal(t, wire, decoded.Input().Branches[0].Wire)
	// Preparation does not change the caller's domain clock or collected event.
	require.Equal(t, 123456789, task.GetOpenAt().Nanosecond())
	require.Len(t, task.Events(), 1)
}

func TestOpeningStableInputIgnoresCandidateArtifactsButIncludesSchedule(t *testing.T) {
	q, err := freeze(openedTask(t), 1)
	require.NoError(t, err)
	f, err := read(q.Artifact)
	require.NoError(t, err)
	want, err := f.inputHash()
	require.NoError(t, err)
	copy := f
	copy.Task.EntryURL = "https://entry.example.test/competing-entry"
	at := copy.Task.OpenAt.Add(time.Second)
	copy.Task.OpenAt = &at
	copy.UpdatedAt = at
	copy.UpdatedBy = 2
	hash, err := copy.inputHash()
	require.NoError(t, err)
	require.Equal(t, want, hash, "first entry/clock/actor are reservation winner artifacts")
	for _, field := range []string{"revision", "scale", "planned", "org", "testee"} {
		t.Run(field, func(t *testing.T) {
			copy := f
			switch field {
			case "revision":
				copy.Task.ScheduleRevision++
			case "scale":
				copy.Task.ScaleCode = "other"
			case "planned":
				copy.Task.PlannedAt = copy.Task.PlannedAt.Add(time.Second)
			case "org":
				copy.Task.OrgID++
			case "testee":
				copy.Task.TesteeID++
			}
			hash, err := copy.inputHash()
			require.NoError(t, err)
			require.NotEqual(t, want, hash)
		})
	}
	copy = f
	copy.Task.PlannedAt = copy.Task.PlannedAt.In(time.FixedZone("east", 8*3600))
	hash, err = copy.inputHash()
	require.NoError(t, err)
	require.Equal(t, want, hash)
}

func TestOpeningPlanRejectsCrossScopeBodyAndNoncanonicalArtifact(t *testing.T) {
	q, err := freeze(openedTask(t), 1)
	require.NoError(t, err)
	f, err := read(q.Artifact)
	require.NoError(t, err)
	f.Reminder = bytes.ReplaceAll(f.Reminder, []byte(`"org_id":7`), []byte(`"org_id":8`))
	raw, err := json.Marshal(f)
	require.NoError(t, err)
	_, err = read(raw)
	require.Error(t, err)
	_, err = read(append(q.Artifact, '\n'))
	require.ErrorIs(t, err, transaction.ErrInvalidPlan)
	_, err = freeze(nil, 0)
	require.Error(t, err)
}

func TestFrozenOpeningRetainsExistingReminderEligibilityAtMySQLRoundingBoundaries(t *testing.T) {
	for _, nanos := range []int{123499499, 123499501, 123500000, 999499501, 999999999} {
		t.Run(fmt.Sprint(nanos), func(t *testing.T) {
			at := time.Date(2026, 10, 20, 14, 0, 0, nanos, time.UTC)
			task := domain.NewAssessmentTaskAt(domain.NewAssessmentPlanID(), 1, 7, testee.NewID(9), "scale", at.Add(-time.Minute), at.Add(-time.Hour))
			require.NoError(t, domain.NewTaskLifecycle().OpenAt(t.Context(), task, "https://example.test/first", at))
			request, err := freeze(task, 1)
			require.NoError(t, err)
			frozen, err := read(request.Artifact)
			require.NoError(t, err)
			decision, err := notification.EvaluateTaskOpenedReminder(notification.TaskOpenedReminderIntent{OrgID: 7, TaskID: task.GetID().String(), TesteeID: task.GetTesteeID().String(), ScheduleRevision: task.GetScheduleRevision(), OpenAt: at}, &appplan.TaskReminderState{OrgID: 7, TaskID: task.GetID().String(), TesteeID: task.GetTesteeID().String(), ScheduleRevision: task.GetScheduleRevision(), Status: domain.TaskStatusOpened, OpenAt: frozen.Task.OpenAt, ExpireAt: frozen.Task.ExpireAt, EntryURL: frozen.Task.EntryURL}, at.Add(time.Second))
			require.NoError(t, err)
			require.Empty(t, decision.SuppressCode, "frozen business clocks must match the unchanged reminder eligibility policy")
		})
	}
}
