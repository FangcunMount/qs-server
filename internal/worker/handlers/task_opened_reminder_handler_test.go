package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/stretchr/testify/require"
)

type reminderHandlerClientStub struct {
	InternalClient
	request *pb.ProcessTaskOpenedReminderRequest
	err     error
}

func (s *reminderHandlerClientStub) ProcessTaskOpenedReminder(
	_ context.Context, req *pb.ProcessTaskOpenedReminderRequest,
) error {
	s.request = req
	return s.err
}

func TestTaskOpenedReminderHandlerPassesImmutableReferenceAndNacksTransportFailure(t *testing.T) {
	opened := time.Date(2026, 9, 26, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	raw, err := json.Marshal(map[string]any{
		"id": "event-1", "eventType": "task.opened.reminder.requested",
		"occurredAt": opened, "aggregateType": "AssessmentTask", "aggregateID": "task-1",
		"data": map[string]any{
			"task_id": "task-1", "plan_id": "plan-1", "org_id": 7,
			"testee_id": "12", "open_at": opened, "schedule_revision": 3,
		},
	})
	require.NoError(t, err)
	client := &reminderHandlerClientStub{err: errors.New("API unavailable")}
	handler := handleTaskOpenedReminder(&Dependencies{InternalClient: client})
	require.ErrorContains(t, handler(context.Background(), "task.opened.reminder.requested", raw), "API unavailable")
	require.Equal(t, "event-1", client.request.GetOpeningEventId())
	require.EqualValues(t, 7, client.request.GetOrgId())
	require.EqualValues(t, 3, client.request.GetScheduleRevision())
	require.True(t, opened.Equal(client.request.GetOpenAt().AsTime()), "protobuf normalizes the same UTC+8 instant to UTC")
	require.NotContains(t, string(raw), "entry_url", "bearer URL must not be carried by the durable event")
	client.err = nil
	require.NoError(t, handler(context.Background(), "task.opened.reminder.requested", raw))
}
