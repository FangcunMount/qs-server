package answersheet

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type actorLookupClientStub struct {
	getResults map[uint64]*grpcclient.TesteeResponse
	getErrors  map[uint64]error
	existsIDs  map[uint64]uint64
}

func (s *actorLookupClientStub) GetTestee(_ context.Context, testeeID uint64) (*grpcclient.TesteeResponse, error) {
	if err, ok := s.getErrors[testeeID]; ok {
		return nil, err
	}
	if result, ok := s.getResults[testeeID]; ok {
		return result, nil
	}
	return nil, status.Error(codes.NotFound, "受试者不存在")
}

func (s *actorLookupClientStub) TesteeExists(_ context.Context, _ uint64, iamChildID uint64) (bool, uint64, error) {
	if id, ok := s.existsIDs[iamChildID]; ok {
		return true, id, nil
	}
	return false, 0, nil
}

func TestResolveCanonicalTesteeReturnsOriginalID(t *testing.T) {
	stub := &actorLookupClientStub{
		getResults: map[uint64]*grpcclient.TesteeResponse{
			615001: {ID: 615001, Name: "王小明"},
		},
		getErrors: map[uint64]error{},
		existsIDs: map[uint64]uint64{},
	}
	service := &SubmissionService{actorClient: stub}

	testee, resolvedID, err := service.resolveCanonicalTestee(context.Background(), 615001)
	if err != nil {
		t.Fatalf("resolve canonical testee: %v", err)
	}
	if resolvedID != 615001 {
		t.Fatalf("expected resolved id 615001, got %d", resolvedID)
	}
	if testee == nil || testee.ID != 615001 {
		t.Fatalf("unexpected testee: %+v", testee)
	}
}

func TestResolveCanonicalTesteeFallsBackFromProfileID(t *testing.T) {
	const (
		profileID         = 615966157324694062
		canonicalTesteeID = 615969735435104814
	)

	stub := &actorLookupClientStub{
		getResults: map[uint64]*grpcclient.TesteeResponse{
			canonicalTesteeID: {
				ID:         canonicalTesteeID,
				OrgID:      1,
				IAMChildID: "615966157324694062",
				Name:       "宋博文",
			},
		},
		getErrors: map[uint64]error{
			profileID: status.Error(codes.NotFound, "受试者不存在"),
		},
		existsIDs: map[uint64]uint64{
			profileID: canonicalTesteeID,
		},
	}
	service := &SubmissionService{
		actorClient:         stub,
		guardianshipService: new(iam.GuardianshipService),
	}

	testee, resolvedID, err := service.resolveCanonicalTestee(context.Background(), profileID)
	if err != nil {
		t.Fatalf("resolve canonical testee with profile fallback: %v", err)
	}
	if resolvedID != canonicalTesteeID {
		t.Fatalf("expected canonical id %d, got %d", canonicalTesteeID, resolvedID)
	}
	if testee == nil || testee.ID != canonicalTesteeID {
		t.Fatalf("unexpected canonical testee: %+v", testee)
	}
}

func TestNewSubmissionServiceAlwaysInitializesQueue(t *testing.T) {
	service := NewSubmissionService(nil, nil, nil, &options.SubmitQueueOptions{
		Enabled:     false,
		QueueSize:   8,
		WorkerCount: 1,
	}, nil)

	if service.queue == nil {
		t.Fatal("expected submit queue to be initialized even when enabled=false")
	}
}
