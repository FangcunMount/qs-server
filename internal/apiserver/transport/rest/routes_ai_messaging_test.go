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

type mqRouteSubmitter struct {
	app.RuntimeCommandSubmitter
	calls                int
	evaluationScope      app.EvaluationScope
	participantScope     app.DraftScope
	commandID, sessionID string
	cancel               app.EvaluationCancel
	retry                app.ParticipantRetry
}

func (s *mqRouteSubmitter) SubmitEvaluationCancel(_ context.Context, scope app.EvaluationScope, id string, command app.EvaluationCancel) error {
	s.calls++
	s.evaluationScope, s.commandID, s.cancel = scope, id, command
	return nil
}

func (s *mqRouteSubmitter) SubmitParticipantRetry(_ context.Context, scope app.DraftScope, sessionID string, command app.ParticipantRetry) error {
	s.calls++
	s.participantScope, s.commandID, s.sessionID, s.retry = scope, command.CommandID, sessionID, command
	return nil
}

func assertMQRouteSubmitted(t *testing.T, w *httptest.ResponseRecorder, commandID string) {
	t.Helper()
	var response struct {
		Data struct {
			OperationID string `json:"operation_id"`
			CommandID   string `json:"command_id"`
			Status      string `json:"status"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.OperationID != commandID || response.Data.CommandID != commandID || response.Data.Status != "submitted" {
		t.Fatal("route lost original submitted identity", w.Body.String())
	}
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
