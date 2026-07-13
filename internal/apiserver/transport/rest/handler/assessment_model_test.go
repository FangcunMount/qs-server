package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
	"github.com/gin-gonic/gin"
)

type assessmentModelPublicationStub struct {
	publishResult *modelcatalog.ModelSummary
	publishErr    error
	publishCalled bool
}

type assessmentReleaseStub struct {
	publishCalled bool
	archiveCalled bool
}

func (s *assessmentReleaseStub) PublishRelease(_ context.Context, _ modelcatalog.ActorContext, code string) (*modelcatalog.AssessmentRelease, error) {
	s.publishCalled = true
	return &modelcatalog.AssessmentRelease{ModelCode: code, ModelStatus: "published", QuestionnaireCode: "q1", QuestionnaireVersion: "1.0.0", QuestionnaireStatus: "published"}, nil
}

func (s *assessmentReleaseStub) ArchiveRelease(_ context.Context, _ modelcatalog.ActorContext, code string) (*modelcatalog.AssessmentRelease, error) {
	s.archiveCalled = true
	return &modelcatalog.AssessmentRelease{ModelCode: code, ModelStatus: "archived", QuestionnaireCode: "q1", QuestionnaireVersion: "1.0.0", QuestionnaireStatus: "archived"}, nil
}

func (s *assessmentModelPublicationStub) Publish(_ context.Context, _ modelcatalog.ActorContext, _ string) (*modelcatalog.ModelSummary, error) {
	s.publishCalled = true
	return s.publishResult, s.publishErr
}

func (*assessmentModelPublicationStub) Unpublish(context.Context, modelcatalog.ActorContext, string) (*modelcatalog.ModelSummary, error) {
	return nil, nil
}

type assessmentModelDefinitionStub struct {
	previewResult *modelcatalog.PreviewReportResult
	previewErr    error
}

func (*assessmentModelDefinitionStub) GetDefinition(context.Context, modelcatalog.ActorContext, string) (*domain.Definition, error) {
	return nil, nil
}
func (*assessmentModelDefinitionStub) SaveDefinition(context.Context, modelcatalog.ActorContext, string, *domain.Definition) (*domain.Definition, error) {
	return nil, nil
}
func (*assessmentModelDefinitionStub) ValidateDefinition(context.Context, modelcatalog.ActorContext, string) (*modelcatalog.ValidationResult, error) {
	return nil, nil
}
func (s *assessmentModelDefinitionStub) PreviewReport(context.Context, modelcatalog.ActorContext, string, json.RawMessage) (*modelcatalog.PreviewReportResult, error) {
	return s.previewResult, s.previewErr
}
func (*assessmentModelDefinitionStub) ApplyCodes(context.Context, modelcatalog.ActorContext, modelcatalog.ApplyCodesDTO) ([]string, error) {
	return nil, nil
}

type assessmentModelQueryStub struct {
	qrCodeURL string
	qrCodeErr error
}

func (*assessmentModelQueryStub) Get(context.Context, modelcatalog.ActorContext, string) (*modelcatalog.ModelSummary, error) {
	return nil, nil
}
func (*assessmentModelQueryStub) List(context.Context, modelcatalog.ActorContext, modelcatalog.ListModelsDTO) (*modelcatalog.ModelListResult, error) {
	return nil, nil
}
func (*assessmentModelQueryStub) GetPublished(context.Context, modelcatalog.ActorContext, string, string) (*modelcatalog.PublishedModelDetail, error) {
	return nil, nil
}
func (*assessmentModelQueryStub) ListPublished(context.Context, modelcatalog.ActorContext, modelcatalog.ListModelsDTO) (*modelcatalog.PublishedModelListResult, error) {
	return nil, nil
}
func (*assessmentModelQueryStub) ListHotPublished(context.Context, modelcatalog.ActorContext, modelcatalog.ListModelsDTO, int, int) (*modelcatalog.HotModelListResult, error) {
	return nil, nil
}
func (*assessmentModelQueryStub) GetQuestionnaire(context.Context, modelcatalog.ActorContext, string) (*modelcatalog.QuestionnaireBindingResult, error) {
	return nil, nil
}
func (*assessmentModelQueryStub) Options(context.Context, modelcatalog.ActorContext, string) (*modelcatalog.OptionsResult, error) {
	return nil, nil
}
func (s *assessmentModelQueryStub) GetQRCode(context.Context, modelcatalog.ActorContext, string) (string, error) {
	return s.qrCodeURL, s.qrCodeErr
}

func setAssessmentModelActor(c *gin.Context) {
	c.Set(restmiddleware.PrincipalKey, securityplane.Principal{Kind: securityplane.PrincipalKindUser, Source: securityplane.PrincipalSourceHTTPJWT, OrgID: 1, HasOrgID: true})
	c.Set(restmiddleware.OrgScopeKey, securityplane.OrgScope{OrgID: 1, HasOrgID: true})
}

func TestAssessmentModelPublishUsesPublicationService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &assessmentModelPublicationStub{publishResult: &modelcatalog.ModelSummary{Code: "model_ok", Title: "Model"}}
	handler := NewAssessmentModelHandler(nil, nil, svc, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/assessment-models/model_bad/publish", nil)
	c.Params = gin.Params{{Key: "code", Value: "model_bad"}}
	setAssessmentModelActor(c)

	handler.Publish(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !svc.publishCalled {
		t.Fatal("publication service was not called")
	}
}

func TestAssessmentReleaseHandlerUsesSinglePairService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &assessmentReleaseStub{}
	handler := NewAssessmentReleaseHandler(svc)

	for _, tc := range []struct {
		name string
		call func(*gin.Context)
	}{
		{name: "publish", call: handler.Publish},
		{name: "archive", call: handler.Archive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/assessment-releases/model_ok/"+tc.name, nil)
			c.Params = gin.Params{{Key: "code", Value: "model_ok"}}
			setAssessmentModelActor(c)

			tc.call(c)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
	if !svc.publishCalled || !svc.archiveCalled {
		t.Fatalf("release calls = publish:%t archive:%t, want both true", svc.publishCalled, svc.archiveCalled)
	}
}

func TestAssessmentModelPreviewReportReturnsValidationResultWhenInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &assessmentModelDefinitionStub{
		previewErr: modelcatalog.NewValidationFailedError([]modelcatalog.ValidationIssue{
			{
				Field:   "answers[0].question_code",
				Message: `question_code "UNKNOWN" 不存在于绑定问卷`,
				Code:    "question_code.not_found",
				Level:   "error",
			},
		}),
	}
	handler := NewAssessmentModelHandler(nil, svc, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/assessment-models/model_bad/preview-report",
		strings.NewReader(`{"answers":[{"question_code":"UNKNOWN","score":1}]}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "code", Value: "model_bad"}}
	setAssessmentModelActor(c)

	handler.PreviewReport(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Passed bool                           `json:"passed"`
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

	svc := &assessmentModelQueryStub{qrCodeURL: "https://example.com/qrcodes/personality_demo.png"}
	handler := NewAssessmentModelHandler(nil, nil, nil, svc)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessment-models/personality_demo/qrcode", nil)
	c.Params = gin.Params{{Key: "code", Value: "personality_demo"}}
	setAssessmentModelActor(c)

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
