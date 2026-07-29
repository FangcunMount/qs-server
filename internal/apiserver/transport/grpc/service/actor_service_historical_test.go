package service

import (
	"context"
	"testing"
	"time"

	actorpb "github.com/FangcunMount/qs-server/api/grpc/gen/actor"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
)

type actorRegistrationCapture struct {
	dto        testeeApp.RegisterTesteeDTO
	historical historicalseed.Context
	hasHistory bool
}

func (c *actorRegistrationCapture) Register(ctx context.Context, dto testeeApp.RegisterTesteeDTO) (*testeeApp.TesteeResult, error) {
	c.dto = dto
	c.historical, c.hasHistory = historicalseed.FromContext(ctx)
	result := &testeeApp.TesteeResult{ID: 1, OrgID: dto.OrgID, Name: dto.Name, Gender: dto.Gender, Birthday: dto.Birthday, Source: dto.Source}
	return result, nil
}

func (*actorRegistrationCapture) EnsureByProfile(context.Context, testeeApp.EnsureTesteeDTO) (*testeeApp.TesteeResult, error) {
	return nil, nil
}

func (*actorRegistrationCapture) GetMyProfile(context.Context, int64, uint64) (*testeeApp.TesteeResult, error) {
	return nil, nil
}

func TestActorServiceUsesVerifiedHistoricalTesteeCreatedAt(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	createdAt := time.Date(2025, 1, 1, 8, 42, 0, 0, location)
	historical := historicalseed.Context{
		BatchID: "batch", ScenarioID: "2025-01-01/1/create_testee/scale", OrgID: 1, Version: historicalseed.Version1,
		Timeline: historicalseed.Timeline{TesteeCreatedAt: &createdAt},
	}
	registration := &actorRegistrationCapture{}
	service := NewActorService(registration, nil, nil, nil)
	service.SetHistoricalSeedVerifier(&historicalseed.Verifier{
		Enabled: true, AllowedOrgIDs: map[uint64]struct{}{1: {}}, Location: location,
		Earliest: time.Date(2025, 1, 1, 0, 0, 0, 0, location), Latest: time.Date(2026, 7, 27, 0, 0, 0, 0, location),
	})

	_, err := service.CreateTestee(context.Background(), &actorpb.CreateTesteeRequest{
		OrgId: 1, Name: "testee", Gender: 1, HistoricalContext: historicalseed.ToProto(historical),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !registration.hasHistory || registration.historical.Timeline.TesteeCreatedAt == nil || !registration.historical.Timeline.TesteeCreatedAt.Equal(createdAt) {
		t.Fatalf("registration historical context=%+v, want testee_created_at=%s", registration.historical, createdAt)
	}
}

func TestActorServiceOrdinaryCreateDoesNotOverrideCreatedAt(t *testing.T) {
	registration := &actorRegistrationCapture{}
	service := NewActorService(registration, nil, nil, nil)
	if _, err := service.CreateTestee(context.Background(), &actorpb.CreateTesteeRequest{OrgId: 1, Name: "testee", Gender: 1}); err != nil {
		t.Fatal(err)
	}
	if registration.hasHistory {
		t.Fatalf("ordinary registration unexpectedly received historical context: %+v", registration.historical)
	}
}
