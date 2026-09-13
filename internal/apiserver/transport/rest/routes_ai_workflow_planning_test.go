package rest

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type planRouteGateway struct {
	app.EvaluationGateway
	calls int
	scope app.DraftScope
	query app.EvaluationPlanQuery
}

func (g *planRouteGateway) PrepareEvaluation(_ context.Context, scope app.DraftScope, query app.EvaluationPlanQuery) (app.EvaluationPlan, error) {
	g.calls++
	g.scope = scope
	g.query = query
	return app.EvaluationPlan{CandidateCount: 35}, nil
}

func TestPlanningRouteUsesAuditPermissionAndProtectedScopeOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ref := app.FrozenEvaluationRef{ID: "asset", Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64)}
	query := app.EvaluationPlanQuery{Suite: ref, GenerationRoute: ref, SemanticRoute: ref}
	body, _ := json.Marshal(query)
	forged := strings.TrimSuffix(string(body), "}") + `,"organization_id":999,"operator_user_id":999,"scope":{"organization_id":999,"operator_user_id":999}}`
	for _, authorized := range []bool{true, false} {
		gateway := &planRouteGateway{}
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: gateway}}})
		engine := gin.New()
		if authorized {
			engine.Use(aiRouteSnapshotMiddleware(false))
		}
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		w := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/prepare", strings.NewReader(forged))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, request)
		if authorized && (w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34 || gateway.query != query) {
			t.Fatal(w.Code, w.Body.String(), gateway)
		}
		if !authorized && (w.Code == 200 || gateway.calls != 0) {
			t.Fatal("unauthorized plan forwarded", w.Code)
		}
		if authorized {
			for _, invalid := range []string{`{}`, strings.Repeat(" ", 8193) + string(body)} {
				w = httptest.NewRecorder()
				request = httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/prepare", strings.NewReader(invalid))
				request.Header.Set("Content-Type", "application/json")
				engine.ServeHTTP(w, request)
				if w.Code != 400 || gateway.calls != 1 {
					t.Fatal("invalid plan forwarded", w.Code, gateway.calls)
				}
			}
		}
	}
}
