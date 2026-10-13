package handler

import (
	"encoding/json"
	"errors"
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

func TestAIWorkflowCatalogAndPublicationKeepErrorPriorityAndResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, handler := range []struct {
		name    string
		failure func(*gin.Context, error)
	}{
		{"catalog", NewAIWorkflowCatalogHandler(nil).failure},
		{"publication", NewAIWorkflowPublicationHandler(nil).failure},
	} {
		for _, test := range []struct {
			name                 string
			err                  error
			code, http           int
			catalog, publication string
		}{
			{"invalid", fmt.Errorf("wrapped: %w", app.ErrInvalid), code.ErrInvalidArgument, 400, "Invalid AI asset query", "Invalid AI publication operation"},
			{"invalid_rpc", status.Error(codes.InvalidArgument, "private"), code.ErrInvalidArgument, 400, "Invalid AI asset query", "Invalid AI publication operation"},
			{"denied", app.ErrGovernanceDenied, code.ErrPermissionDenied, 403, "AI asset audit permission required", "AI publication governance permission required"},
			{"denied_rpc", status.Error(codes.PermissionDenied, "private"), code.ErrPermissionDenied, 403, "AI asset audit permission required", "AI publication governance permission required"},
			{"missing", app.ErrNotFound, code.ErrPageNotFound, 404, "AI asset unavailable", "AI publication or command receipt unavailable"},
			{"missing_rpc", status.Error(codes.NotFound, "private"), code.ErrPageNotFound, 404, "AI asset unavailable", "AI publication or command receipt unavailable"},
			{"conflict", app.ErrConflict, code.ErrConflict, 409, "AI asset response differs from confirmed query", "AI publication version or confirmed state changed"},
			{"conflict_rpc", status.Error(codes.Aborted, "private"), code.ErrConflict, 409, "AI asset response differs from confirmed query", "AI publication version or confirmed state changed"},
			{"disabled", app.ErrManagementUnavailable, code.ErrUnsupportedOperation, 501, "Operation is not supported", "Operation is not supported"},
			{"unknown", status.Error(codes.Unavailable, "private"), code.ErrUnknown, 500, "Internal server error", "Internal server error"},
			{"capacity_stays_unknown", status.Error(codes.ResourceExhausted, "Evaluation capacity exhausted"), code.ErrUnknown, 500, "Internal server error", "Internal server error"},
			{"invalid_before_conflict", errors.Join(app.ErrConflict, app.ErrInvalid), code.ErrInvalidArgument, 400, "Invalid AI asset query", "Invalid AI publication operation"},
			{"denied_before_disabled", errors.Join(app.ErrManagementUnavailable, app.ErrGovernanceDenied), code.ErrPermissionDenied, 403, "AI asset audit permission required", "AI publication governance permission required"},
			{"missing_before_conflict", errors.Join(app.ErrConflict, app.ErrNotFound), code.ErrPageNotFound, 404, "AI asset unavailable", "AI publication or command receipt unavailable"},
			{"conflict_before_disabled", errors.Join(app.ErrManagementUnavailable, app.ErrConflict), code.ErrConflict, 409, "AI asset response differs from confirmed query", "AI publication version or confirmed state changed"},
		} {
			t.Run(handler.name+"/"+test.name, func(t *testing.T) {
				response := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(response)
				ctx.Request = httptest.NewRequest("GET", "/", nil)
				handler.failure(ctx, test.err)
				var body core.Response
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				message := test.catalog
				if handler.name == "publication" {
					message = test.publication
				}
				if response.Code != test.http || body.Code != test.code || body.Message != message {
					t.Fatalf("response=%d/%d/%q, want=%d/%d/%q", response.Code, body.Code, body.Message, test.http, test.code, message)
				}
			})
		}
	}
}
