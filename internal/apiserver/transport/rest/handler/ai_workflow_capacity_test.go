package handler

import (
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http/httptest"
	"testing"
)

func TestCapacityDenialIsDistinctFromTransportOutcomeUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, item := range []struct {
		message string
		want    int
	}{
		{"Evaluation capacity exhausted", 429},
		{"grpc: received message larger than max", 500},
	} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("GET", "/", nil)
		NewAIWorkflowManagementHandler(nil).failure(ctx, status.Error(codes.ResourceExhausted, item.message))
		if w.Code != item.want {
			t.Fatal(item.message, w.Code, w.Body.String())
		}
	}
}
