package rest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (g *publicationRouteGateway) ListPublicationHistory(_ context.Context, s app.PublicationScope, q app.PublicationHistoryQuery) (app.PublicationHistoryPage, error) {
	g.reads++
	g.scope = s
	g.historyQuery = q
	return app.PublicationHistoryPage{SchemaVersion: "qs-ai-publication-history/v1", Selector: q.Selector, Entries: []app.PublicationHistoryEntry{}}, g.err
}
func (g *publicationRouteGateway) GetPublicationHistory(_ context.Context, s app.PublicationScope, q app.PublicationSelector, version int64) (app.PublicationReceipt, error) {
	g.reads++
	g.scope = s
	g.selector = q
	g.historyVersion = version
	return app.PublicationReceipt{Actor: "user:99", Current: app.PublicationState{Selector: q, Version: version}}, g.err
}

const historyRouteQuery = "?audience=participant&model_kind=scale&decision_kind=score_range"

func TestPublicationHistoryRoutesUseAuditAndProtectedScope(t *testing.T) {
	g := &publicationRouteGateway{}
	e := publicationRouter(g, false)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+"/history"+historyRouteQuery+"&organization_id=999&operator_user_id=999", nil))
	if w.Code != 200 || g.reads != 1 || g.scope != (app.PublicationScope{OrganizationID: 12, OperatorUserID: 34}) || g.historyQuery.Limit != 20 || g.historyQuery.BeforeVersion != 0 {
		t.Fatal(w.Code, w.Body.String(), g)
	}
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+"/history"+historyRouteQuery+"&limit=2&before_version=8&model_code=scale-a&model_version=v2", nil))
	if w.Code != 200 || g.historyQuery.Limit != 2 || g.historyQuery.BeforeVersion != 8 || g.historyQuery.Selector.ModelVersion == nil || *g.historyQuery.Selector.ModelVersion != "v2" {
		t.Fatal(w.Code, g)
	}
	w = httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+"/history/3"+historyRouteQuery, nil))
	if w.Code != 200 || g.historyVersion != 3 || !strings.Contains(w.Body.String(), "user:99") || g.writes != 0 {
		t.Fatal(w.Code, w.Body.String(), g)
	}
}

func TestPublicationHistoryInvalidQueryDoesNotReachAI(t *testing.T) {
	g := &publicationRouteGateway{}
	e := publicationRouter(g, false)
	for _, suffix := range []string{
		"/history", "/history" + historyRouteQuery + "&limit=", "/history" + historyRouteQuery + "&limit=0",
		"/history" + historyRouteQuery + "&limit=21", "/history" + historyRouteQuery + "&limit=2147483648",
		"/history" + historyRouteQuery + "&before_version=-1", "/history" + historyRouteQuery + "&before_version=9223372036854775808",
		"/history" + historyRouteQuery + "&model_code=", "/history" + historyRouteQuery + "&model_version=v1",
		"/history" + historyRouteQuery + "&limit=1&limit=2", "/history" + historyRouteQuery + "&audience=participant",
		"/history/0" + historyRouteQuery, "/history/-1" + historyRouteQuery, "/history/nope" + historyRouteQuery,
		"/history/1" + historyRouteQuery + "&limit=1", "/history/1" + historyRouteQuery + "&before_version=2",
	} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+suffix, nil))
		if w.Code != 400 {
			t.Fatal(suffix, w.Code, w.Body.String())
		}
	}
	if g.reads != 0 || g.writes != 0 {
		t.Fatal("invalid request forwarded")
	}
}

func TestPublicationHistoryErrorsAreSanitizedAndRoutesDisabled(t *testing.T) {
	for _, suffix := range []string{"/history", "/history/1"} {
		g := &publicationRouteGateway{err: status.Error(codes.Unavailable, "secret endpoint")}
		e := publicationRouter(g, false)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+suffix+historyRouteQuery, nil))
		if w.Code != 500 || g.reads != 1 || strings.Contains(w.Body.String(), "secret endpoint") {
			t.Fatal(w.Code, w.Body.String(), g)
		}
		r := newRouterWithBudgets(Deps{})
		disabled := gin.New()
		r.registerInterpretationInternalV2Routes(disabled.Group("/internal/v2"))
		w = httptest.NewRecorder()
		disabled.ServeHTTP(w, httptest.NewRequest("GET", publicationRouteBase+suffix+historyRouteQuery, nil))
		if w.Code != 404 {
			t.Fatal("disabled management exposes history", w.Code)
		}
	}
}
