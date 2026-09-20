package rest

import (
	"context"
	"encoding/json"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

type flowRouteGateway struct {
	calls int
	scope app.DraftScope
	kind  string
}

func (g *flowRouteGateway) ReadFlow(_ context.Context, s app.DraftScope, kind, _ string) (json.RawMessage, error) {
	g.calls++
	g.scope = s
	g.kind = kind
	return json.RawMessage(`{"nodes":[]}`), nil
}
func TestFlowRoutesUseTrustedScopeAndDoNotShadowSolutionReads(t *testing.T) {
	g := &flowRouteGateway{}
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowFlows: &app.FlowAdministration{Gateway: g}, AIWorkflowSolutions: &app.SolutionAdministration{Gateway: &solutionRouteGateway{}}, AIWorkflowPublications: &app.PublicationAdministration{}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(true))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	for _, kind := range []string{"solution", "publication"} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", "/internal/v2/interpretation/ai-workflow/"+kind+"s/"+publicationRouteID+"/flow?organization_id=99", nil))
		if w.Code != 200 || g.scope != (app.DraftScope{OrganizationID: 12, OperatorUserID: 34}) || g.kind != kind {
			t.Fatal(w.Code, g.scope, g.kind)
		}
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", solutionBase+"/"+publicationRouteID, nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	calls := g.calls
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", solutionBase+"/invalid/flow", nil))
	if w.Code != 400 || g.calls != calls {
		t.Fatal(w.Code, g.calls)
	}
	if _, err := (&app.FlowAdministration{Gateway: g}).Read(context.Background(), app.DraftScope{OrganizationID: 12, OperatorUserID: 34}, "solution", publicationRouteID); err != app.ErrGovernanceDenied {
		t.Fatal(err)
	}
}
