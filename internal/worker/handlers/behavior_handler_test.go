package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestHandleBehaviorProjector_MissingInternalClient(t *testing.T) {
	handler := handleBehaviorProjector(&Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	err := handler(context.Background(), eventcatalog.FootprintEntryOpened, mustBuildBehaviorEventPayload(t, "evt-1", eventcatalog.FootprintEntryOpened, map[string]any{
		"org_id":       int64(1),
		"clinician_id": uint64(11),
		"entry_id":     uint64(21),
		"occurred_at":  time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC),
	}))
	if err == nil || !strings.Contains(err.Error(), "internal client is not available") {
		t.Fatalf("expected missing internal client error, got %v", err)
	}
}

func TestHandleBehaviorProjector_EntryOpenedProjectsBehaviorEvent(t *testing.T) {
	occurredAt := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleBehaviorProjector(deps)

	err := handler(context.Background(), eventcatalog.FootprintEntryOpened, mustBuildBehaviorEventPayload(t, "evt-entry-opened", eventcatalog.FootprintEntryOpened, map[string]any{
		"org_id":       int64(7),
		"clinician_id": uint64(101),
		"entry_id":     uint64(202),
		"occurred_at":  occurredAt,
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if client.projectBehaviorCalls != 1 {
		t.Fatalf("expected 1 projection call, got %d", client.projectBehaviorCalls)
	}
	req := client.projectBehaviorRequest
	if req == nil {
		t.Fatal("expected projection request to be captured")
	}
	if req.GetEventId() != "evt-entry-opened" || req.GetEventType() != eventcatalog.FootprintEntryOpened {
		t.Fatalf("unexpected request identity: %#v", req)
	}
	if req.GetOrgId() != 7 || req.GetClinicianId() != 101 || req.GetEntryId() != 202 {
		t.Fatalf("unexpected request payload: %#v", req)
	}
	if req.GetOccurredAt().AsTime() != occurredAt {
		t.Fatalf("unexpected occurred_at: %v", req.GetOccurredAt().AsTime())
	}
}

func TestHandleBehaviorProjector_CareRelationshipTransferredMapsClinicians(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}
	handler := handleBehaviorProjector(deps)

	err := handler(context.Background(), eventcatalog.FootprintCareRelationshipTransferred, mustBuildBehaviorEventPayload(t, "evt-transfer", eventcatalog.FootprintCareRelationshipTransferred, map[string]any{
		"org_id":            int64(3),
		"from_clinician_id": uint64(10),
		"to_clinician_id":   uint64(20),
		"testee_id":         uint64(30),
		"occurred_at":       time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC),
	}))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	req := client.projectBehaviorRequest
	if req == nil {
		t.Fatal("expected projection request to be captured")
	}
	if req.GetClinicianId() != 20 || req.GetSourceClinicianId() != 10 || req.GetTesteeId() != 30 {
		t.Fatalf("unexpected transfer mapping: %#v", req)
	}
}

func TestHandleBehaviorProjector_UnsupportedEventType(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	handler := handleBehaviorProjector(&Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	})

	err := handler(context.Background(), "footprint.unknown", []byte(`{"id":"evt-x","eventType":"footprint.unknown","data":{}}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported behavior event type "footprint.unknown"`) {
		t.Fatalf("expected unsupported event error, got %v", err)
	}
	if client.projectBehaviorCalls != 0 {
		t.Fatalf("expected no projection call, got %d", client.projectBehaviorCalls)
	}
}

type nilResponseBehaviorClient struct {
	fakeWorkerInternalClient
}

func (c *nilResponseBehaviorClient) ProjectBehaviorEvent(
	_ context.Context,
	_ *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	return nil, nil
}

func TestProjectBehaviorEvent_NilResponseReturnsError(t *testing.T) {
	deps := &Dependencies{InternalClient: &nilResponseBehaviorClient{}}
	req := &pb.ProjectBehaviorEventRequest{
		EventId:   "evt-nil",
		EventType: eventcatalog.FootprintReportGenerated,
		OrgId:     1,
	}

	err := projectBehaviorEvent(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), deps, req)
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("expected empty response error, got %v", err)
	}
}

func TestProjectBehaviorEvent_PropagatesGRPCError(t *testing.T) {
	client := &fakeWorkerInternalClient{projectBehaviorErr: errors.New("grpc unavailable")}
	deps := &Dependencies{InternalClient: client}

	err := projectBehaviorEvent(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), deps, &pb.ProjectBehaviorEventRequest{
		EventId:   "evt-grpc",
		EventType: eventcatalog.FootprintAssessmentCreated,
	})
	if err == nil || !strings.Contains(err.Error(), "grpc unavailable") {
		t.Fatalf("expected grpc error, got %v", err)
	}
}

func TestBehaviorProjectLogFields_NilRequest(t *testing.T) {
	fields := behaviorProjectLogFields(nil)
	if len(fields) != 1 {
		t.Fatalf("expected 1 field for nil request, got %d", len(fields))
	}
}

func TestBehaviorProjectLogFields_IncludesOptionalFields(t *testing.T) {
	occurredAt := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	fields := behaviorProjectLogFields(&pb.ProjectBehaviorEventRequest{
		EventId:       "evt-fields",
		EventType:     eventcatalog.FootprintAssessmentCreated,
		OrgId:         9,
		FailureReason: "timeout",
		OccurredAt:    timestamppb.New(occurredAt),
	})
	if len(fields) < 12 {
		t.Fatalf("expected optional fields to be included, got %d attrs", len(fields))
	}
}

func mustBuildBehaviorEventPayload(t *testing.T, eventID, eventType string, data map[string]any) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"id":            eventID,
		"eventType":     eventType,
		"occurredAt":    time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC),
		"aggregateType": "BehaviorFootprint",
		"aggregateID":   "agg-1",
		"data":          data,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}
