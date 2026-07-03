package acl

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
)

type grpcActorReaderStub struct {
	testee *grpcbridge.TesteeResponse
	exists bool
	id     uint64
	err    error
}

func (s *grpcActorReaderStub) GetTestee(context.Context, uint64) (*grpcbridge.TesteeResponse, error) {
	return s.testee, s.err
}

func (s *grpcActorReaderStub) TesteeExists(context.Context, uint64, uint64) (bool, uint64, error) {
	return s.exists, s.id, s.err
}

type grpcActorWriterStub struct {
	grpcActorReaderStub
}

func (s *grpcActorWriterStub) CreateTestee(context.Context, *grpcbridge.CreateTesteeRequest) (*grpcbridge.TesteeResponse, error) {
	return nil, nil
}

func (s *grpcActorWriterStub) GetTesteeCareContext(context.Context, uint64) (*grpcbridge.TesteeCareContextResponse, error) {
	return nil, nil
}

func (s *grpcActorWriterStub) UpdateTestee(context.Context, *grpcbridge.UpdateTesteeRequest) (*grpcbridge.TesteeResponse, error) {
	return nil, nil
}

func (s *grpcActorWriterStub) ListTesteesByUser(context.Context, []uint64, int32, int32) ([]*grpcbridge.TesteeResponse, int64, error) {
	return nil, 0, nil
}

func testeeGRPCResponse() *grpcbridge.TesteeResponse {
	return &grpcbridge.TesteeResponse{
		ID:           9,
		OrgID:        42,
		IAMProfileID: "profile-1",
		Name:         "Alice",
		Birthday:     time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC),
	}
}

func TestTesteeActorLookupMapsActorTestee(t *testing.T) {
	t.Parallel()

	lookup := NewTesteeActorLookup(&grpcActorReaderStub{
		testee: &grpcbridge.TesteeResponse{
			OrgID:        42,
			IAMProfileID: "profile-1",
			Name:         "Alice",
		},
	})
	got, err := lookup.GetTestee(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.OrgID != 42 || got.IAMProfileID != "profile-1" || got.Name != "Alice" {
		t.Fatalf("GetTestee() = %+v", got)
	}
}

func TestTesteeActorAdapterReusesActorReaderBridge(t *testing.T) {
	t.Parallel()

	adapter := NewTesteeActorAdapter(&grpcActorWriterStub{
		grpcActorReaderStub: grpcActorReaderStub{testee: testeeGRPCResponse()},
	})
	got, err := adapter.GetTestee(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "9" || got.Name != "Alice" {
		t.Fatalf("GetTestee() = %+v", got)
	}
}

func TestActorReaderBridgeTesteeExists(t *testing.T) {
	t.Parallel()

	lookup := NewTesteeActorLookup(&grpcActorReaderStub{exists: true, id: 99})
	exists, id, err := lookup.TesteeExists(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || id != 99 {
		t.Fatalf("TesteeExists() = (%v, %d)", exists, id)
	}
}
