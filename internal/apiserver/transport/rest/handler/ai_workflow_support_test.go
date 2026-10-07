package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAIWorkflowFailuresKeepAssetSpecificResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, handler := range []struct {
		name, asset string
		failure     func(*gin.Context, error)
	}{
		{"profile", "Profile", NewAIWorkflowProfileHandler(nil).failure},
		{"suite", "Suite", NewAIWorkflowSuiteHandler(nil).failure},
		{"prompt", "draft", NewAIWorkflowPromptDraftHandler(nil).failure},
		{"semantic", "draft", NewAIWorkflowSemanticDraftHandler(nil).failure},
		{"solution", "draft", NewAIWorkflowSolutionHandler(nil).failure},
	} {
		for _, failure := range []struct {
			name       string
			err        error
			code, http int
			message    string
		}{
			{"invalid", fmt.Errorf("wrapped: %w", app.ErrInvalid), code.ErrInvalidArgument, 400, "Invalid AI %s operation"},
			{"invalid RPC", status.Error(codes.InvalidArgument, "private"), code.ErrInvalidArgument, 400, "Invalid AI %s operation"},
			{"denied", app.ErrGovernanceDenied, code.ErrPermissionDenied, 403, "AI %s governance permission required"},
			{"denied RPC", status.Error(codes.PermissionDenied, "private"), code.ErrPermissionDenied, 403, "AI %s governance permission required"},
			{"missing", app.ErrNotFound, code.ErrPageNotFound, 404, "AI %s revision or command receipt unavailable"},
			{"missing RPC", status.Error(codes.NotFound, "private"), code.ErrPageNotFound, 404, "AI %s revision or command receipt unavailable"},
			{"conflict", app.ErrConflict, code.ErrConflict, 409, "AI %s version or confirmed state changed"},
			{"conflict RPC", status.Error(codes.Aborted, "private"), code.ErrConflict, 409, "AI %s version or confirmed state changed"},
			{"disabled", app.ErrManagementUnavailable, code.ErrUnsupportedOperation, 501, "Operation is not supported"},
			{"unknown", status.Error(codes.Unavailable, "private"), code.ErrUnknown, 500, "Internal server error"},
		} {
			t.Run(handler.name+"/"+failure.name, func(t *testing.T) {
				response := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(response)
				ctx.Request = httptest.NewRequest("GET", "/", nil)
				handler.failure(ctx, failure.err)
				var body core.Response
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				message := failure.message
				if failure.http < 500 {
					message = fmt.Sprintf(message, handler.asset)
				}
				if response.Code != failure.http || body.Code != failure.code || body.Message != message {
					t.Fatalf("response = %d/%d/%q, want %d/%d/%q", response.Code, body.Code, body.Message, failure.http, failure.code, message)
				}
			})
		}
	}
}
