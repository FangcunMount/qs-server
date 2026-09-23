package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/collection-server/application/aiexplanation"
	aiport "github.com/FangcunMount/qs-server/internal/collection-server/port/aiexplanation"
	"github.com/gin-gonic/gin"
)

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

type workflowHandlerStub struct {
	called bool
}

func (s *workflowHandlerStub) RequestWorkflow(_ context.Context, testee, assessment uint64, r app.WorkflowRequest) (*app.WorkflowAccepted, error) {
	s.called = true
	if testee != 7 || assessment != 42 || r.ReportID != "99" {
		return nil, app.ErrInvalidRequest
	}
	return &app.WorkflowAccepted{RequestID: r.RequestID, Status: "accepted"}, nil
}
func TestWorkflowHandlerUsesDistinctAcceptedRequestIdentity(t *testing.T) {
	stub := &workflowHandlerStub{}
	h := NewAIExplanationHandler(stub)
	recorder, c := newAIExplanationTestContext(http.MethodPost, "/api/v1/assessments/42/ai-workflows?testee_id=7", `{"request_id":"00000000-0000-4000-8000-000000000001","report_id":"99"}`)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "42"})
	h.RequestWorkflow(c)
	if !stub.called || recorder.Code != http.StatusAccepted || strings.Contains(recorder.Body.String(), "generation_id") {
		t.Fatalf("response=%d %s", recorder.Code, recorder.Body.String())
	}
}

func (s *workflowHandlerStub) GetWorkflow(_ context.Context, testee, assessment uint64, requestID string) (*app.WorkflowResult, error) {
	s.called = true
	if testee != 7 || assessment != 42 || requestID != "00000000-0000-4000-8000-000000000001" {
		return nil, app.ErrInvalidRequest
	}
	return &app.WorkflowResult{RequestID: requestID, Status: "running", Version: 2}, nil
}
func TestWorkflowReadHandlerUsesParticipantAndRequestIdentity(t *testing.T) {
	stub := &workflowHandlerStub{}
	h := NewAIExplanationHandler(stub)
	recorder, c := newAIExplanationTestContext(http.MethodGet, "/api/v1/assessments/42/ai-workflows/00000000-0000-4000-8000-000000000001?testee_id=7", "")
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "42"}, gin.Param{Key: "request_id", Value: "00000000-0000-4000-8000-000000000001"})
	h.GetWorkflow(c)
	if !stub.called || recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"running"`) || strings.Contains(recorder.Body.String(), "generation_id") {
		t.Fatalf("response=%d %s", recorder.Code, recorder.Body.String())
	}
}

func (s *workflowHandlerStub) GetWorkflowSource(_ context.Context, testee, assessment uint64) (*app.WorkflowSource, error) {
	s.called = true
	if testee != 7 || assessment != 42 {
		return nil, app.ErrInvalidRequest
	}
	return &app.WorkflowSource{Status: "ready", ReportID: "99", SourceVersion: "standard-v1:101", AIEligibility: &aiport.WorkflowEligibility{Status: "unavailable", ReasonCode: "publication_missing"}}, nil
}
func TestWorkflowSourceRouteReturnsProvenanceInsteadOfParsingSourceAsRequestID(t *testing.T) {
	stub := &workflowHandlerStub{}
	h := NewAIExplanationHandler(stub)
	r := gin.New()
	r.GET("/assessments/:id/ai-workflows/source", h.GetWorkflowSource)
	r.GET("/assessments/:id/ai-workflows/:request_id", h.GetWorkflow)
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assessments/42/ai-workflows/source?testee_id=7", nil))
	if !stub.called || recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"report_id":"99"`) || strings.Contains(recorder.Body.String(), "request_id") || !strings.Contains(recorder.Body.String(), `"ai_eligibility":{"status":"unavailable","reason_code":"publication_missing"}`) {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
}
