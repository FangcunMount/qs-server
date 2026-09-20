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

type quotaRouteGateway struct {
	calls int
	scope app.DraftScope
	err   error
}

func (g *quotaRouteGateway) ReadQuota(_ context.Context, s app.DraftScope, _, _ string, _ int64) (json.RawMessage, error) {
	g.calls++
	g.scope = s
	return json.RawMessage(`{"ok":true}`), g.err
}
func (g *quotaRouteGateway) WriteQuota(_ context.Context, s app.DraftScope, _ string, _ json.RawMessage) (json.RawMessage, error) {
	g.calls++
	g.scope = s
	return json.RawMessage(`{"ok":true}`), g.err
}
func quotaRouter(g *quotaRouteGateway, admin bool) *gin.Engine {
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowQuotas: &app.QuotaAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}

const quotaBase = "/internal/v2/interpretation/ai-workflow/quotas"

func TestQuotaRoutesTrustedScopeAndWriteAuthorization(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &quotaRouteGateway{}
		e := quotaRouter(g, admin)
		for _, op := range []string{"update", "rollback"} {
			body := `{"command_id":"` + publicationRouteID + `","reason":"修改方案","expected_revision":1}`
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("POST", quotaBase+"/"+op, strings.NewReader(body)))
			if admin && (w.Code != 200 || g.scope != (app.DraftScope{OrganizationID: 12, OperatorUserID: 34})) {
				t.Fatal(w.Code, w.Body.String(), g.scope)
			}
			if !admin && (w.Code != 403 || g.calls != 0) {
				t.Fatal("unauthorized write", w.Code, g.calls)
			}
		}
		for _, path := range []string{"", "/history", "/commands/" + publicationRouteID} {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", quotaBase+path, nil))
			if w.Code != 200 {
				t.Fatal(path, w.Code, w.Body.String())
			}
		}
	}
}
func TestQuotaRoutesInvalidInputAndOutcomeUnknown(t *testing.T) {
	g := &quotaRouteGateway{}
	e := quotaRouter(g, true)
	for _, body := range []string{`{}`, `null`, `{"command_id":"` + publicationRouteID + `","reason":"x","expected_revision":-1}`, strings.Repeat("x", 15*1024+1)} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("POST", quotaBase+"/update", strings.NewReader(body)))
		if w.Code != 400 || g.calls != 0 {
			t.Fatal(w.Code, g.calls)
		}
	}
	g.err = status.Error(codes.Unavailable, "private endpoint secret")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", quotaBase, nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "private endpoint") {
		t.Fatal(w.Code, w.Body.String())
	}
}
