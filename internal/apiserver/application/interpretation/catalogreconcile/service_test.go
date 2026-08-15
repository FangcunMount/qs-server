package catalogreconcile

import (
	"context"
	"testing"
)

type fakeStore struct {
	err       error
	plan      RepairPlan
	pages     []DriftPage
	listCalls int
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

func (f *fakeStore) FindRepairPlan(context.Context, string) (RepairPlan, error) { return f.plan, f.err }
func (f *fakeStore) ApplyRepair(context.Context, RepairPlan) (string, error) {
	return "repaired", f.err
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
