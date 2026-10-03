package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	middleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type mqHTTPCommands struct {
	calls     int
	org, user int64
	id        string
	err       error
}

func (s *mqHTTPCommands) SubmitParticipantRetry(_ context.Context, scope app.DraftScope, _ string, command app.ParticipantRetry) error {
	s.calls++
	s.org, s.user, s.id = scope.OrganizationID, scope.OperatorUserID, command.CommandID
	return s.err
}
func (s *mqHTTPCommands) SubmitEvaluationStart(_ context.Context, scope app.EvaluationScope, id string, _ app.EvaluationStart) error {
	s.calls++
	s.org, s.user, s.id = scope.OrganizationID, scope.OperatorUserID, id
	return s.err
}
func (s *mqHTTPCommands) SubmitEvaluationCancel(_ context.Context, scope app.EvaluationScope, id string, _ app.EvaluationCancel) error {
	s.calls++
	s.org, s.user, s.id = scope.OrganizationID, scope.OperatorUserID, id
	return s.err
}

type mqParticipantGateway struct {
	app.ParticipantManagementGateway
}

func TestMQHTTP202MeansQSCommitAndUsesOriginalCommandIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := "00000000-0000-4000-8000-000000000005"
	for _, kind := range []string{"start", "cancel", "retry"} {
		for _, scenario := range []string{"submitted", "missing_id", "revoked", "storage_failed", "maintenance"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				commands := &mqHTTPCommands{}
				if scenario == "storage_failed" {
					commands.err = errors.New("secret driver detail")
				}
				if scenario == "maintenance" {
					commands.err = app.ErrRuntimeAdmissionClosed
				}
				gateway := &managementGateway{}
				evaluation := NewAIWorkflowManagementHandler(&app.EvaluationAdministration{Gateway: gateway, Messages: commands})
				participant := NewAIWorkflowParticipantHandler(&app.ParticipantAdministration{Gateway: &mqParticipantGateway{}, Messages: commands})
				commandID := id
				if scenario == "missing_id" {
					commandID = ""
				}
				body := `{"command_id":"` + commandID + `","organization_id":999,"operator_user_id":999,"expected_version":6,"reason":"确认操作","confirm":true,"discard":false,"expected_provider_invocations":1,"expected_run_id":"00000000-0000-4000-8000-000000000003"}`
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("POST", "/internal/v2/interpretation/ai-workflow/evaluations/run/"+kind, strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Params = gin.Params{{Key: "run_id", Value: "00000000-0000-4000-8000-000000000001"}, {Key: "session_id", Value: "00000000-0000-4000-8000-000000000002"}}
				c.Set(middleware.OrgIDKey, uint64(12))
				c.Set(middleware.UserIDKey, uint64(34))
				snapshot := &authz.Snapshot{EffectiveRoles: []string{"qs:admin"}, Permissions: []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}}
				if scenario == "revoked" {
					snapshot.Permissions = nil
				}
				c.Request = c.Request.WithContext(authz.WithSnapshot(c.Request.Context(), snapshot))
				switch kind {
				case "start":
					evaluation.Start(c)
				case "cancel":
					evaluation.Cancel(c)
				case "retry":
					participant.Retry(c)
				}
				expected, calls := 202, 1
				switch scenario {
				case "missing_id":
					expected, calls = 400, 0
				case "revoked":
					expected, calls = 403, 0
				case "storage_failed":
					expected = 500
				case "maintenance":
					expected = 429
				}
				if w.Code != expected || commands.calls != calls || gateway.calls != 0 {
					t.Fatalf("status=%d submit=%d grpc=%d", w.Code, commands.calls, gateway.calls)
				}
				if strings.Contains(w.Body.String(), "secret driver detail") {
					t.Fatal("driver details leaked")
				}
				if scenario == "maintenance" && (!strings.Contains(w.Body.String(), app.RuntimeAdmissionClosedReason) || strings.Contains(w.Body.String(), `"status":"submitted"`)) {
					t.Fatal("maintenance refusal confused with submitted operation", w.Body.String())
				}
				if scenario == "submitted" {
					var response struct {
						Code int
						Data map[string]string
					}
					if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					if response.Code != 0 || response.Data["operation_id"] != id || response.Data["command_id"] != id || response.Data["status"] != "submitted" || response.Data["status_url"] != "/internal/v2/interpretation/ai-workflow/operations/"+id || commands.org != 12 || commands.user != 34 {
						t.Fatal("submission, identity or protected scope lost", response, commands)
					}
				}
			})
		}
	}
}
