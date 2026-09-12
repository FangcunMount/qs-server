package rest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

func (g *candidateRouteGateway) ReopenEvaluationReview(_ context.Context, s app.EvaluationScope, _ app.EvaluationReopen) (app.EvaluationState, error) {
	g.writes++
	return app.EvaluationState{RunID: s.RunID}, nil
}

func TestWorkflowReopeningRouteRequiresOrgAdmin(t *testing.T) {
	for _, admin := range []bool{false, true} {
		gateway := &candidateRouteGateway{}
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: gateway}}})
		engine := gin.New()
		engine.Use(aiRouteSnapshotMiddleware(admin))
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		request := httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/00000000-0000-4000-8000-000000000001/reopen-review", strings.NewReader(`{"expected_version":8,"reason":"复核","confirm":true}`))
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, request)
		if admin && (w.Code != 200 || gateway.writes != 1) {
			t.Fatal("authorized reopening not forwarded", w.Code)
		}
		if !admin && (w.Code != 403 || gateway.writes != 0) {
			t.Fatal("audit-only user reopened review", w.Code)
		}
	}
}
