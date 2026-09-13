package rest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type evaluationCatalogRouteGateway struct {
	app.EvaluationGateway
	calls int
	scope app.DraftScope
	query app.EvaluationCatalogQuery
}

func (g *evaluationCatalogRouteGateway) ListEvaluations(_ context.Context, scope app.DraftScope, query app.EvaluationCatalogQuery) (app.EvaluationCatalogPage, error) {
	g.calls++
	g.scope = scope
	g.query = query
	return app.EvaluationCatalogPage{Items: []app.EvaluationSummary{}}, nil
}
func TestEvaluationCatalogRouteRequiresAuditAndIgnoresForgedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, authorized := range []bool{true, false} {
		gateway := &evaluationCatalogRouteGateway{}
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: gateway}}})
		engine := gin.New()
		if authorized {
			engine.Use(aiRouteSnapshotMiddleware(false))
		}
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest("GET", "/internal/v2/interpretation/ai-workflow/evaluations?status=requested&limit=2&organization_id=999&operator_user_id=999", nil))
		if authorized && (recorder.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 || gateway.query.Limit != 2 || gateway.query.Status != "requested") {
			t.Fatal(recorder.Code, recorder.Body.String(), gateway)
		}
		if !authorized && (recorder.Code == 200 || gateway.calls != 0) {
			t.Fatal("unauthorized query forwarded")
		}
		if authorized {
			for _, query := range []string{"limit=-1", "limit=101", "limit=no", "status=failed", "cursor=bad", "cursor=" + strings.Repeat("x", 8193)} {
				recorder = httptest.NewRecorder()
				engine.ServeHTTP(recorder, httptest.NewRequest("GET", "/internal/v2/interpretation/ai-workflow/evaluations?"+query, nil))
				if recorder.Code != 400 || gateway.calls != 1 {
					t.Fatal(query[:3], recorder.Code, gateway.calls)
				}
			}
		}
	}
}
