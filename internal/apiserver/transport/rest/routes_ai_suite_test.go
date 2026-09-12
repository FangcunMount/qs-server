package rest

import (
	"context"
	"encoding/json"
	"errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http/httptest"
	"strings"
	"testing"
)

type suiteRouteGateway struct {
	calls int
	scope app.DraftScope
	err   error
}

func (g *suiteRouteGateway) RegisterSuite(_ context.Context, s app.DraftScope, _ app.RegisterSuite) (app.SuiteRegistrationReceipt, error) {
	g.calls++
	g.scope = s
	return app.SuiteRegistrationReceipt{}, g.err
}
func (g *suiteRouteGateway) GetSuiteReceipt(_ context.Context, s app.DraftScope, _ string) (app.SuiteRegistrationReceipt, error) {
	g.calls++
	g.scope = s
	return app.SuiteRegistrationReceipt{}, g.err
}

const suiteBase = "/internal/v2/interpretation/ai-workflow/suites"

func suiteRouter(g *suiteRouteGateway, admin bool) *gin.Engine {
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowSuites: &app.SuiteAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}
func suiteBody() string {
	b := draftBody()
	b["profile"] = b["source"]
	b["prompt"] = b["source"]
	b["generation_route"] = b["source"]
	b["source"] = app.FrozenEvaluationRef{ID: "source", Version: "v1", Fingerprint: "sha256:" + strings.Repeat("a", 64)}
	b["suite_id"] = "new-suite"
	b["suite_version"] = "v1"
	raw, _ := json.Marshal(b)
	return string(raw)
}
func TestSuiteRoutesPreserveAuthorizationAndProtectedScope(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &suiteRouteGateway{}
		e := suiteRouter(g, admin)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", suiteBase+"/register", strings.NewReader(suiteBody()))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(w, req)
		if admin {
			if w.Code != 200 || g.calls != 1 || g.scope.OrganizationID != 12 || g.scope.OperatorUserID != 34 {
				t.Fatal(w.Code, g)
			}
		} else if w.Code != 403 || g.calls != 0 {
			t.Fatal(w.Code, g.calls)
		}
		g.calls = 0
		w = httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", suiteBase+"/commands/"+publicationRouteID, nil))
		if w.Code != 200 || g.calls != 1 {
			t.Fatal(w.Code, g.calls)
		}
	}
	g := &suiteRouteGateway{}
	service := &app.SuiteAdministration{Gateway: g}
	if _, err := service.Register(context.Background(), app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, app.RegisterSuite{}); !errors.Is(err, app.ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if _, err := service.GetReceipt(context.Background(), app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, publicationRouteID); !errors.Is(err, app.ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if g.calls != 0 {
		t.Fatal("called without authorization")
	}
}
func TestSuiteMalformedRequestsAndSanitizedOutcome(t *testing.T) {
	for _, body := range []string{`{"command_id":12}`, `{"suite_id":{}}`, strings.Repeat("x", 16*1024+1), `{}`} {
		g := &suiteRouteGateway{}
		e := suiteRouter(g, true)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", suiteBase+"/register", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(w, r)
		if w.Code != 400 || g.calls != 0 {
			t.Fatal(w.Code, g.calls)
		}
	}
	for _, tc := range []struct {
		rpc  codes.Code
		http int
	}{{codes.Unavailable, 500}, {codes.DeadlineExceeded, 500}, {codes.Aborted, 409}, {codes.InvalidArgument, 400}, {codes.NotFound, 404}, {codes.PermissionDenied, 403}} {
		g := &suiteRouteGateway{err: status.Error(tc.rpc, "private endpoint")}
		e := suiteRouter(g, true)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", suiteBase+"/register", strings.NewReader(suiteBody()))
		r.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(w, r)
		if w.Code != tc.http || g.calls != 1 || strings.Contains(w.Body.String(), "private endpoint") {
			t.Fatal(w.Code, g.calls, w.Body.String())
		}
	}
}
