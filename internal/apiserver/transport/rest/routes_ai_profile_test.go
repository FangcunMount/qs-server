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

type profileRouteGateway struct {
	calls int
	scope app.DraftScope
	err   error
}

func (g *profileRouteGateway) RegisterProfile(_ context.Context, s app.DraftScope, _ app.RegisterProfile) (app.ProfileRegistrationReceipt, error) {
	g.calls++
	g.scope = s
	return app.ProfileRegistrationReceipt{}, g.err
}
func (g *profileRouteGateway) GetProfileReceipt(_ context.Context, s app.DraftScope, _ string) (app.ProfileRegistrationReceipt, error) {
	g.calls++
	g.scope = s
	return app.ProfileRegistrationReceipt{}, g.err
}

const profileBase = "/internal/v2/interpretation/ai-workflow/profiles"

func profileRouter(g *profileRouteGateway, admin bool) *gin.Engine {
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowProfiles: &app.ProfileAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}
func profileBody() string {
	b := draftBody()
	b["definition_json"] = `{"profile_id":"p","version":"v2"}`
	b["prompt"] = b["source"]
	b["generation_route"] = b["source"]
	raw, _ := json.Marshal(b)
	return string(raw)
}
func TestProfileRoutesPreserveAuthorizationAndProtectedScope(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &profileRouteGateway{}
		e := profileRouter(g, admin)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", profileBase+"/register", strings.NewReader(profileBody()))
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
		e.ServeHTTP(w, httptest.NewRequest("GET", profileBase+"/commands/"+publicationRouteID, nil))
		if w.Code != 200 || g.calls != 1 {
			t.Fatal(w.Code, g.calls)
		}
	}
	g := &profileRouteGateway{}
	service := &app.ProfileAdministration{Gateway: g}
	if _, err := service.Register(context.Background(), app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, app.RegisterProfile{}); !errors.Is(err, app.ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if _, err := service.GetReceipt(context.Background(), app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, publicationRouteID); !errors.Is(err, app.ErrGovernanceDenied) {
		t.Fatal(err)
	}
	if g.calls != 0 {
		t.Fatal("called without authorization")
	}
}
func TestProfileMalformedRequestsAndSanitizedOutcome(t *testing.T) {
	for _, body := range []string{`{"command_id":12}`, `{"definition_json":{}}`, strings.Repeat("x", 256*1024+1), `{}`} {
		g := &profileRouteGateway{}
		e := profileRouter(g, true)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", profileBase+"/register", strings.NewReader(body))
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
		g := &profileRouteGateway{err: status.Error(tc.rpc, "private endpoint")}
		e := profileRouter(g, true)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", profileBase+"/register", strings.NewReader(profileBody()))
		r.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(w, r)
		if w.Code != tc.http || g.calls != 1 || strings.Contains(w.Body.String(), "private endpoint") {
			t.Fatal(w.Code, g.calls, w.Body.String())
		}
	}
}
