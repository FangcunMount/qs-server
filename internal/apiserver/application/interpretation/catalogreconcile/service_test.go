package catalogreconcile

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	counts    DriftCounts
	err       error
	plan      RepairPlan
	recovered string
	pages     []DriftPage
	listCalls int
	authority OutcomeAssociation
}

func (f *fakeStore) CountDrifts(context.Context, Filter) (DriftCounts, error) {
	return f.counts, f.err
}

func (f *fakeStore) ListDrifts(context.Context, Filter, string, int) (DriftPage, error) {
	if f.listCalls < len(f.pages) {
		page := f.pages[f.listCalls]
		f.listCalls++
		return page, f.err
	}
	return DriftPage{Items: []DriftItem{{AssessmentID: 1, Kind: DriftDangling, Version: "v1", Source: "artifact"}}}, f.err
}
func (f *fakeStore) SaveRepairPlan(_ context.Context, plan RepairPlan) error {
	f.plan = plan
	return f.err
}

type archiveAuthorityStub struct {
	association OutcomeAssociation
	err         error
}

func (a archiveAuthorityStub) FindCommittedOutcome(context.Context, uint64) (OutcomeAssociation, error) {
	return a.association, a.err
}
func (f *fakeStore) FindRepairPlan(context.Context, string) (RepairPlan, error) { return f.plan, f.err }
func (f *fakeStore) RecoverArchiveAssociation(context.Context, uint64, OutcomeAssociation) (string, error) {
	if f.recovered == "" {
		return "already_repaired", f.err
	}
	return f.recovered, f.err
}
func (f *fakeStore) ApplyRepair(context.Context, RepairPlan) (string, error) {
	return "repaired", f.err
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

func TestListDriftsRequiresStableKind(t *testing.T) {
	t.Parallel()
	service := NewService(&fakeStore{})
	if _, err := service.ListDrifts(context.Background(), Filter{}, "", 500); err == nil {
		t.Fatal("expected drift kind error")
	}
	page, err := service.ListDrifts(context.Background(), Filter{Kind: DriftDangling}, "", 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != DriftDangling {
		t.Fatalf("page = %#v", page)
	}
}

func TestReconcileOnceRejectsMissingStore(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	if _, err := service.ReconcileOnce(context.Background(), Filter{}); err == nil {
		t.Fatal("expected missing store error")
	}
}

func TestRepairRecoversArchiveAssociationFromCommittedOutcome(t *testing.T) {
	t.Parallel()
	item := DriftItem{
		AssessmentID: 7, Kind: DriftAssociationMismatch, Source: "archive",
		Version: "v1", Fields: []string{"org_id"},
	}
	store := &fakeStore{
		plan: RepairPlan{
			DryRunID: "dry-1", OrgID: 9, Item: item,
			ExpiresAt: time.Now().Add(time.Hour),
		},
		recovered: "repaired",
		pages:     []DriftPage{{}},
	}
	service := NewService(store)
	service.BindArchiveAuthority(archiveAuthorityStub{association: OutcomeAssociation{
		OutcomeID: 11, OrgID: 9, AssessmentID: 7, TesteeID: 13,
	}})
	result, err := service.Repair(context.Background(), RepairCommand{
		OrgID: 9, DryRunID: "dry-1", ExpectedCatalogVersion: "v1", ExpectedSource: "archive",
	})
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if result.Status != "repaired" {
		t.Fatalf("status = %q, want repaired", result.Status)
	}
}
