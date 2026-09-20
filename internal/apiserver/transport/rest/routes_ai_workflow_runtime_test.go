package rest

import (
	"context"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

type runtimeRouteStore struct {
	calls int
	org   int64
	query app.RuntimeQuery
}

func (s *runtimeRouteStore) ListRuntime(_ context.Context, org int64, q app.RuntimeQuery) (app.RuntimePage, error) {
	s.calls++
	s.org = org
	s.query = q
	return app.RuntimePage{Items: []app.RuntimeRequest{}}, nil
}
func (s *runtimeRouteStore) GetRuntime(_ context.Context, org int64, id string) (app.RuntimeRequest, error) {
	s.calls++
	s.org = org
	return app.RuntimeRequest{RequestID: id}, nil
}
func TestRuntimeRoutesKeepTrustedScopeAndRejectAmbiguousQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		path          string
		admin         bool
		status, calls int
	}{
		{"?assessment_id=42&organization_id=99", true, 200, 1},
		{"?assessment_id=42", false, 403, 0},
		{"?assessment_id=42&assessment_id=43", true, 400, 0},
		{"?limit=51", true, 400, 0}, {"?since=invalid", true, 400, 0},
		{"/00000000-0000-4000-8000-000000000001?organization_id=99", true, 200, 1},
		{"/00000000-0000-4000-8000-000000000001", false, 403, 0},
	} {
		t.Run(tc.path, func(t *testing.T) {
			store := &runtimeRouteStore{}
			engine := gin.New()
			engine.Use(aiRouteSnapshotMiddleware(tc.admin))
			router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowRuntime: &app.RuntimeAdministration{Store: store}}})
			router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest("GET", "/internal/v2/interpretation/ai-workflow/runtime/requests"+tc.path, nil))
			if w.Code != tc.status || store.calls != tc.calls {
				t.Fatal(w.Code, w.Body.String(), store)
			}
			if tc.calls > 0 && store.org != 12 {
				t.Fatal(store)
			}
		})
	}
}
