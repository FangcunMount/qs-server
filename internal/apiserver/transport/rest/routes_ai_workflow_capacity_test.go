package rest

import (
	"context"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

type capacityRouteGateway struct {
	app.EvaluationGateway
	calls int
	scope app.DraftScope
}

func (g *capacityRouteGateway) GetEvaluationCapacity(_ context.Context, scope app.DraftScope) (app.EvaluationCapacity, error) {
	g.calls++
	g.scope = scope
	return app.EvaluationCapacity{OrganizationID: scope.OrganizationID}, nil
}
func TestCapacityUsesCurrentAdminAndTrustedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, admin := range []bool{false, true} {
		gateway := &capacityRouteGateway{}
		engine := gin.New()
		engine.Use(aiRouteSnapshotMiddleware(admin))
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: gateway}}})
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest("GET", "/internal/v2/interpretation/ai-workflow/evaluation-capacity?organization_id=99&operator_user_id=99", nil))
		if admin && (w.Code != 200 || gateway.calls != 1 || gateway.scope.OrganizationID != 12 || gateway.scope.OperatorUserID != 34) {
			t.Fatal(w.Code, gateway)
		}
		if !admin && (w.Code != 403 || gateway.calls != 0) {
			t.Fatal(w.Code, gateway)
		}
	}
}
