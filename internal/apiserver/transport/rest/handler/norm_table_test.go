package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/gin-gonic/gin"
)

type normTableServiceStub struct {
	imported *domain.Norm
}

func (s *normTableServiceStub) Import(_ context.Context, _ modelcatalog.ActorContext, table *domain.Norm) (*modelcatalog.NormTableDetail, error) {
	s.imported = table
	return modelcatalog.NormTableDetailFromDomain(table), nil
}

func (*normTableServiceStub) Get(_ context.Context, _ modelcatalog.ActorContext, tableVersion string) (*modelcatalog.NormTableDetail, error) {
	return &modelcatalog.NormTableDetail{NormTableSummary: modelcatalog.NormTableSummary{TableVersion: tableVersion}}, nil
}

func (*normTableServiceStub) List(_ context.Context, _ modelcatalog.ActorContext, input modelcatalog.ListNormTablesDTO) (*modelcatalog.NormTableListResult, error) {
	return &modelcatalog.NormTableListResult{Items: []modelcatalog.NormTableSummary{{TableVersion: "brief2-parent-2026"}}, Total: 1, Page: input.Page, PageSize: input.PageSize}, nil
}

func TestNormTableImportBindsFormalDTO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &normTableServiceStub{}
	handler := NewNormTableHandler(service)
	handler.BaseHandler = *NewBaseHandler()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/norm-tables", strings.NewReader(`{
        "table_version":"brief2-parent-2026","form_variant":"parent","kind":"behavioral_rating","algorithm":"brief2",
        "factors":[{"factor_code":"gec","lookup":[{"raw_score_min":10,"raw_score_max":10,"t_score":55,"percentile":69}]}]
    }`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAssessmentModelActor(ctx)

	handler.Import(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if service.imported == nil || service.imported.TableVersion != "brief2-parent-2026" || service.imported.Factors[0].Lookup[0].TScore != 55 {
		t.Fatalf("bound table = %+v", service.imported)
	}
	var body struct {
		Data struct {
			TableVersion string `json:"table_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.TableVersion != "brief2-parent-2026" {
		t.Fatalf("table_version = %q", body.Data.TableVersion)
	}
}

func TestNormTableListReturnsPagingEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewNormTableHandler(&normTableServiceStub{})
	handler.BaseHandler = *NewBaseHandler()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/norm-tables?page=2&page_size=10", nil)
	setAssessmentModelActor(ctx)

	handler.List(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data modelcatalog.NormTableListResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Total != 1 || body.Data.Page != 2 || body.Data.PageSize != 10 {
		t.Fatalf("response data = %+v", body.Data)
	}
}

func TestNormTableImportRejectsMissingRequiredLookupScore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewNormTableHandler(&normTableServiceStub{})
	handler.BaseHandler = *NewBaseHandler()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/norm-tables", strings.NewReader(`{
        "table_version":"brief2-parent-2026","form_variant":"parent","kind":"behavioral_rating","algorithm":"brief2",
        "factors":[{"factor_code":"gec","lookup":[{"raw_score_min":0,"raw_score_max":0,"percentile":0}]}]
    }`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	setAssessmentModelActor(ctx)

	handler.Import(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
}
