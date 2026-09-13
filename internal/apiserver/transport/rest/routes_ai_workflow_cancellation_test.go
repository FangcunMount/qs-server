package rest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

func (g *candidateRouteGateway) CancelEvaluation(_ context.Context, scope app.EvaluationScope, _ app.EvaluationCancel) (app.EvaluationState, error) {
	g.writes++
	return app.EvaluationState{RunID: scope.RunID}, nil
}

func TestWorkflowCancelRouteRequiresOrgAdminAndGovernanceDependency(t *testing.T) {
	for _, admin := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			gateway := &candidateRouteGateway{}
			deps := Deps{}
			if enabled {
				deps.Interpretation.AIWorkflowManagement = &app.EvaluationAdministration{Gateway: gateway}
			}
			router := newRouterWithBudgets(deps)
			engine := gin.New()
			engine.Use(aiRouteSnapshotMiddleware(admin))
			router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
			request := httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/00000000-0000-4000-8000-000000000001/cancel", strings.NewReader(`{"expected_version":8,"reason":"停止","confirm":true,"discard":false}`))
			request.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, request)
			want := 404
			if enabled {
				want = 403
				if admin {
					want = 200
				}
			}
			if w.Code != want || (gateway.writes == 1) != (enabled && admin) {
				t.Fatal("unexpected cancellation route access", w.Code, gateway.writes)
			}
		}
	}
}
