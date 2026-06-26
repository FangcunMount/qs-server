package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	personalityassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/personalityassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitysession"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/gin-gonic/gin"
)

func TestMiniProgramPersonalityAssessmentHTTPFlowContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		testeeID             uint64 = 1001
		answerSheetID        uint64 = 9001
		assessmentID         uint64 = 8001
		modelCode                   = "PERSONALITY_MODEL_A"
		questionnaireCode           = "Q_PERSONALITY_A"
		questionnaireVersion        = "1.0.0"
	)

	sessionHandler := NewPersonalityAssessmentSessionHandler(personalitysession.NewService(
		&httpFlowModelReader{model: &personalitymodel.PersonalityModelResponse{
			Code:                 modelCode,
			Version:              questionnaireVersion,
			QuestionnaireCode:    questionnaireCode,
			QuestionnaireVersion: questionnaireVersion,
			Status:               "published",
		}},
		&httpFlowQuestionnaireReader{questionnaire: &questionnaire.QuestionnaireResponse{
			Code:    questionnaireCode,
			Version: questionnaireVersion,
			Status:  "published",
		}},
	))

	sessionRecorder := httptest.NewRecorder()
	sessionCtx, _ := gin.CreateTestContext(sessionRecorder)
	sessionBody, _ := json.Marshal(personalitysession.StartSessionRequest{
		ModelCode: modelCode,
		TesteeID:  testeeID,
	})
	sessionCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/personality-assessment-sessions", bytes.NewReader(sessionBody))
	sessionCtx.Request.Header.Set("Content-Type", "application/json")
	sessionHandler.Start(sessionCtx)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", sessionRecorder.Code, sessionRecorder.Body.String())
	}

	var sessionResp struct {
		Data personalitysession.StartSessionResponse `json:"data"`
	}
	if err := json.Unmarshal(sessionRecorder.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("unmarshal session response: %v", err)
	}
	if sessionResp.Data.SubmitContract.QuestionnaireCode != questionnaireCode {
		t.Fatalf("submit contract = %#v", sessionResp.Data.SubmitContract)
	}

	assessmentHandler := NewPersonalityAssessmentHandler(&httpFlowAssessmentQueryService{
		report: &personalityassessment.AssessmentReportResponse{AssessmentID: "8001"},
	}, nil)

	reportRecorder := httptest.NewRecorder()
	reportCtx, _ := gin.CreateTestContext(reportRecorder)
	reportCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/personality-assessments/8001/report?testee_id=1001", nil)
	reportCtx.Params = gin.Params{{Key: "id", Value: "8001"}}
	assessmentHandler.GetReport(reportCtx)
	if reportRecorder.Code != http.StatusOK {
		t.Fatalf("report status = %d, body = %s", reportRecorder.Code, reportRecorder.Body.String())
	}

	_ = answerSheetID
	_ = assessmentID
}

type httpFlowModelReader struct {
	model *personalitymodel.PersonalityModelResponse
}

func (r *httpFlowModelReader) Get(context.Context, string) (*personalitymodel.PersonalityModelResponse, error) {
	return r.model, nil
}

type httpFlowQuestionnaireReader struct {
	questionnaire *questionnaire.QuestionnaireResponse
}

func (r *httpFlowQuestionnaireReader) Get(context.Context, string, string) (*questionnaire.QuestionnaireResponse, error) {
	return r.questionnaire, nil
}

type httpFlowAssessmentQueryService struct {
	report *personalityassessment.AssessmentReportResponse
}

func (s *httpFlowAssessmentQueryService) List(context.Context, uint64, *personalityassessment.ListAssessmentsRequest) (*personalityassessment.ListAssessmentsResponse, error) {
	panic("unexpected List call")
}

func (s *httpFlowAssessmentQueryService) Get(context.Context, uint64, uint64) (*personalityassessment.AssessmentDetailResponse, error) {
	panic("unexpected Get call")
}

func (s *httpFlowAssessmentQueryService) GetReport(_ context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentReportResponse, error) {
	if testeeID != 1001 || assessmentID != 8001 {
		return nil, nil
	}
	return s.report, nil
}

func (s *httpFlowAssessmentQueryService) WaitReport(context.Context, uint64, uint64, time.Duration) (*personalityassessment.AssessmentStatusResponse, error) {
	panic("unexpected WaitReport call")
}
