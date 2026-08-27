package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/collection-server/application/aiexplanation"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type aiExplanationServiceStub struct {
	capability func(context.Context, uint64, uint64, app.Request) (*app.Response, error)
	request    func(context.Context, uint64, uint64, app.Request) (*app.Response, error)
	get        func(context.Context, uint64, uint64, string) (*app.Response, error)
	export     func(context.Context, uint64, int, string) (*app.ExportPage, error)
}

func (s aiExplanationServiceStub) Capability(ctx context.Context, testeeID, assessmentID uint64, request app.Request) (*app.Response, error) {
	return s.capability(ctx, testeeID, assessmentID, request)
}

func (s aiExplanationServiceStub) Request(ctx context.Context, testeeID, assessmentID uint64, request app.Request) (*app.Response, error) {
	return s.request(ctx, testeeID, assessmentID, request)
}

func (s aiExplanationServiceStub) Get(ctx context.Context, testeeID, assessmentID uint64, generationID string) (*app.Response, error) {
	return s.get(ctx, testeeID, assessmentID, generationID)
}

func (s aiExplanationServiceStub) Export(ctx context.Context, testeeID uint64, pageSize int, cursor string) (*app.ExportPage, error) {
	return s.export(ctx, testeeID, pageSize, cursor)
}

func TestAIExplanationHandlerReturnsAcceptedForPendingGeneration(t *testing.T) {
	handler := NewAIExplanationHandler(aiExplanationServiceStub{request: func(_ context.Context, testeeID, assessmentID uint64, request app.Request) (*app.Response, error) {
		if testeeID != 7 || assessmentID != 42 || request.Locale != "zh-CN" || len(request.FocusAreas) != 1 {
			t.Fatalf("request = testee:%d assessment:%d payload:%#v", testeeID, assessmentID, request)
		}
		return &app.Response{Status: "pending", GenerationID: "9001", SourceState: "current"}, nil
	}})
	recorder, c := newAIExplanationTestContext(http.MethodPost, "/api/v1/assessments/42/ai-explanations?testee_id=7", "{\"locale\":\"zh-CN\",\"focus_areas\":[\"work\"]}")
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "42"})

	handler.Request(c)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data app.Response `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Status != "pending" || response.Data.GenerationID != "9001" {
		t.Fatalf("response=%#v", response.Data)
	}
}

func TestAIExplanationHandlerReturnsDisabledCapabilityAsNormalState(t *testing.T) {
	handler := NewAIExplanationHandler(aiExplanationServiceStub{capability: func(context.Context, uint64, uint64, app.Request) (*app.Response, error) {
		return &app.Response{Status: "not_applicable", ReasonCode: "feature_disabled", SourceState: "unavailable"}, nil
	}})
	recorder, c := newAIExplanationTestContext(http.MethodGet, "/api/v1/assessments/42/ai-explanation/capability?testee_id=7", "")
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "42"})

	handler.Capability(c)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "\"reason_code\":\"feature_disabled\"") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAIExplanationHandlerMapsApplicationValidationToBadRequest(t *testing.T) {
	handler := NewAIExplanationHandler(aiExplanationServiceStub{capability: func(context.Context, uint64, uint64, app.Request) (*app.Response, error) {
		return nil, app.ErrInvalidRequest
	}})
	recorder, c := newAIExplanationTestContext(http.MethodGet, "/api/v1/assessments/42/ai-explanation/capability?testee_id=7", "")
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "42"})

	handler.Capability(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAIExplanationHandlerMapsCapacityWithoutLeakingBackendError(t *testing.T) {
	handler := NewAIExplanationHandler(aiExplanationServiceStub{request: func(context.Context, uint64, uint64, app.Request) (*app.Response, error) {
		return nil, status.Error(codes.ResourceExhausted, "secret provider detail")
	}})
	recorder, c := newAIExplanationTestContext(http.MethodPost, "/api/v1/assessments/42/ai-explanations?testee_id=7", `{"locale":"zh-CN"}`)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "42"})

	handler.Request(c)

	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" || strings.Contains(recorder.Body.String(), "secret provider detail") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAIExplanationHandlerExportsParticipantProjection(t *testing.T) {
	handler := NewAIExplanationHandler(aiExplanationServiceStub{export: func(_ context.Context, testeeID uint64, pageSize int, cursor string) (*app.ExportPage, error) {
		if testeeID != 7 || pageSize != 25 || cursor != "page-1" {
			t.Fatalf("export request = testee:%d size:%d cursor:%q", testeeID, pageSize, cursor)
		}
		return &app.ExportPage{SchemaVersion: "ai-explanation-subject-export/v1", OrgID: 9, TesteeID: 7, Items: []app.ExportItem{}}, nil
	}})
	recorder, c := newAIExplanationTestContext(http.MethodGet, "/api/v1/ai-explanations/export?testee_id=7&page_size=25&cursor=page-1", "")

	handler.Export(c)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ai-explanation-subject-export/v1") || !strings.Contains(recorder.Body.String(), "\"items\":[]") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAIExplanationHandlerExportFailsClosedWhenUnavailable(t *testing.T) {
	handler := NewAIExplanationHandler(aiExplanationServiceStub{export: func(context.Context, uint64, int, string) (*app.ExportPage, error) {
		return nil, app.ErrUnavailable
	}})
	recorder, c := newAIExplanationTestContext(http.MethodGet, "/api/v1/ai-explanations/export?testee_id=7", "")

	handler.Export(c)

	if recorder.Code != http.StatusServiceUnavailable || strings.Contains(recorder.Body.String(), "items") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSecondsUntilNextUTCDate(t *testing.T) {
	at := time.Date(2026, 8, 27, 23, 59, 59, 500_000_000, time.UTC)
	if got := secondsUntilNextUTCDate(at); got != 1 {
		t.Fatalf("retry after = %d, want 1", got)
	}
	if got := secondsUntilNextUTCDate(time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)); got != 12*60*60 {
		t.Fatalf("retry after = %d, want %d", got, 12*60*60)
	}
}

func newAIExplanationTestContext(method, target, body string) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return recorder, c
}
