package rest

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFreezeRouteRequiresExplicitRevisionAndSanitizesUnknownOutcome(t *testing.T) {
	for _, value := range []any{nil, 0, -1, "1"} {
		g := &draftRouteGateway{}
		e := draftRouter(g, true)
		body := draftBody()
		body["expected_revision"] = value
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", draftRouteBase+"/"+publicationRouteID+"/freeze", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		if w.Code != 400 || g.calls != 0 {
			t.Fatal(value, w.Code, g.calls)
		}
	}
	for _, tc := range []struct {
		rpc  codes.Code
		http int
	}{{codes.Unavailable, 500}, {codes.DeadlineExceeded, 500}, {codes.Aborted, 409}, {codes.InvalidArgument, 400}, {codes.NotFound, 404}, {codes.PermissionDenied, 403}} {
		g := &draftRouteGateway{err: status.Error(tc.rpc, "private secret endpoint")}
		e := draftRouter(g, true)
		raw, _ := json.Marshal(draftBody())
		r := httptest.NewRequest("POST", draftRouteBase+"/"+publicationRouteID+"/freeze", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		if w.Code != tc.http || g.calls != 1 || strings.Contains(w.Body.String(), "private secret") {
			t.Fatal(w.Code, g.calls, w.Body.String())
		}
	}
}

func TestDraftWritesRejectMalformedJSONBeforeRPC(t *testing.T) {
	for _, suffix := range []string{"/create", "/revisions", "/freeze"} {
		for _, body := range []string{`{"command_id":17}`, `{"expected_revision":"1"}`, `{"reason":`} {
			g := &draftRouteGateway{}
			e := draftRouter(g, true)
			r := httptest.NewRequest("POST", draftRouteBase+"/"+publicationRouteID+suffix, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			if w.Code != 400 || g.calls != 0 {
				t.Fatal(suffix, body, w.Code, g.calls)
			}
		}
	}
}
