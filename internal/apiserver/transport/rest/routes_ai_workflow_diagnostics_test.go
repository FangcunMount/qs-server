package rest

import (
	"context"
	"net/http/httptest"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type diagnosticRouteGateway struct {
	app.EvaluationGateway
	calls int
	scope app.EvaluationScope
}

func (g *diagnosticRouteGateway) ListEvaluationExecutions(_ context.Context, s app.EvaluationScope, q app.ExecutionQuery) (app.ExecutionPage, error) {
	g.calls++
	g.scope = s
	return app.ExecutionPage{RunID: s.RunID, Version: q.ExpectedVersion, Executions: []app.ExecutionSummary{}}, nil
}
func (g *diagnosticRouteGateway) GetEvaluationExecutionOutput(_ context.Context, s app.EvaluationScope, v int64, id string) (app.ExecutionOutput, error) {
	g.calls++
	g.scope = s
	return app.ExecutionOutput{RunID: s.RunID, Version: v, Execution: app.ExecutionSummary{ExecutionID: id}}, nil
}
func TestDiagnosticRoutesRequireCurrentAuditAndBoundVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := "/internal/v2/interpretation/ai-workflow/evaluations/44444444-4444-4444-8444-444444444444/executions"
	for _, authorized := range []bool{false, true} {
		g := &diagnosticRouteGateway{}
		engine := gin.New()
		if authorized {
			engine.Use(aiRouteSnapshotMiddleware(false))
		}
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowManagement: &app.EvaluationAdministration{Gateway: g}}})
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		for _, suffix := range []string{"", "/execution:1/output"} {
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest("GET", base+suffix+"?expected_version=7&organization_id=99&operator_user_id=99", nil))
			if authorized && (w.Code != 200 || g.scope.OrganizationID != 12 || g.scope.OperatorUserID != 34) {
				t.Fatal(w.Code, w.Body.String(), g)
			}
			if !authorized && (w.Code == 200 || g.calls != 0) {
				t.Fatal("unauthorized read forwarded")
			}
		}
		if authorized {
			for _, invalid := range []string{"", "expected_version=0", "expected_version=1&expected_version=2", "expected_version=7&limit=51", "expected_version=7&cursor=bad%20id"} {
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, httptest.NewRequest("GET", base+"?"+invalid, nil))
				if w.Code != 400 || g.calls != 2 {
					t.Fatal(invalid, w.Code, g.calls)
				}
			}
		}
	}
}
