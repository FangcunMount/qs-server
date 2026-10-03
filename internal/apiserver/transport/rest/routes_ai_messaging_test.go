package rest

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/gin-gonic/gin"
)

type mqOperationReader struct {
	calls int
	scope app.OperationScope
}

func (s *mqOperationReader) ReadOperation(_ context.Context, scope app.OperationScope, id string) (app.MessagingOperation, error) {
	s.calls++
	s.scope = scope
	return app.MessagingOperation{OperationID: id, CommandID: id, Status: "submitted", TransportStatus: "awaiting_receipt", ResourceID: "00000000-0000-4000-8000-000000000001"}, nil
}

func TestMQOperationRouteRequiresCurrentPermissionAndSeparatesPublication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := "00000000-0000-4000-8000-000000000005"
	for _, admin := range []bool{true, false} {
		reader := &mqOperationReader{}
		engine := gin.New()
		engine.Use(aiRouteSnapshotMiddleware(admin))
		router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIWorkflowOperations: &app.OperationAdministration{Store: reader}}})
		router.registerInterpretationInternalV2Routes(engine.Group("/internal/v2"))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest("GET", "/internal/v2/interpretation/ai-workflow/operations/"+id+"?organization_id=999&operator_user_id=999", nil))
		if !admin {
			if w.Code != 403 || reader.calls != 0 {
				t.Fatal("revoked access queried operation", w.Code, reader.calls)
			}
			continue
		}
		if w.Code != 200 || reader.calls != 1 || reader.scope.OrganizationID == "999" || reader.scope.SubjectID == "999" {
			t.Fatal("protected operation scope lost", w.Code, reader.scope)
		}
		var response struct{ Data app.MessagingOperation }
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Data.Status != "submitted" || response.Data.TransportStatus != "awaiting_receipt" || response.Data.Decision != "" {
			t.Fatal("published represented as accepted", response.Data)
		}
	}
}
