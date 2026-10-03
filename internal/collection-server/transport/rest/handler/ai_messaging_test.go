package handler

import (
	"context"
	"encoding/json"
	app "github.com/FangcunMount/qs-server/internal/collection-server/application/aiexplanation"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"strings"
	"testing"
)

type mqWorkflowHandler struct {
	workflowHandlerStub
	operationCalls int
	denied         bool
}

func (s *mqWorkflowHandler) RequestWorkflow(_ context.Context, _, _ uint64, r app.WorkflowRequest) (*app.WorkflowAccepted, error) {
	return &app.WorkflowAccepted{RequestID: r.RequestID, Status: "submitted"}, nil
}
func (s *mqWorkflowHandler) GetWorkflowOperation(_ context.Context, testee, assessment uint64, request, id string) (*app.WorkflowOperation, error) {
	s.operationCalls++
	if s.denied {
		return nil, status.Error(codes.PermissionDenied, "revoked")
	}
	return &app.WorkflowOperation{OperationID: id, CommandID: id, Status: "submitted", TransportStatus: "awaiting_receipt"}, nil
}
func TestMQPublic202ReturnsOriginalIdentityAndReadOnlyStatusURL(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	s := &mqWorkflowHandler{}
	h := NewAIExplanationHandler(s)
	w, c := newAIExplanationTestContext(http.MethodPost, "/api/v1/assessments/42/ai-workflows?testee_id=7", `{"request_id":"`+id+`","report_id":"99"}`)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	h.RequestWorkflow(c)
	var result struct{ Data app.WorkflowAccepted }
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if w.Code != 202 || result.Data.Status != "submitted" || result.Data.OperationID != id || result.Data.CommandID != id || !strings.HasPrefix(result.Data.StatusURL, "/api/v1/interpretation/ai-workflow/operations/"+id+"?") {
		t.Fatal(w.Body.String())
	}
	if !strings.Contains(result.Data.StatusURL, "assessment_id=42") || !strings.Contains(result.Data.StatusURL, "testee_id=7") {
		t.Fatal("status URL lost permission context")
	}
	for _, scenario := range []string{"allowed", "revoked", "invalid_identity"} {
		t.Run(scenario, func(t *testing.T) {
			s.operationCalls = 0
			s.denied = scenario == "revoked"
			target := result.Data.StatusURL
			if scenario == "invalid_identity" {
				target = strings.Replace(target, "testee_id=7", "testee_id=07", 1)
			}
			w, c := newAIExplanationTestContext(http.MethodGet, target, "")
			c.Params = gin.Params{{Key: "command_id", Value: id}}
			h.GetOperation(c)
			want := 200
			if scenario == "revoked" {
				want = 403
			}
			if scenario == "invalid_identity" {
				want = 400
			}
			if w.Code != want || (scenario == "invalid_identity" && s.operationCalls != 0) {
				t.Fatalf("response=%d %s", w.Code, w.Body.String())
			}
		})
	}
}
