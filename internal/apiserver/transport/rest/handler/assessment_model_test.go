package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type assessmentModelServiceStub struct {
	validateResult *assessmentmodel.ValidationResult
	validateErr    error
	publishResult  *assessmentmodel.ModelSummary
	publishErr     error
	publishCalled  bool
}

func (s *assessmentModelServiceStub) List(context.Context, assessmentmodel.ListModelsDTO) (*assessmentmodel.ModelListResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Create(context.Context, assessmentmodel.CreateModelDTO) (*assessmentmodel.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Get(context.Context, string) (*assessmentmodel.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) UpdateBasicInfo(context.Context, assessmentmodel.UpdateBasicInfoDTO) (*assessmentmodel.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Delete(context.Context, string) error {
	return nil
}

func (s *assessmentModelServiceStub) Publish(context.Context, string) (*assessmentmodel.ModelSummary, error) {
	s.publishCalled = true
	return s.publishResult, s.publishErr
}

func (s *assessmentModelServiceStub) Unpublish(context.Context, string) (*assessmentmodel.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Archive(context.Context, string) (*assessmentmodel.ModelSummary, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) BindQuestionnaire(context.Context, assessmentmodel.BindQuestionnaireDTO) (*assessmentmodel.QuestionnaireBindingResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) GetQuestionnaire(context.Context, string) (*assessmentmodel.QuestionnaireBindingResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) GetDefinition(context.Context, string) (*assessmentmodel.DefinitionDTO, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) UpdateDefinition(context.Context, string, assessmentmodel.DefinitionDTO) (*assessmentmodel.DefinitionDTO, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Options(context.Context, string) (*assessmentmodel.OptionsResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) ApplyCodes(context.Context, assessmentmodel.ApplyCodesDTO) ([]string, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) Validate(context.Context, string) (*assessmentmodel.ValidationResult, error) {
	return s.validateResult, s.validateErr
}

func (s *assessmentModelServiceStub) PreviewReport(context.Context, string, json.RawMessage) (*assessmentmodel.PreviewReportResult, error) {
	return nil, nil
}

func (s *assessmentModelServiceStub) GetQRCode(context.Context, string) (string, error) {
	return "", nil
}

func TestAssessmentModelPublishReturnsValidationResultWhenInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	validation := assessmentmodel.NewValidationResult([]assessmentmodel.ValidationIssue{
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
			Issues []assessmentmodel.ValidationIssue `json:"issues"`
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
