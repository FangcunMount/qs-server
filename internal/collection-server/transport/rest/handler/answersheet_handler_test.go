package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	collectionmiddleware "github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/middleware"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAnswerSheetSubmissionService struct {
	accept    func(context.Context, string, uint64, *answersheet.SubmitAnswerSheetRequest) (*answersheet.SubmitAnswerSheetResponse, error)
	readiness func(context.Context, uint64, uint64, uint64) (*answersheet.AssessmentReadinessResponse, error)
}

func (f *fakeAnswerSheetSubmissionService) AcceptDurably(ctx context.Context, requestID string, writerID uint64, req *answersheet.SubmitAnswerSheetRequest) (*answersheet.SubmitAnswerSheetResponse, error) {
	return f.accept(ctx, requestID, writerID, req)
}

func (f *fakeAnswerSheetSubmissionService) GetAssessmentReadiness(ctx context.Context, writerID, answerSheetID, testeeID uint64) (*answersheet.AssessmentReadinessResponse, error) {
	return f.readiness(ctx, writerID, answerSheetID, testeeID)
}

func (*fakeAnswerSheetSubmissionService) Get(context.Context, uint64) (*answersheet.AnswerSheetResponse, error) {
	return nil, nil
}

func TestAnswerSheetHandlerReturnsAcceptedOnlyWithDurableID(t *testing.T) {
	service := &fakeAnswerSheetSubmissionService{accept: func(_ context.Context, requestID string, writerID uint64, req *answersheet.SubmitAnswerSheetRequest) (*answersheet.SubmitAnswerSheetResponse, error) {
		if requestID != "req-123" || writerID != 99 || req.IdempotencyKey != "submit-1234" {
			t.Fatalf("unexpected accept input: request=%q writer=%d req=%+v", requestID, writerID, req)
		}
		return &answersheet.SubmitAnswerSheetResponse{ID: "90010001"}, nil
	}}
	handler := NewAnswerSheetHandler(service)
	recorder, c := newAnswerSheetTestContext(http.MethodPost, "/api/v1/answersheets", `{
		"questionnaire_code":"qs","questionnaire_version":"v1","idempotency_key":"submit-1234",
		"testee_id":1,"answers":[{"question_code":"q1","question_type":"Text","value":"answer"}]
	}`)
	c.Request.Header.Set(pkgmiddleware.XRequestIDKey, "req-123")
	c.Set(collectionmiddleware.UserIDKey, uint64(99))
	handler.Submit(c)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data answersheet.SubmitAcceptedResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Status != "accepted" || response.Data.RequestID != "req-123" || response.Data.AnswerSheetID != "90010001" {
		t.Fatalf("response = %#v", response.Data)
	}
}

func TestAnswerSheetHandlerDoesNotReturn202WhenDurableSaveFails(t *testing.T) {
	service := &fakeAnswerSheetSubmissionService{accept: func(context.Context, string, uint64, *answersheet.SubmitAnswerSheetRequest) (*answersheet.SubmitAnswerSheetResponse, error) {
		return nil, status.Error(codes.Unavailable, "mongo unavailable")
	}}
	handler := NewAnswerSheetHandler(service)
	recorder, c := newAnswerSheetTestContext(http.MethodPost, "/api/v1/answersheets", `{
		"questionnaire_code":"qs","questionnaire_version":"v1","idempotency_key":"submit-1234",
		"testee_id":1,"answers":[{"question_code":"q1","question_type":"Text","value":"answer"}]
	}`)
	c.Set(collectionmiddleware.UserIDKey, uint64(99))
	handler.Submit(c)
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("status=%d retry-after=%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
}

func TestAnswerSheetHandlerMapsIdempotencyConflict(t *testing.T) {
	service := &fakeAnswerSheetSubmissionService{accept: func(context.Context, string, uint64, *answersheet.SubmitAnswerSheetRequest) (*answersheet.SubmitAnswerSheetResponse, error) {
		return nil, status.Error(codes.AlreadyExists, "idempotency conflict")
	}}
	handler := NewAnswerSheetHandler(service)
	recorder, c := newAnswerSheetTestContext(http.MethodPost, "/api/v1/answersheets", `{
		"questionnaire_code":"qs","questionnaire_version":"v1","idempotency_key":"submit-1234",
		"testee_id":1,"answers":[{"question_code":"q1","question_type":"Text","value":"answer"}]
	}`)
	c.Set(collectionmiddleware.UserIDKey, uint64(99))
	handler.Submit(c)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAnswerSheetHandlerAssessmentReadiness(t *testing.T) {
	service := &fakeAnswerSheetSubmissionService{readiness: func(_ context.Context, writerID, answerSheetID, testeeID uint64) (*answersheet.AssessmentReadinessResponse, error) {
		if writerID != 99 || answerSheetID != 42 || testeeID != 7 {
			t.Fatalf("unexpected readiness input: %d %d %d", writerID, answerSheetID, testeeID)
		}
		return &answersheet.AssessmentReadinessResponse{Status: "pending", AnswerSheetID: "42", NextPollAfterMs: 2000}, nil
	}}
	handler := NewAnswerSheetHandler(service)
	recorder, c := newAnswerSheetTestContext(http.MethodGet, "/api/v1/answersheets/42/assessment-readiness?testee_id=7", "")
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "42"})
	c.Set(collectionmiddleware.UserIDKey, uint64(99))
	handler.AssessmentReadiness(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newAnswerSheetTestContext(method, target, body string) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return recorder, c
}
