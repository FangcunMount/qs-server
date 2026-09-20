package rest

import (
	"context"
	"encoding/json"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http/httptest"
	"strings"
	"testing"
)

type solutionRouteGateway struct {
	calls int
	scope app.DraftScope
	err   error
}

func (g *solutionRouteGateway) ReadSolution(_ context.Context, s app.DraftScope, _, _, _ string) (json.RawMessage, error) {
	g.calls++
	g.scope = s
	return json.RawMessage(`{"ok":true}`), g.err
}
func (g *solutionRouteGateway) WriteSolution(_ context.Context, s app.DraftScope, _, _ string, _ json.RawMessage) (json.RawMessage, error) {
	g.calls++
	g.scope = s
	return json.RawMessage(`{"ok":true}`), g.err
}
func solutionRouter(g *solutionRouteGateway, admin bool) *gin.Engine {
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowSolutions: &app.SolutionAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}

const solutionBase = "/internal/v2/interpretation/ai-workflow/solutions"

func TestSolutionRoutesTrustedScopeAndWriteAuthorization(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &solutionRouteGateway{}
		e := solutionRouter(g, admin)
		for _, op := range []string{"create", "save", "prepare"} {
			body := `{"command_id":"` + publicationRouteID + `","reason":"修改方案","expected_revision":1}`
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("POST", solutionBase+"/"+publicationRouteID+"/"+op, strings.NewReader(body)))
			if admin && (w.Code != 200 || g.scope != (app.DraftScope{OrganizationID: 12, OperatorUserID: 34})) {
				t.Fatal(w.Code, w.Body.String(), g.scope)
			}
			if !admin && (w.Code != 403 || g.calls != 0) {
				t.Fatal("unauthorized write", w.Code, g.calls)
			}
		}
		for _, path := range []string{"", "/models", "/" + publicationRouteID, "/commands/" + publicationRouteID} {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", solutionBase+path, nil))
			if w.Code != 200 {
				t.Fatal(path, w.Code, w.Body.String())
			}
		}
	}
}
func TestSolutionRoutesInvalidInputAndOutcomeUnknown(t *testing.T) {
	g := &solutionRouteGateway{}
	e := solutionRouter(g, true)
	for _, body := range []string{`{}`, `null`, `{"command_id":"` + publicationRouteID + `","reason":"x","expected_revision":0}`, strings.Repeat("x", 256*1024+1)} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("POST", solutionBase+"/"+publicationRouteID+"/save", strings.NewReader(body)))
		if w.Code != 400 || g.calls != 0 {
			t.Fatal(w.Code, g.calls)
		}
	}
	g.err = status.Error(codes.Unavailable, "private endpoint secret")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", solutionBase+"/models", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "private endpoint") {
		t.Fatal(w.Code, w.Body.String())
	}
}
