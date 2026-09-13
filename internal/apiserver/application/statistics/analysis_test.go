package statistics

import (
	"context"
	"encoding/json"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/actorctx"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"strings"
	"testing"
	"time"
)

func TestAnalysisUsesOperationsPermissionAndNeverCompanyCache(t *testing.T) {
	s, store, scope, cache, ctx := operationsFixture()
	ctx = actorctx.WithOperatorOrgID(ctx, 1)
	out, err := s.AnalysisOverview(ctx, 1, OperationsFilter{StoreIDs: []uint64{10}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Scope != "stores" || len(store.scopedRange.StoreIDs) != 1 || cache.gets != 0 {
		t.Fatalf("unexpected scope or cache: %+v", out)
	}
	scope.value.StoreIDs = []uint64{20}
	if _, err = s.AnalysisOverview(ctx, 1, OperationsFilter{StoreIDs: []uint64{10}}); err == nil {
		t.Fatal("revoked store accepted")
	}
	if _, err = s.AnalysisOverview(context.Background(), 1, OperationsFilter{}); err == nil {
		t.Fatal("anonymous accepted")
	}
}

func TestAnalysisRejectsCompanyMismatchAndUnpublishedCoverage(t *testing.T) {
	s, store, _, _, ctx := operationsFixture()
	ctx = actorctx.WithOperatorOrgID(ctx, 2)
	if _, err := s.AnalysisOverview(ctx, 1, OperationsFilter{}); err == nil {
		t.Fatal("wrong company accepted")
	}
	ctx = actorctx.WithOperatorOrgID(ctx, 1)
	store.coverage = nil
	if _, err := s.AnalysisOverview(ctx, 1, OperationsFilter{}); err == nil {
		t.Fatal("missing publication coverage accepted")
	}
}

func TestAnalysisEntryWhitelistDoesNotExposeToken(t *testing.T) {
	value := analysisEntry(EntryItem{ID: 636809255101411886, ClinicianID: 636809251561419310, Token: "SECRET", TargetCode: "scale"})
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SECRET") || strings.Contains(string(b), "token") {
		t.Fatal(string(b))
	}
	if !strings.Contains(string(b), `"id":"636809255101411886"`) {
		t.Fatal("ID must remain string")
	}
}

func TestAnalysisAllEndpointsFailClosed(t *testing.T) {
	s, _, _, cache, ctx := operationsFixture()
	ctx = actorctx.WithOperatorOrgID(ctx, 1)
	for _, c := range []context.Context{context.Background(), actorctx.WithOperatorOrgID(context.Background(), 1)} {
		if _, err := s.AnalysisOverview(c, 1, OperationsFilter{}); err == nil {
			t.Fatal("overview allowed")
		}
		if _, err := s.AnalysisClinicians(c, 1, OperationsFilter{}, 1, 20); err == nil {
			t.Fatal("clinicians allowed")
		}
		if _, err := s.AnalysisEntries(c, 1, OperationsFilter{}, nil, nil, 1, 20); err == nil {
			t.Fatal("entries allowed")
		}
	}
	if cache.gets != 0 {
		t.Fatal("denied access used cache")
	}
	if _, err := s.AnalysisEntries(ctx, 1, OperationsFilter{StoreIDs: []uint64{999}}, nil, nil, 1, 20); err == nil {
		t.Fatal("foreign store allowed")
	}
}

func (s *operationsStoreStub) ScopedClinicianAnalysis(_ context.Context, _ int64, scope authz.StoreRange, _, _ time.Time, _, _ int) ([]ClinicianItem, int64, ClinicianSummary, error) {
	s.scopedRange = scope
	return []ClinicianItem{{ID: 42, Name: "visible doctor"}}, 23, ClinicianSummary{ClinicianCount: 23}, nil
}
func (s *operationsStoreStub) ScopedAnalysisClinicianVisible(_ context.Context, _ int64, _ authz.StoreRange, id uint64) (bool, error) {
	return id == 42, nil
}
func TestAnalysisDimensionsUseOperationsPermissionAndFullSummary(t *testing.T) {
	s, store, scope, cache, ctx := operationsFixture()
	ctx = actorctx.WithOperatorOrgID(ctx, 1)
	out, err := s.AnalysisClinicians(ctx, 1, OperationsFilter{}, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 23 || out.Summary.ClinicianCount != 23 || len(out.Items) != 1 || out.Page != 2 {
		t.Fatalf("pagination replaced summary: %+v", out)
	}
	id := uint64(42)
	if _, err = s.AnalysisEntries(ctx, 1, OperationsFilter{}, &id, nil, 1, 20); err != nil {
		t.Fatal(err)
	}
	if cache.gets != 0 || len(store.scopedRange.StoreIDs) != 1 {
		t.Fatal("scope/cache changed")
	}
	id = 43
	if _, err = s.AnalysisEntries(ctx, 1, OperationsFilter{}, &id, nil, 1, 20); err == nil {
		t.Fatal("invisible clinician accepted")
	}
	scope.value = authz.StoreRange{AllStores: true}
	if _, err = s.AnalysisOverview(ctx, 1, OperationsFilter{StoreIDs: []uint64{999}}); err == nil {
		t.Fatal("all stores allowed foreign company ID")
	}
}
