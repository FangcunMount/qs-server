package testee

import (
	"context"
	"testing"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
)

type historicalRegistrationRepository struct {
	saved *domain.Testee
}

func (r *historicalRegistrationRepository) Save(_ context.Context, testee *domain.Testee) error {
	r.saved = testee
	return nil
}

func (*historicalRegistrationRepository) Update(context.Context, *domain.Testee) error { return nil }
func (*historicalRegistrationRepository) FindByID(context.Context, domain.ID) (*domain.Testee, error) {
	return nil, nil
}
func (*historicalRegistrationRepository) FindByProfile(context.Context, int64, uint64) (*domain.Testee, error) {
	return nil, nil
}
func (*historicalRegistrationRepository) Delete(context.Context, domain.ID) error { return nil }

func newHistoricalRegistrationService(repo *historicalRegistrationRepository) TesteeRegistrationService {
	uow := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	})
	return NewRegistrationService(repo, nil, domain.NewValidator(repo), nil, uow, nil)
}

func TestRegistrationServicePersistsHistoricalTesteeCreatedAt(t *testing.T) {
	createdAt := time.Date(2025, 1, 1, 8, 42, 0, 0, time.FixedZone("CST", 8*60*60))
	historical := historicalseed.Context{
		BatchID: "batch", ScenarioID: "2025-01-01/1/create_testee/scale", OrgID: 1, Version: historicalseed.Version1,
		Timeline: historicalseed.Timeline{TesteeCreatedAt: &createdAt},
	}
	repo := &historicalRegistrationRepository{}
	service := newHistoricalRegistrationService(repo)

	result, err := service.Register(historicalseed.WithContext(context.Background(), historical), RegisterTesteeDTO{OrgID: 1, Name: "testee", Gender: 1})
	if err != nil {
		t.Fatal(err)
	}
	if repo.saved == nil || !repo.saved.CreatedAt().Equal(createdAt) || !result.CreatedAt.Equal(createdAt) {
		t.Fatalf("saved=%v result.created_at=%s, want %s", repo.saved, result.CreatedAt, createdAt)
	}
}

func TestRegistrationServiceLeavesOrdinaryCreatedAtForRepository(t *testing.T) {
	repo := &historicalRegistrationRepository{}
	service := newHistoricalRegistrationService(repo)

	if _, err := service.Register(context.Background(), RegisterTesteeDTO{OrgID: 1, Name: "testee", Gender: 1}); err != nil {
		t.Fatal(err)
	}
	if repo.saved == nil || !repo.saved.CreatedAt().IsZero() {
		t.Fatalf("ordinary created_at=%v, want zero for repository auto timestamp", repo.saved)
	}
}

func TestRegistrationServiceFailsClosedWhenHistoricalTesteeTimeIsMissing(t *testing.T) {
	historical := historicalseed.Context{BatchID: "batch", ScenarioID: "scenario", OrgID: 1, Version: historicalseed.Version1}
	repo := &historicalRegistrationRepository{}
	service := newHistoricalRegistrationService(repo)

	if _, err := service.Register(historicalseed.WithContext(context.Background(), historical), RegisterTesteeDTO{OrgID: 1, Name: "testee", Gender: 1}); err == nil {
		t.Fatal("expected missing historical testee time error")
	}
	if repo.saved != nil {
		t.Fatal("testee was persisted after missing historical time")
	}
}
