package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBaseHandlerErrorResponseUsesBusinessMessageForClientErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/test", nil)

	NewBaseHandler().Error(ctx, cberrors.WithCode(errorCode.ErrInvalidArgument, "量表必须包含一个总分因子"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Message != "量表必须包含一个总分因子" {
		t.Fatalf("message = %q, want business message", body.Message)
	}
}

func TestBaseHandlerErrorResponseUsesCauseForWrappedClientErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/test", nil)

	err := cberrors.WrapC(errors.New("factors: 量表必须包含一个总分因子"), errorCode.ErrInvalidArgument, "执行量表生命周期操作失败")
	NewBaseHandler().Error(ctx, err)

	var body Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Message != "factors: 量表必须包含一个总分因子" {
		t.Fatalf("message = %q, want wrapped cause", body.Message)
	}
}

func TestBaseHandlerErrorResponseKeepsGenericMessageForServerErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/test", nil)

	NewBaseHandler().Error(ctx, cberrors.WithCode(errorCode.ErrDatabase, "database password leaked"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	var body Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Message == "database password leaked" {
		t.Fatalf("server error leaked detail message")
	}
}

func TestBaseHandlerErrorResponseMapsStatisticsOverloadToServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v2/statistics/overview", nil)

	NewBaseHandler().Error(ctx, cberrors.WithCode(errorCode.ErrStatisticsOverloaded, "statistics_read_overloaded"))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	var body Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != errorCode.ErrStatisticsOverloaded {
		t.Fatalf("code = %d, want %d", body.Code, errorCode.ErrStatisticsOverloaded)
	}
	if body.Message != "Statistics temporarily overloaded" {
		t.Fatalf("message = %q", body.Message)
	}
}

func TestBaseHandlerErrorResponseTreatsContextCanceledAsClientClosedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	NewBaseHandler().Error(ctx, cberrors.Wrap(context.Canceled, "failed to count accessible testees"))

	if recorder.Code != httpStatusClientClosedRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, httpStatusClientClosedRequest)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty response for canceled request", recorder.Body.String())
	}
}

func TestBaseHandlerErrorResponseTreatsGRPCCanceledAsClientClosedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	NewBaseHandler().Error(ctx, cberrors.Wrap(status.Error(codes.Canceled, "context canceled"), "get scale failed"))

	if recorder.Code != httpStatusClientClosedRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, httpStatusClientClosedRequest)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty response for canceled gRPC request", recorder.Body.String())
	}
}
