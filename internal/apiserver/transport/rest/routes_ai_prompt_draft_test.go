package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type draftRouteGateway struct {
	calls    int
	scope    app.DraftScope
	revision *int64
	err      error
}

func (g *draftRouteGateway) CreatePromptDraft(_ context.Context, s app.DraftScope, id string, _ app.CreatePromptDraft) (app.PromptDraftState, error) {
	g.calls++
	g.scope = s
	return app.PromptDraftState{DraftID: id}, g.err
}
func (g *draftRouteGateway) RevisePromptDraft(_ context.Context, s app.DraftScope, id string, _ app.RevisePromptDraft) (app.PromptDraftState, error) {
	g.calls++
	g.scope = s
	return app.PromptDraftState{DraftID: id}, g.err
}
func (g *draftRouteGateway) GetPromptDraft(_ context.Context, s app.DraftScope, id string, r *int64) (app.PromptDraftState, error) {
	g.calls++
	g.scope = s
	g.revision = r
	return app.PromptDraftState{DraftID: id}, g.err
}
func (g *draftRouteGateway) GetPromptDraftReceipt(_ context.Context, s app.DraftScope, id string) (app.PromptDraftState, error) {
	g.calls++
	g.scope = s
	return app.PromptDraftState{CommandID: id}, g.err
}

const draftRouteBase = "/internal/v2/interpretation/ai-workflow/prompt-drafts"

func draftRouter(g *draftRouteGateway, admin bool) *gin.Engine {
	r := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowPromptDrafts: &app.PromptDraftAdministration{Gateway: g}}})
	e := gin.New()
	e.Use(aiRouteSnapshotMiddleware(admin))
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	return e
}
func draftBody() map[string]any {
	return map[string]any{"organization_id": 999, "operator_user_id": 999, "command_id": publicationRouteID, "reason": "草稿修订", "source": map[string]string{"identity": "p", "version": "v1", "fingerprint": "sha256:" + strings.Repeat("a", 64), "content_sha256": strings.Repeat("b", 64)}, "template_id": "p", "target_version": "v2", "expected_revision": 1, "content": map[string]any{"system_message": "未完成 {{", "task_template": "", "data_preamble": "", "allowed_placeholders": []string{}}}
}
func TestDraftRoutesAuthorizationScopeAndHistoricalRead(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &draftRouteGateway{}
		e := draftRouter(g, admin)
		for _, suffix := range []string{"/create", "/revisions", "/freeze"} {
			raw, _ := json.Marshal(draftBody())
			req := httptest.NewRequest("POST", draftRouteBase+"/"+publicationRouteID+suffix, strings.NewReader(string(raw)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			e.ServeHTTP(w, req)
			if admin && (w.Code != 200 || g.scope != (app.DraftScope{OrganizationID: 12, OperatorUserID: 34})) {
				t.Fatal(w.Code, w.Body.String(), g.scope)
			}
			if !admin && (w.Code != 403 || g.calls != 0) {
				t.Fatal("audit write reached AI", w.Code, g.calls)
			}
		}
		for _, suffix := range []string{"/" + publicationRouteID, "/" + publicationRouteID + "?revision=1", "/commands/" + publicationRouteID, "/freeze-commands/" + publicationRouteID} {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", draftRouteBase+suffix, nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		if g.revision == nil || *g.revision != 1 {
			t.Fatal("historical revision lost")
		}
	}
}
func TestDraftRoutesRejectInvalidCommandsAndQueries(t *testing.T) {
	g := &draftRouteGateway{}
	e := draftRouter(g, true)
	for _, mutate := range []func(map[string]any){func(b map[string]any) { delete(b, "content") }, func(b map[string]any) { b["content"] = nil }, func(b map[string]any) { b["expected_revision"] = 0 }, func(b map[string]any) { b["command_id"] = "invalid" }, func(b map[string]any) { b["reason"] = "" }} {
		body := draftBody()
		mutate(body)
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", draftRouteBase+"/"+publicationRouteID+"/revisions", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, query := range []string{"?revision=", "?revision=0", "?revision=-1", "?revision=x", "?revision=1&revision=2", "?revision=9223372036854775808"} {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", draftRouteBase+"/"+publicationRouteID+query, nil))
		if w.Code != 400 {
			t.Fatal(query, w.Code)
		}
	}
	if g.calls != 0 {
		t.Fatal("invalid request forwarded")
	}
}
func TestDraftRoutesErrorsDoNotRetryOrLeakAndDisabledRoutesAbsent(t *testing.T) {
	for _, tc := range []struct {
		rpc  codes.Code
		http int
	}{{codes.Unavailable, 500}, {codes.DeadlineExceeded, 500}, {codes.Aborted, 409}, {codes.NotFound, 404}, {codes.PermissionDenied, 403}, {codes.InvalidArgument, 400}} {
		g := &draftRouteGateway{err: status.Error(tc.rpc, "private secret endpoint")}
		e := draftRouter(g, true)
		raw, _ := json.Marshal(draftBody())
		req := httptest.NewRequest("POST", draftRouteBase+"/"+publicationRouteID+"/create", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		if w.Code != tc.http || g.calls != 1 || strings.Contains(w.Body.String(), "private secret") {
			t.Fatal(w.Code, g.calls, w.Body.String())
		}
	}
	r := newRouterWithBudgets(Deps{})
	e := gin.New()
	r.registerInterpretationInternalV2Routes(e.Group("/internal/v2"))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest("GET", draftRouteBase+"/"+publicationRouteID, nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestDraftServiceRechecksPermissionBeforeEveryCall(t *testing.T) {
	g := &draftRouteGateway{}
	s := &app.PromptDraftAdministration{Gateway: g}
	scope := app.DraftScope{OrganizationID: 12, OperatorUserID: 34}
	for _, ctx := range []context.Context{context.Background(), authz.WithSnapshot(context.Background(), &authz.Snapshot{})} {
		if _, err := s.Create(ctx, scope, publicationRouteID, app.CreatePromptDraft{}); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := s.Revise(ctx, scope, publicationRouteID, app.RevisePromptDraft{}); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := s.Get(ctx, scope, publicationRouteID, nil); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := s.Freeze(ctx, scope, publicationRouteID, app.FreezePromptDraft{}); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := s.GetFreezeReceipt(ctx, scope, publicationRouteID); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
		if _, err := s.GetReceipt(ctx, scope, publicationRouteID); !errors.Is(err, app.ErrGovernanceDenied) {
			t.Fatal(err)
		}
	}
	if g.calls != 0 {
		t.Fatal("revoked request reached AI")
	}
}

func (g *draftRouteGateway) FreezePromptDraft(_ context.Context, s app.DraftScope, id string, c app.FreezePromptDraft) (app.FrozenPromptReceipt, error) {
	g.calls++
	g.scope = s
	return app.FrozenPromptReceipt{Scope: s, Command: app.FrozenPromptCommand{DraftID: id, CommandID: c.CommandID, ExpectedRevision: c.ExpectedRevision, Reason: c.Reason}}, g.err
}
func (g *draftRouteGateway) GetPromptFreezeReceipt(_ context.Context, s app.DraftScope, id string) (app.FrozenPromptReceipt, error) {
	g.calls++
	g.scope = s
	return app.FrozenPromptReceipt{Scope: s, Command: app.FrozenPromptCommand{CommandID: id}}, g.err
}
