package rest

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

func TestLifecycleReadScopeAndRevisionSelector(t *testing.T) {
	for _, admin := range []bool{false, true} {
		g := &draftRouteGateway{}
		e := draftRouter(g, admin)
		path := draftRouteBase + "/" + publicationRouteID + "/lifecycle"
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", path+"?organization_id=999&operator_user_id=999", nil))
		if w.Code != 200 || g.calls != 1 || g.scope != (app.DraftScope{OrganizationID: 12, OperatorUserID: 34}) {
			t.Fatal(w.Code, w.Body.String(), g)
		}
		for _, suffix := range []string{"?revision=1", "?revision=", "?revision=1&revision=2"} {
			w = httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", path+suffix, nil))
			if w.Code != 400 || g.calls != 1 {
				t.Fatal("historical lifecycle reached AI", w.Code, g.calls)
			}
		}
	}
	g := &draftRouteGateway{}
	s := &app.PromptDraftAdministration{Gateway: g}
	if _, err := s.GetLifecycle(context.Background(), app.DraftScope{OrganizationID: 12, OperatorUserID: 34}, publicationRouteID); !errors.Is(err, app.ErrGovernanceDenied) || g.calls != 0 {
		t.Fatal("unauthorized lifecycle reached AI", err)
	}
}
