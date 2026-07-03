package handlers

import (
	"fmt"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func parseFootprintPayload[T any](payload []byte, label string, data *T) (*EventEnvelope, T, error) {
	var zero T
	env, err := ParseEventData(payload, data)
	if err != nil {
		return nil, zero, fmt.Errorf("failed to parse footprint %s event: %w", label, err)
	}
	return env, *data, nil
}

func newBehaviorRequest(env *EventEnvelope, eventType string, orgID int64, occurredAt time.Time) *pb.ProjectBehaviorEventRequest {
	return &pb.ProjectBehaviorEventRequest{
		EventId:    env.ID,
		EventType:  eventType,
		OrgId:      orgID,
		OccurredAt: timestamppb.New(occurredAt),
	}
}
