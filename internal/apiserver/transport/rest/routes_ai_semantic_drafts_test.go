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

type semanticDraftRouteGateway struct {
	calls int
	scope app.DraftScope
}

func (g *semanticDraftRouteGateway) ReadSemanticDraft(_ context.Context, s app.DraftScope, _, _ string, _ int64) (json.RawMessage, error) {
	g.calls++
	g.scope = s
	return json.RawMessage(`{"ok":true}`), nil
}
func (g *semanticDraftRouteGateway) WriteSemanticDraft(_ context.Context, s app.DraftScope, _ string, _ json.RawMessage) (json.RawMessage, error) {
	g.calls++
	g.scope = s
	return json.RawMessage(`{"ok":true}`), nil
}
func semanticDraftRouter(g *semanticDraftRouteGateway, admin bool) *gin.Engine {
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowSemanticDrafts: &app.SemanticDraftAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}

const semanticDraftBase = "/internal/v2/interpretation/ai-workflow/semantic-prompts/drafts"

func TestSemanticDraftRoutesRequireAdminForWritesAndTrustedScope(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &semanticDraftRouteGateway{}
		e := semanticDraftRouter(g, admin)
		for _, op := range []string{"create", "revise", "freeze"} {
			body := `{"draft_id":"` + publicationRouteID + `","command_id":"` + publicationRouteID + `","reason":"编辑裁判","expected_revision":1}`
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("POST", semanticDraftBase+"/"+publicationRouteID+"/"+op, strings.NewReader(body)))
			if admin && (w.Code != 200 || g.scope != (app.DraftScope{OrganizationID: 12, OperatorUserID: 34})) {
				t.Fatal(w.Code, w.Body.String(), g.scope)
			}
			if !admin && (w.Code != 403 || g.calls != 0) {
				t.Fatal("unauthorized write", w.Code, g.calls)
			}
		}
		for _, path := range []string{"/" + publicationRouteID, "/commands/" + publicationRouteID} {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", semanticDraftBase+path, nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("POST", semanticDraftBase+"/"+publicationRouteID+"/validate?revision=1", nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestSemanticDraftRejectsPathIdentityMismatchAndMissingCAS(t *testing.T) {
	g := &semanticDraftRouteGateway{}
	e := semanticDraftRouter(g, true)
	for _, body := range []string{`{}`, `null`, `{"draft_id":"00000000-0000-4000-8000-000000000002","command_id":"` + publicationRouteID + `","reason":"x","expected_revision":1}`, `{"draft_id":"` + publicationRouteID + `","command_id":"` + publicationRouteID + `","reason":"x"}`} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("POST", semanticDraftBase+"/"+publicationRouteID+"/revise", strings.NewReader(body)))
		if w.Code != 400 || g.calls != 0 {
			t.Fatal(w.Code, g.calls)
		}
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("POST", semanticDraftBase+"/"+publicationRouteID+"/validate", nil))
	if w.Code != 400 || g.calls != 0 {
		t.Fatal(w.Code, g.calls)
	}
}
