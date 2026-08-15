package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	interpretationcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type catalogSnapshotServiceStub struct {
	snapshot interpretationcatalog.AuditSnapshot
	err      error
}

func (s catalogSnapshotServiceStub) LatestAuditSnapshot(context.Context, int64) (interpretationcatalog.AuditSnapshot, error) {
	return s.snapshot, s.err
}
func (catalogSnapshotServiceStub) ListDrifts(context.Context, interpretationcatalog.Filter, string, int) (interpretationcatalog.DriftPage, error) {
	return interpretationcatalog.DriftPage{}, nil
}
func (catalogSnapshotServiceStub) CreateRepairPlan(context.Context, int64, interpretationcatalog.Filter) (interpretationcatalog.RepairPlan, error) {
	return interpretationcatalog.RepairPlan{}, nil
}
func (catalogSnapshotServiceStub) Repair(context.Context, interpretationcatalog.RepairCommand) (interpretationcatalog.RepairResult, error) {
	return interpretationcatalog.RepairResult{}, nil
}
func TestCatalogReconcileReturnsCompletedSnapshotHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	completedAt := time.Date(2026, 8, 5, 12, 0, 0, 123, time.UTC)
	handler := NewInterpretationCatalogReconcileHandler(catalogSnapshotServiceStub{snapshot: interpretationcatalog.AuditSnapshot{
		CycleID: "cycle-1", CompletedAt: completedAt, Counts: interpretationcatalog.DriftCounts{Missing: 2},
	}})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/catalog/reconcile", nil)
	ctx.Set(middleware.OrgIDKey, uint64(1))
	ctx.Set(middleware.UserIDKey, uint64(2))

	handler.Reconcile(ctx)

	if recorder.Code != http.StatusOK || recorder.Header().Get("X-QS-Catalog-Audit-Cycle-ID") != "cycle-1" || recorder.Header().Get("X-QS-Catalog-Audit-Completed-At") != completedAt.Format(time.RFC3339Nano) {
		t.Fatalf("response = %d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"Missing":2`) {
		t.Fatalf("snapshot counts missing from response: %s", recorder.Body.String())
	}
}

func TestCatalogReconcileReturnsStableNotReady503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewInterpretationCatalogReconcileHandler(catalogSnapshotServiceStub{err: interpretationcatalog.ErrAuditNotReady})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/catalog/reconcile", nil)
	ctx.Set(middleware.OrgIDKey, uint64(1))
	ctx.Set(middleware.UserIDKey, uint64(2))

	handler.Reconcile(ctx)

	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "catalog_audit_not_ready") {
		t.Fatalf("response = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
