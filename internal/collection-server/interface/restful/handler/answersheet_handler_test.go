package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	collectionmiddleware "github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/middleware"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
)

type fakeAnswerSheetSubmissionService struct {
	submitQueued func(ctx context.Context, requestID string, writerID uint64, req *answersheet.SubmitAnswerSheetRequest) error
}

func (f *fakeAnswerSheetSubmissionService) SubmitQueued(ctx context.Context, requestID string, writerID uint64, req *answersheet.SubmitAnswerSheetRequest) error {
	if f.submitQueued == nil {
		panic("unexpected SubmitQueued call")
	}
	return f.submitQueued(ctx, requestID, writerID, req)
}

func (f *fakeAnswerSheetSubmissionService) GetSubmitStatus(string) (*answersheet.SubmitStatusResponse, bool) {
	panic("unexpected GetSubmitStatus call")
}

func (f *fakeAnswerSheetSubmissionService) Get(context.Context, uint64) (*answersheet.AnswerSheetResponse, error) {
	panic("unexpected Get call")
}

func TestAnswerSheetHandlerSubmitAlwaysReturnsAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewAnswerSheetHandler(&fakeAnswerSheetSubmissionService{
		submitQueued: func(_ context.Context, requestID string, writerID uint64, req *answersheet.SubmitAnswerSheetRequest) error {
			if requestID != "req-123" {
				t.Fatalf("expected request id req-123, got %q", requestID)
			}
			if writerID != 99 {
				t.Fatalf("expected writer id 99, got %d", writerID)
			}
			if req == nil || req.QuestionnaireCode != "qs" {
				t.Fatalf("unexpected request: %+v", req)
			}
			return nil
		},
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/answersheets", strings.NewReader(`{
		"questionnaire_code":"qs",
		"questionnaire_version":"v1",
		"testee_id":1,
		"answers":[{"question_code":"q1","question_type":"single_choice","value":"1"}]
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set(pkgmiddleware.XRequestIDKey, "req-123")
	c.Set(collectionmiddleware.UserIDKey, uint64(99))

	handler.Submit(c)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", recorder.Code)
	}

	var resp struct {
		Code    int                                `json:"code"`
		Message string                             `json:"message"`
		Data    answersheet.SubmitAcceptedResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("expected code 0, got %d", resp.Code)
	}
	if resp.Message != "accepted" {
		t.Fatalf("expected message accepted, got %q", resp.Message)
	}
	if resp.Data.Status != answersheet.SubmitStatusQueued {
		t.Fatalf("expected queued status, got %q", resp.Data.Status)
	}
	if resp.Data.RequestID != "req-123" {
		t.Fatalf("expected request id req-123, got %q", resp.Data.RequestID)
	}
}

func TestAnswerSheetHandlerSubmitQueueFull(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewAnswerSheetHandler(&fakeAnswerSheetSubmissionService{
		submitQueued: func(context.Context, string, uint64, *answersheet.SubmitAnswerSheetRequest) error {
			return answersheet.ErrQueueFull
		},
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/answersheets", strings.NewReader(`{
		"questionnaire_code":"qs",
		"questionnaire_version":"v1",
		"testee_id":1,
		"answers":[{"question_code":"q1","question_type":"single_choice","value":"1"}]
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(collectionmiddleware.UserIDKey, uint64(99))

	handler.Submit(c)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", recorder.Code)
	}

	var resp core.ErrResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Message != "submit queue full" {
		t.Fatalf("expected queue full message, got %q", resp.Message)
	}
}
