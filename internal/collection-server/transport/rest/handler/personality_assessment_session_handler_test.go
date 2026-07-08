package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitysession"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/gin-gonic/gin"
)

func TestPersonalityAssessmentSessionHandlerStartReturnsSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewPersonalityAssessmentSessionHandler(personalitysession.NewService(
		&sessionModelReader{model: &typologymodel.PersonalityModelResponse{
			Code:                 "MBTI_OEJTS",
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
			Status:               "published",
		}},
		&sessionQuestionnaireReader{questionnaire: &questionnaire.QuestionnaireResponse{
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
		}},
	))

	body, err := json.Marshal(personalitysession.StartSessionRequest{
		ModelCode: "MBTI_OEJTS",
		TesteeID:  7,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/personality-assessment-sessions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Start(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPersonalityAssessmentSessionHandlerStartAcceptsStringTesteeID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewPersonalityAssessmentSessionHandler(personalitysession.NewService(
		&sessionModelReader{model: &typologymodel.PersonalityModelResponse{
			Code:                 "MBTI_OEJTS",
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
			Status:               "published",
		}},
		&sessionQuestionnaireReader{questionnaire: &questionnaire.QuestionnaireResponse{
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
		}},
	))

	body := []byte(`{"model_code":"MBTI_OEJTS","testee_id":"618855887087350318"}`)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/personality-assessment-sessions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Start(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

type sessionModelReader struct {
	model *typologymodel.PersonalityModelResponse
}

func (r *sessionModelReader) Get(context.Context, string) (*typologymodel.PersonalityModelResponse, error) {
	return r.model, nil
}

type sessionQuestionnaireReader struct {
	questionnaire *questionnaire.QuestionnaireResponse
}

func (r *sessionQuestionnaireReader) Get(context.Context, string, string) (*questionnaire.QuestionnaireResponse, error) {
	return r.questionnaire, nil
}
