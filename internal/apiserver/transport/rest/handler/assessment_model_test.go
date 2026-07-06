package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type assessmentModelServiceStub struct {
	validateResult *modelcatalog.ValidationResult
	validateErr    error
	publishResult  *modelcatalog.ModelSummary
	publishErr     error
	publishCalled  bool
	previewResult  *modelcatalog.PreviewReportResult
	previewErr     error
	qrCodeURL      string
	qrCodeErr      error
}

func (s *assessmentModelServiceStub) List(context.Context, modelcatalog.ListModelsDTO) (*modelcatalog.ModelListResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Create(context.Context, modelcatalog.CreateModelDTO) (*modelcatalog.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Get(context.Context, string) (*modelcatalog.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) UpdateBasicInfo(context.Context, modelcatalog.UpdateBasicInfoDTO) (*modelcatalog.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Delete(context.Context, string) error {
	return nil
}

func (s *assessmentModelServiceStub) Publish(context.Context, string) (*modelcatalog.ModelSummary, error) {
	s.publishCalled = true
	return s.publishResult, s.publishErr
}

func (s *assessmentModelServiceStub) Unpublish(context.Context, string) (*modelcatalog.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Archive(context.Context, string) (*modelcatalog.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) BindQuestionnaire(context.Context, modelcatalog.BindQuestionnaireDTO) (*modelcatalog.QuestionnaireBindingResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) GetQuestionnaire(context.Context, string) (*modelcatalog.QuestionnaireBindingResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) GetDefinition(context.Context, string) (*modelcatalog.DefinitionDTO, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) UpdateDefinition(context.Context, string, modelcatalog.DefinitionDTO) (*modelcatalog.DefinitionDTO, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Options(context.Context, string) (*modelcatalog.OptionsResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) ApplyCodes(context.Context, modelcatalog.ApplyCodesDTO) ([]string, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Validate(context.Context, string) (*modelcatalog.ValidationResult, error) {
	return s.validateResult, s.validateErr
}

func (s *assessmentModelServiceStub) PreviewReport(context.Context, string, json.RawMessage) (*modelcatalog.PreviewReportResult, error) {
	return s.previewResult, s.previewErr
}

func (s *assessmentModelServiceStub) GetQRCode(context.Context, string) (string, error) {
	return s.qrCodeURL, s.qrCodeErr
}

func TestAssessmentModelPublishReturnsValidationResultWhenInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	validation := modelcatalog.NewValidationResult([]modelcatalog.ValidationIssue{
		{
			Field:   "definition.payload",
			Message: "模型定义 payload 不能为空",
			Code:    "definition.payload.required",
			Level:   "error",
		},
	})
	svc := &assessmentModelServiceStub{validateResult: validation}
	handler := NewAssessmentModelHandler(svc)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/assessment-models/model_bad/publish", nil)
	c.Params = gin.Params{{Key: "code", Value: "model_bad"}}

	handler.Publish(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.publishCalled {
		t.Fatal("Publish should not be called when validation fails")
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Passed bool                              `json:"passed"`
			Valid  bool                              `json:"valid"`
			Issues []modelcatalog.ValidationIssue `json:"issues"`
			Errors []string                          `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != code.ErrAssessmentModelValidationFailed {
		t.Fatalf("response code = %d, want %d", body.Code, code.ErrAssessmentModelValidationFailed)
	}
	if body.Message != "模型校验失败" {
		t.Fatalf("response message = %q", body.Message)
	}
	if body.Data.Passed || body.Data.Valid {
		t.Fatalf("validation result should be failed, got passed=%v valid=%v", body.Data.Passed, body.Data.Valid)
	}
	if len(body.Data.Issues) != 1 || body.Data.Issues[0].Code != "definition.payload.required" {
		t.Fatalf("unexpected issues: %+v", body.Data.Issues)
	}
	if len(body.Data.Errors) != 1 || body.Data.Errors[0] != "模型定义 payload 不能为空" {
		t.Fatalf("unexpected errors: %+v", body.Data.Errors)
	}
}

func TestAssessmentModelPreviewReportReturnsValidationResultWhenInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &assessmentModelServiceStub{
		previewErr: modelcatalog.NewValidationFailedError([]modelcatalog.ValidationIssue{
			{
				Field:   "answers[0].question_code",
				Message: `question_code "UNKNOWN" 不存在于绑定问卷`,
				Code:    "question_code.not_found",
				Level:   "error",
			},
		}),
	}
	handler := NewAssessmentModelHandler(svc)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/assessment-models/model_bad/preview-report",
		strings.NewReader(`{"answers":[{"question_code":"UNKNOWN","score":1}]}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "code", Value: "model_bad"}}

	handler.PreviewReport(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Passed bool                              `json:"passed"`
			Issues []modelcatalog.ValidationIssue `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != code.ErrAssessmentModelValidationFailed {
		t.Fatalf("response code = %d, want %d", body.Code, code.ErrAssessmentModelValidationFailed)
	}
	if body.Data.Passed {
		t.Fatal("validation result should be failed")
	}
	if len(body.Data.Issues) != 1 || body.Data.Issues[0].Code != "question_code.not_found" {
		t.Fatalf("unexpected issues: %+v", body.Data.Issues)
	}
}

func TestAssessmentModelGetQRCodeReturnsURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &assessmentModelServiceStub{qrCodeURL: "https://example.com/qrcodes/personality_demo.png"}
	handler := NewAssessmentModelHandler(svc)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessment-models/personality_demo/qrcode", nil)
	c.Params = gin.Params{{Key: "code", Value: "personality_demo"}}

	handler.GetQRCode(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			QRCodeURL string `json:"qrcode_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.QRCodeURL != svc.qrCodeURL {
		t.Fatalf("qrcode_url = %q, want %q", body.Data.QRCodeURL, svc.qrCodeURL)
	}
}
