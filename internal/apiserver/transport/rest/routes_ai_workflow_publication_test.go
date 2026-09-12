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

type publicationRouteGateway struct {
	app.PublicationGateway
	writes, reads int
	scope         app.PublicationScope
	selector      app.PublicationSelector
	err           error
}

func (g *publicationRouteGateway) DisablePublication(_ context.Context, s app.PublicationScope, c app.PublicationCommand) (app.PublicationReceipt, error) {
	g.writes++
	g.scope = s
	return app.PublicationReceipt{CommandID: c.CommandID}, g.err
}
func (g *publicationRouteGateway) PublishConfiguration(ctx context.Context, s app.PublicationScope, c app.PublishConfiguration) (app.PublicationReceipt, error) {
	return g.DisablePublication(ctx, s, c.PublicationCommand)
}
func (g *publicationRouteGateway) RollbackPublication(ctx context.Context, s app.PublicationScope, c app.RollbackPublication) (app.PublicationReceipt, error) {
	return g.DisablePublication(ctx, s, c.PublicationCommand)
}
func (g *publicationRouteGateway) GetPublication(_ context.Context, s app.PublicationScope, q app.PublicationSelector) (app.PublicationState, error) {
	g.reads++
	g.scope = s
	g.selector = q
	return app.PublicationState{Selector: q}, g.err
}
func (g *publicationRouteGateway) GetPublicationReceipt(_ context.Context, s app.PublicationScope, id string) (app.PublicationReceipt, error) {
	g.reads++
	g.scope = s
	return app.PublicationReceipt{CommandID: id}, g.err
}

const publicationRouteBase = "/internal/v2/interpretation/ai-workflow/publications"
const publicationRouteID = "00000000-0000-4000-8000-000000000001"

func publicationRouteBody() map[string]any {
	return map[string]any{"organization_id": 999, "operator_user_id": 999, "command_id": publicationRouteID, "reason": "发布核对", "confirm": true, "expected": map[string]any{"selector": map[string]string{"audience": "participant", "model_kind": "scale", "decision_kind": "score_range"}, "version": 0, "active_publication_id": ""}, "run_id": publicationRouteID, "run_version": 9, "release_fingerprint": "sha256:" + strings.Repeat("a", 64), "target_publication_id": publicationRouteID}
}
func publicationRouter(g *publicationRouteGateway, admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowPublications: &app.PublicationAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}
func TestPublicationRoutesRequireAdminAndUseProtectedScope(t *testing.T) {
	for _, admin := range []bool{false, true} {
		for _, suffix := range []string{"/publish", "/rollback", "/disable"} {
			g := &publicationRouteGateway{}
			e := publicationRouter(g, admin)
			raw, _ := json.Marshal(publicationRouteBody())
			r := httptest.NewRequest("POST", publicationRouteBase+suffix, strings.NewReader(string(raw)))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			if admin {
				if w.Code != 200 || g.writes != 1 || g.scope != (app.PublicationScope{OrganizationID: 12, OperatorUserID: 34}) {
					t.Fatal(suffix, w.Code, g.scope)
				}
			} else if w.Code != 403 || g.writes != 0 {
				t.Fatal("audit-only write", suffix, w.Code)
			}
		}
	}
}
func TestPublicationReadsAndExplicitEmptySelector(t *testing.T) {
	g := &publicationRouteGateway{}
	e := publicationRouter(g, false)
	query := "?audience=participant&model_kind=scale&decision_kind=score_range"
	for _, suffix := range []string{query, query + "&model_code=scale-a&model_version=v2", "/commands/" + publicationRouteID} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+suffix, nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if g.reads != 3 || g.selector.ModelCode == nil || *g.selector.ModelCode != "scale-a" {
		t.Fatal("audit query not forwarded")
	}
	for _, suffix := range []string{query + "&model_code=", query + "&model_version=v1", "/commands/invalid"} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+suffix, nil))
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if g.reads != 3 {
		t.Fatal("invalid query forwarded")
	}
}
func TestPublicationTimeoutHasNoRetryAndSanitizedError(t *testing.T) {
	for _, test := range []struct {
		code codes.Code
		http int
	}{{codes.Unavailable, 500}, {codes.DeadlineExceeded, 500}, {codes.Aborted, 409}, {codes.NotFound, 404}, {codes.PermissionDenied, 403}, {codes.InvalidArgument, 400}} {
		g := &publicationRouteGateway{err: status.Error(test.code, "private secret endpoint")}
		e := publicationRouter(g, true)
		raw, _ := json.Marshal(publicationRouteBody())
		r := httptest.NewRequest("POST", publicationRouteBase+"/publish", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		if w.Code != test.http || g.writes != 1 || strings.Contains(w.Body.String(), "private secret") {
			t.Fatal(w.Code, w.Body.String(), g.writes)
		}
	}
}
func TestPublicationRoutesAbsentWhenManagementDisabled(t *testing.T) {
	r := newRouterWithBudgets(Deps{})
	e := gin.New()
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase, nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
