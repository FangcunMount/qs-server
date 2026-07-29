package grpcclient

import (
	"context"
	"testing"
	"time"

	actorpb "github.com/FangcunMount/qs-server/api/grpc/gen/actor"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"google.golang.org/grpc"
)

type actorHistoricalCaptureClient struct {
	actorpb.ActorServiceClient
	req *actorpb.CreateTesteeRequest
}

func (c *actorHistoricalCaptureClient) CreateTestee(_ context.Context, req *actorpb.CreateTesteeRequest, _ ...grpc.CallOption) (*actorpb.TesteeResponse, error) {
	c.req = req
	return &actorpb.TesteeResponse{Id: 1, OrgId: req.GetOrgId(), Name: req.GetName()}, nil
}

func TestActorClientForwardsHistoricalContextWhenCreatingTestee(t *testing.T) {
	createdAt := time.Date(2025, 1, 1, 8, 42, 0, 0, time.FixedZone("CST", 8*60*60))
	historical := historicalseed.Context{
		BatchID: "batch", ScenarioID: "2025-01-01/1/create_testee/scale", OrgID: 1, Version: historicalseed.Version1,
		Timeline: historicalseed.Timeline{TesteeCreatedAt: &createdAt},
	}
	capture := &actorHistoricalCaptureClient{}
	client := &ActorClient{client: capture, base: &Client{config: &ClientConfig{Timeout: time.Second}}}

	if _, err := client.CreateTestee(historicalseed.WithContext(context.Background(), historical), &CreateTesteeRequest{OrgID: 1, Name: "testee"}); err != nil {
		t.Fatal(err)
	}
	if capture.req == nil || capture.req.GetHistoricalContext() == nil {
		t.Fatal("historical context was not forwarded")
	}
	got, err := historicalseed.FromProto(capture.req.GetHistoricalContext())
	if err != nil {
		t.Fatal(err)
	}
	if got.Timeline.TesteeCreatedAt == nil || !got.Timeline.TesteeCreatedAt.Equal(createdAt) {
		t.Fatalf("forwarded testee_created_at=%v, want %s", got.Timeline.TesteeCreatedAt, createdAt)
	}
}

func TestActorClientLeavesOrdinaryCreateTesteeUnchanged(t *testing.T) {
	capture := &actorHistoricalCaptureClient{}
	client := &ActorClient{client: capture, base: &Client{config: &ClientConfig{Timeout: time.Second}}}

	if _, err := client.CreateTestee(context.Background(), &CreateTesteeRequest{OrgID: 1, Name: "testee"}); err != nil {
		t.Fatal(err)
	}
	if capture.req.GetHistoricalContext() != nil {
		t.Fatal("ordinary create unexpectedly carried historical context")
	}
}
