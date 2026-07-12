package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/gin-gonic/gin"
)

type questionnaireContentServiceStub struct {
	questionnaire.QuestionnaireContentService
	updated *questionnaire.UpdateQuestionDTO
	removed struct {
		questionnaireCode string
		questionCode      string
	}
}

func (s *questionnaireContentServiceStub) UpdateQuestion(_ context.Context, dto questionnaire.UpdateQuestionDTO) (*questionnaire.QuestionnaireResult, error) {
	s.updated = &dto
	return &questionnaire.QuestionnaireResult{Code: dto.QuestionnaireCode}, nil
}

func (s *questionnaireContentServiceStub) RemoveQuestion(_ context.Context, questionnaireCode, questionCode string) (*questionnaire.QuestionnaireResult, error) {
	s.removed.questionnaireCode = questionnaireCode
	s.removed.questionCode = questionCode
	return &questionnaire.QuestionnaireResult{Code: questionnaireCode}, nil
}

func TestQuestionnaireHandlerUpdateQuestionReadsQCodeRouteParameter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &questionnaireContentServiceStub{}
	h := NewQuestionnaireHandler(nil, service, nil, nil)
	h.BaseHandler = *NewBaseHandler()

	engine := gin.New()
	engine.PUT("/questionnaires/:code/questions/:qcode", h.UpdateQuestion)
	req := httptest.NewRequest(http.MethodPut, "/questionnaires/survey-1/questions/question-1", bytes.NewBufferString(`{"code":"question-1","stem":"题干","type":"Radio"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	engine.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if service.updated == nil || service.updated.QuestionnaireCode != "survey-1" {
		t.Fatalf("UpdateQuestion DTO = %+v, want questionnaire code survey-1", service.updated)
	}
}

func TestQuestionnaireHandlerRemoveQuestionReadsQCodeRouteParameter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &questionnaireContentServiceStub{}
	h := NewQuestionnaireHandler(nil, service, nil, nil)
	h.BaseHandler = *NewBaseHandler()

	engine := gin.New()
	engine.DELETE("/questionnaires/:code/questions/:qcode", h.RemoveQuestion)
	resp := httptest.NewRecorder()

	engine.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/questionnaires/survey-1/questions/question-1", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	if service.removed.questionnaireCode != "survey-1" || service.removed.questionCode != "question-1" {
		t.Fatalf("RemoveQuestion arguments = %+v, want survey-1/question-1", service.removed)
	}
}
