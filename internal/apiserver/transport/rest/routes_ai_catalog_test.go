package rest

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type catalogRouteGateway struct {
	calls int
	scope app.DraftScope
	query app.AssetCatalogQuery
	get   app.AssetCatalogGet
	err   error
}

func (g *catalogRouteGateway) ListAssets(_ context.Context, s app.DraftScope, q app.AssetCatalogQuery) (app.AssetCatalogPage, error) {
	g.calls++
	g.scope = s
	g.query = q
	return app.AssetCatalogPage{Items: []app.AssetCatalogItem{}}, g.err
}
func (g *catalogRouteGateway) GetAsset(_ context.Context, s app.DraftScope, q app.AssetCatalogGet) (app.AssetCatalogDetail, error) {
	g.calls++
	g.scope = s
	g.get = q
	return app.AssetCatalogDetail{}, g.err
}

const catalogBase = "/internal/v2/interpretation/ai-workflow/assets/"

func catalogRouter(g *catalogRouteGateway, admin bool) *gin.Engine {
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowAssets: &app.AssetCatalogAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}
func TestCatalogReadRoutesUseAuditAndTrustedScope(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &catalogRouteGateway{}
		e := catalogRouter(g, admin)
		for _, path := range []string{"profile?identity=中文%2F配置&organization_id=9&operator_user_id=10", "profile/detail?identity=中文%2F配置&version=v1%2Ftest"} {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", catalogBase+path, nil))
			if w.Code != 200 || g.scope.OrganizationID != 12 || g.scope.OperatorUserID != 34 {
				t.Fatal(w.Code, g)
			}
		}
		if g.calls != 2 || g.query.Limit != 20 || g.query.Identity != "中文/配置" || g.get.Version != "v1/test" {
			t.Fatal(g)
		}
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("POST", catalogBase+"profile", nil))
		if w.Code != 404 {
			t.Fatal(w.Code)
		}
	}
}
func TestCatalogDeniesMissingRevokedPermissionAndDisabledService(t *testing.T) {
	g := &catalogRouteGateway{}
	s := &app.AssetCatalogAdministration{Gateway: g}
	scope := app.DraftScope{OrganizationID: 7, OperatorUserID: 42}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if _, err := s.List(ctx, scope, app.AssetCatalogQuery{Kind: "profile"}); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := s.Get(ctx, scope, app.AssetCatalogGet{Kind: "profile", Identity: "x", Version: "v1"}); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
	}
	if g.calls != 0 {
		t.Fatal(g)
	}
	e := gin.New()
	h := handler.NewAIWorkflowCatalogHandler(s)
	e.GET("/:kind", h.List)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", "/profile", nil))
	if w.Code == 200 || g.calls != 0 {
		t.Fatal(w.Code, g)
	}
	ctx := authz.WithSnapshot(context.Background(), &authz.Snapshot{Permissions: []authz.Permission{{Resource: "qs:evaluation:collection:reports", Action: "audit", Mode: authz.AuthorizationModeUnconditional}}})
	var disabled *app.AssetCatalogAdministration
	if _, err := disabled.List(ctx, scope, app.AssetCatalogQuery{Kind: "profile"}); !errors.Is(err, app.ErrManagementUnavailable) {
		t.Fatal(err)
	}
}
func TestCatalogBadQueriesAndSafeErrors(t *testing.T) {
	for _, path := range []string{"profile?limit=0", "profile?limit=51", "profile?limit=abc", "receipt", "profile?cursor=bad", "profile/detail?identity=x", "profile?cursor=" + strings.Repeat("x", 17000)} {
		g := &catalogRouteGateway{}
		w := httptest.NewRecorder()
		catalogRouter(g, false).ServeHTTP(w, httptest.NewRequest("GET", catalogBase+path, nil))
		if w.Code != 400 || g.calls != 0 {
			t.Fatal(path[:min(40, len(path))], w.Code, g.calls)
		}
	}
	for _, tc := range []struct {
		rpc  codes.Code
		http int
	}{{codes.Unavailable, 500}, {codes.DeadlineExceeded, 500}, {codes.InvalidArgument, 400}, {codes.NotFound, 404}, {codes.PermissionDenied, 403}, {codes.Aborted, 409}} {
		g := &catalogRouteGateway{err: status.Error(tc.rpc, "private endpoint")}
		w := httptest.NewRecorder()
		catalogRouter(g, false).ServeHTTP(w, httptest.NewRequest("GET", catalogBase+"profile", nil))
		if w.Code != tc.http || g.calls != 1 || strings.Contains(w.Body.String(), "private endpoint") {
			t.Fatal(w.Code, g.calls, w.Body.String())
		}
	}
}
