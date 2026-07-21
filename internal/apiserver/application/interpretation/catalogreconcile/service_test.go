package catalogreconcile

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	counts DriftCounts
	err    error
}

func (f *fakeStore) CountDrifts(context.Context, Filter) (DriftCounts, error) {
	return f.counts, f.err
}

func TestReconcileOnceDetectsFourDriftClasses(t *testing.T) {
	t.Parallel()

	store := &fakeStore{counts: DriftCounts{
		Missing:             1,
		Dangling:            2,
		AssociationMismatch: 3,
		WrongWinner:         4,
	}}
	service := NewService(store)
	got, err := service.ReconcileOnce(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("ReconcileOnce: %v", err)
	}
	if got != store.counts {
		t.Fatalf("counts = %#v, want %#v", got, store.counts)
	}
	if got.Total() != 10 {
		t.Fatalf("total = %d, want 10", got.Total())
	}
}

func TestReconcileOnceRejectsMissingStore(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	if _, err := service.ReconcileOnce(context.Background(), Filter{}); err == nil {
		t.Fatal("expected missing store error")
	}
}

func TestRepairRequiresExplicitAuthorization(t *testing.T) {
	t.Parallel()

	if err := Repair(context.Background(), nil, Filter{}); err == nil {
		t.Fatal("expected repair disabled without authorizer")
	}

	authorizer := repairAuthorizerStub{err: errors.New("denied")}
	if err := Repair(context.Background(), authorizer, Filter{}); err == nil {
		t.Fatal("expected repair denied")
	}

	authorizer = repairAuthorizerStub{}
	if err := Repair(context.Background(), authorizer, Filter{}); err == nil {
		t.Fatal("expected repair not implemented")
	}
}

type repairAuthorizerStub struct {
	err error
}

func (r repairAuthorizerStub) AuthorizeRepair(context.Context) error {
	if r.err != nil {
		return r.err
	}
	return nil
}
