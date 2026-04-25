package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	answersheetapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type stubAnswerSheetManagementService struct {
	lastGetByID uint64
	lastListDTO answersheetapp.ListAnswerSheetsDTO

	getByIDResult *answersheetapp.AnswerSheetResult
	getByIDErr    error
	listResult    *answersheetapp.AnswerSheetSummaryListResult
	listErr       error
}

func (s *stubAnswerSheetManagementService) GetByID(_ context.Context, id uint64) (*answersheetapp.AnswerSheetResult, error) {
	s.lastGetByID = id
	return s.getByIDResult, s.getByIDErr
}

func (s *stubAnswerSheetManagementService) List(_ context.Context, dto answersheetapp.ListAnswerSheetsDTO) (*answersheetapp.AnswerSheetSummaryListResult, error) {
	s.lastListDTO = dto
	return s.listResult, s.listErr
}

func (s *stubAnswerSheetManagementService) Delete(context.Context, uint64) error { return nil }

type stubAnswerSheetSubmissionService struct {
	lastSubmitDTO answersheetapp.SubmitAnswerSheetDTO

	submitResult *answersheetapp.AnswerSheetResult
	submitErr    error
}

func (s *stubAnswerSheetSubmissionService) Submit(_ context.Context, dto answersheetapp.SubmitAnswerSheetDTO) (*answersheetapp.AnswerSheetResult, error) {
	s.lastSubmitDTO = dto
	return s.submitResult, s.submitErr
}

func (s *stubAnswerSheetSubmissionService) GetMyAnswerSheet(context.Context, uint64, uint64) (*answersheetapp.AnswerSheetResult, error) {
	return nil, nil
}

func (s *stubAnswerSheetSubmissionService) ListMyAnswerSheets(context.Context, answersheetapp.ListMyAnswerSheetsDTO) (*answersheetapp.AnswerSheetSummaryListResult, error) {
	return nil, nil
}

func newAnswerSheetHandlerForTest(
	management answersheetapp.AnswerSheetManagementService,
	submission answersheetapp.AnswerSheetSubmissionService,
) *AnswerSheetHandler {
	handler := NewAnswerSheetHandler(management, submission)
	handler.BaseHandler = *NewBaseHandler()
	return handler
}

func newHandlerTestContext(method, target string, body *bytes.Reader) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, body)
	c.Request = req
	return c, rec
}

func TestAnswerSheetHandlerGetByIDSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	filledAt := time.Date(2026, 4, 22, 8, 30, 0, 0, time.UTC)
	management := &stubAnswerSheetManagementService{
		getByIDResult: &answersheetapp.AnswerSheetResult{
			ID:                 42,
			QuestionnaireCode:  "QNR-42",
			QuestionnaireVer:   "v2",
			QuestionnaireTitle: "评估问卷",
			FillerID:           101,
			FillerName:         "Alice",
			FilledAt:           filledAt,
			Score:              86.5,
			Answers: []answersheetapp.AnswerResult{
				{QuestionCode: "q1", QuestionType: "radio", Value: "A", Score: 2},
			},
		},
	}
	handler := newAnswerSheetHandlerForTest(management, &stubAnswerSheetSubmissionService{})

	c, rec := newHandlerTestContext(http.MethodGet, "/api/v1/answersheets/42", bytes.NewReader(nil))
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler.GetByID(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if management.lastGetByID != 42 {
		t.Fatalf("lastGetByID = %d, want 42", management.lastGetByID)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID                string `json:"id"`
			QuestionnaireCode string `json:"questionnaire_code"`
			FillerID          string `json:"filler_id"`
			FillerName        string `json:"filler_name"`
			Answers           []struct {
				QuestionCode string `json:"question_code"`
			} `json:"answers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("code = %d, want 0", payload.Code)
	}
	if payload.Data.ID != "42" || payload.Data.QuestionnaireCode != "QNR-42" {
		t.Fatalf("unexpected response data: %+v", payload.Data)
	}
	if payload.Data.FillerID != "101" || payload.Data.FillerName != "Alice" {
		t.Fatalf("unexpected filler data: %+v", payload.Data)
	}
	if len(payload.Data.Answers) != 1 || payload.Data.Answers[0].QuestionCode != "q1" {
		t.Fatalf("unexpected answers: %+v", payload.Data.Answers)
	}
}

func TestAnswerSheetHandlerListSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	filledAt := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)
	management := &stubAnswerSheetManagementService{
		listResult: &answersheetapp.AnswerSheetSummaryListResult{
			Total: 1,
			Items: []*answersheetapp.AnswerSheetSummaryResult{
				{
					ID:                 77,
					QuestionnaireCode:  "QNR-LIST",
					QuestionnaireTitle: "问卷列表",
					FillerID:           303,
					Score:              91.2,
					FilledAt:           filledAt,
				},
			},
		},
	}
	handler := newAnswerSheetHandlerForTest(management, &stubAnswerSheetSubmissionService{})

	c, rec := newHandlerTestContext(http.MethodGet, "/api/v1/answersheets?page=2&page_size=20&questionnaire_code=QNR-LIST&filler_id=303&start_time=2026-04-01&end_time=2026-04-02", bytes.NewReader(nil))

	handler.List(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if management.lastListDTO.Page != 2 || management.lastListDTO.PageSize != 20 {
		t.Fatalf("unexpected pagination dto: %+v", management.lastListDTO)
	}
	if management.lastListDTO.QuestionnaireCode != "QNR-LIST" {
		t.Fatalf("questionnaire_code = %q, want QNR-LIST", management.lastListDTO.QuestionnaireCode)
	}
	if management.lastListDTO.FillerID == nil || *management.lastListDTO.FillerID != 303 {
		t.Fatalf("filler_id = %+v, want 303", management.lastListDTO.FillerID)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				ID                string `json:"id"`
				QuestionnaireCode string `json:"questionnaire_code"`
				FillerID          string `json:"filler_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 || payload.Data.Total != 1 || len(payload.Data.Items) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Data.Items[0].ID != "77" || payload.Data.Items[0].FillerID != "303" {
		t.Fatalf("unexpected list item: %+v", payload.Data.Items[0])
	}
}

func TestAnswerSheetHandlerAdminSubmitSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	submission := &stubAnswerSheetSubmissionService{
		submitResult: &answersheetapp.AnswerSheetResult{
			ID:                 95,
			QuestionnaireCode:  "QNR-ADMIN",
			QuestionnaireVer:   "v1",
			QuestionnaireTitle: "后台提交",
			FillerID:           909,
			FilledAt:           time.Date(2026, 4, 22, 10, 15, 0, 0, time.UTC),
		},
	}
	handler := newAnswerSheetHandlerForTest(&stubAnswerSheetManagementService{}, submission)

	body := bytes.NewReader([]byte(`{"questionnaire_code":"QNR-ADMIN","questionnaire_version":"v1","testee_id":808,"answers":[{"question_code":"q1","question_type":"radio","value":"A"}]}`))
	c, rec := newHandlerTestContext(http.MethodPost, "/api/v1/answersheets/admin-submit", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.OrgIDKey, uint64(88))
	c.Set(middleware.UserIDKey, uint64(909))

	handler.AdminSubmit(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if submission.lastSubmitDTO.OrgID != 88 || submission.lastSubmitDTO.FillerID != 909 || submission.lastSubmitDTO.TesteeID != 808 {
		t.Fatalf("unexpected submit dto: %+v", submission.lastSubmitDTO)
	}
	if len(submission.lastSubmitDTO.Answers) != 1 || submission.lastSubmitDTO.Answers[0].QuestionCode != "q1" {
		t.Fatalf("unexpected submit answers: %+v", submission.lastSubmitDTO.Answers)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID       string `json:"id"`
			FillerID string `json:"filler_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 || payload.Data.ID != "95" || payload.Data.FillerID != "909" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
