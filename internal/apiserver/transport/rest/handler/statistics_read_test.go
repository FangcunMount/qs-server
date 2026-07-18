package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	componentErrors "github.com/FangcunMount/component-base/pkg/errors"
	authzApp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/gin-gonic/gin"
)

type stubStatisticsReadService struct {
	lastOverviewOrgID  int64
	lastOverviewFilter statisticsApp.QueryFilter
	overviewResult     *domainStatistics.StatisticsOverview
	overviewErr        error

	lastListEntryOrgID      int64
	lastListEntryClinician  *uint64
	lastListEntryActiveOnly *bool
	lastListEntryFilter     statisticsApp.QueryFilter
	lastListEntryPage       int
	lastListEntryPageSize   int
	listEntryResult         *domainStatistics.AssessmentEntryStatisticsList
	listEntryErr            error

	lastCurrentClinicianOrgID    int64
	lastCurrentClinicianOperator int64
	lastCurrentClinicianFilter   statisticsApp.QueryFilter
	currentClinicianResult       *domainStatistics.ClinicianStatistics
	currentClinicianErr          error

	lastBatchOrgID  int64
	lastBatchRefs   []domainStatistics.ContentReference
	lastBatchAccess statisticsApp.ContentStatisticsAccess
	batchResult     *domainStatistics.ContentBatchStatisticsResponse
	batchErr        error
}

func (s *stubStatisticsReadService) GetOverview(_ context.Context, orgID int64, filter statisticsApp.QueryFilter) (*domainStatistics.StatisticsOverview, error) {
	s.lastOverviewOrgID = orgID
	s.lastOverviewFilter = filter
	if s.overviewResult != nil {
		return s.overviewResult, s.overviewErr
	}
	return &domainStatistics.StatisticsOverview{OrgID: orgID}, s.overviewErr
}

func (*stubStatisticsReadService) ListClinicianStatistics(context.Context, int64, statisticsApp.QueryFilter, int, int) (*domainStatistics.ClinicianStatisticsList, error) {
	return nil, nil
}

func (*stubStatisticsReadService) GetClinicianStatistics(context.Context, int64, uint64, statisticsApp.QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	return nil, nil
}

func (s *stubStatisticsReadService) ListAssessmentEntryStatistics(_ context.Context, orgID int64, clinicianID *uint64, activeOnly *bool, filter statisticsApp.QueryFilter, page, pageSize int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	s.lastListEntryOrgID = orgID
	s.lastListEntryClinician = clinicianID
	s.lastListEntryActiveOnly = activeOnly
	s.lastListEntryFilter = filter
	s.lastListEntryPage = page
	s.lastListEntryPageSize = pageSize
	if s.listEntryResult != nil {
		return s.listEntryResult, s.listEntryErr
	}
	return &domainStatistics.AssessmentEntryStatisticsList{}, s.listEntryErr
}

func (*stubStatisticsReadService) GetAssessmentEntryStatistics(context.Context, int64, uint64, statisticsApp.QueryFilter) (*domainStatistics.AssessmentEntryStatistics, error) {
	return nil, nil
}

func (s *stubStatisticsReadService) GetCurrentClinicianStatistics(_ context.Context, orgID int64, operatorUserID int64, filter statisticsApp.QueryFilter) (*domainStatistics.ClinicianStatistics, error) {
	s.lastCurrentClinicianOrgID = orgID
	s.lastCurrentClinicianOperator = operatorUserID
	s.lastCurrentClinicianFilter = filter
	if s.currentClinicianResult != nil {
		return s.currentClinicianResult, s.currentClinicianErr
	}
	return &domainStatistics.ClinicianStatistics{}, s.currentClinicianErr
}

func (*stubStatisticsReadService) ListCurrentClinicianEntryStatistics(context.Context, int64, int64, statisticsApp.QueryFilter, int, int) (*domainStatistics.AssessmentEntryStatisticsList, error) {
	return nil, nil
}

func (*stubStatisticsReadService) GetCurrentClinicianTesteeSummary(context.Context, int64, int64, statisticsApp.QueryFilter) (*domainStatistics.ClinicianTesteeSummaryStatistics, error) {
	return nil, nil
}

func (s *stubStatisticsReadService) GetContentBatchStatistics(_ context.Context, orgID int64, refs []domainStatistics.ContentReference, access statisticsApp.ContentStatisticsAccess) (*domainStatistics.ContentBatchStatisticsResponse, error) {
	s.lastBatchOrgID = orgID
	s.lastBatchRefs = append([]domainStatistics.ContentReference(nil), refs...)
	s.lastBatchAccess = access
	if s.batchResult != nil {
		return s.batchResult, s.batchErr
	}
	return &domainStatistics.ContentBatchStatisticsResponse{}, s.batchErr
}

func newStatisticsHandlerForTest(readService statisticsApp.ReadService) *StatisticsHandler {
	handler := NewStatisticsHandler(readService, nil, nil)
	handler.BaseHandler = *NewBaseHandler()
	return handler
}

func newStatisticsTestContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	return c, rec
}

func TestStatisticsHandlerGetOverviewUsesProtectedOrgScopeAndFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	readService := &stubStatisticsReadService{
		overviewResult: &domainStatistics.StatisticsOverview{OrgID: 88},
	}
	handler := newStatisticsHandlerForTest(readService)
	c, rec := newStatisticsTestContext(http.MethodGet, "/api/v1/statistics/overview?preset=7d&from=2026-04-01&to=2026-04-07", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(88))

	handler.GetOverview(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if readService.lastOverviewOrgID != 88 {
		t.Fatalf("orgID = %d, want 88", readService.lastOverviewOrgID)
	}
	if readService.lastOverviewFilter.Preset != "7d" || readService.lastOverviewFilter.From != "2026-04-01" || readService.lastOverviewFilter.To != "2026-04-07" {
		t.Fatalf("unexpected filter: %+v", readService.lastOverviewFilter)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			OrgID int64 `json:"org_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 || payload.Data.OrgID != 88 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestStatisticsHandlerListAssessmentEntryStatisticsParsesFiltersAndDefaultsPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	readService := &stubStatisticsReadService{
		listEntryResult: &domainStatistics.AssessmentEntryStatisticsList{},
	}
	handler := newStatisticsHandlerForTest(readService)
	c, rec := newStatisticsTestContext(http.MethodGet, "/api/v1/statistics/entries?clinician_id=123&status=active&preset=today", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(66))

	handler.ListAssessmentEntryStatistics(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if readService.lastListEntryOrgID != 66 {
		t.Fatalf("orgID = %d, want 66", readService.lastListEntryOrgID)
	}
	if readService.lastListEntryClinician == nil || *readService.lastListEntryClinician != 123 {
		t.Fatalf("clinicianID = %v, want 123", readService.lastListEntryClinician)
	}
	if readService.lastListEntryActiveOnly == nil || !*readService.lastListEntryActiveOnly {
		t.Fatalf("activeOnly = %v, want true", readService.lastListEntryActiveOnly)
	}
	if readService.lastListEntryPage != 1 || readService.lastListEntryPageSize != 20 {
		t.Fatalf("page = (%d,%d), want (1,20)", readService.lastListEntryPage, readService.lastListEntryPageSize)
	}
	if readService.lastListEntryFilter.Preset != "today" {
		t.Fatalf("unexpected filter: %+v", readService.lastListEntryFilter)
	}
}

func TestStatisticsHandlerListAssessmentEntryStatisticsRejectsInvalidClinicianID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := newStatisticsHandlerForTest(&stubStatisticsReadService{})
	c, rec := newStatisticsTestContext(http.MethodGet, "/api/v1/statistics/entries?clinician_id=bad", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(66))

	handler.ListAssessmentEntryStatistics(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestStatisticsHandlerGetCurrentClinicianOverviewUsesProtectedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	readService := &stubStatisticsReadService{
		currentClinicianResult: &domainStatistics.ClinicianStatistics{},
	}
	handler := newStatisticsHandlerForTest(readService)
	c, rec := newStatisticsTestContext(http.MethodGet, "/api/v1/statistics/clinicians/me/overview?preset=30d", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(91))
	c.Set(restmiddleware.UserIDKey, uint64(701))

	handler.GetCurrentClinicianOverview(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if readService.lastCurrentClinicianOrgID != 91 || readService.lastCurrentClinicianOperator != 701 {
		t.Fatalf("unexpected scope: org=%d user=%d", readService.lastCurrentClinicianOrgID, readService.lastCurrentClinicianOperator)
	}
	if readService.lastCurrentClinicianFilter.Preset != "30d" {
		t.Fatalf("unexpected filter: %+v", readService.lastCurrentClinicianFilter)
	}
}

func TestStatisticsHandlerBatchContentStatisticsRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := newStatisticsHandlerForTest(&stubStatisticsReadService{})
	c, rec := newStatisticsTestContext(http.MethodPost, "/api/v1/statistics/contents/batch", []byte(`{"items":`))
	c.Set(restmiddleware.OrgIDKey, uint64(91))

	handler.BatchContentStatistics(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestStatisticsHandlerBatchContentStatisticsUsesProtectedOrgAndTypedItems(t *testing.T) {
	gin.SetMode(gin.TestMode)

	readService := &stubStatisticsReadService{batchResult: &domainStatistics.ContentBatchStatisticsResponse{}}
	handler := newStatisticsHandlerForTest(readService)
	c, rec := newStatisticsTestContext(http.MethodPost, "/api/v1/statistics/contents/batch", []byte(`{"items":[{"type":"questionnaire","code":"Q-1"},{"type":"scale","code":"S-1"}]}`))
	c.Set(restmiddleware.OrgIDKey, uint64(91))
	snapshot := &authzApp.Snapshot{Roles: []string{"qs:admin"}}
	c.Request = c.Request.WithContext(authzApp.WithSnapshot(c.Request.Context(), snapshot))

	handler.BatchContentStatistics(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if readService.lastBatchOrgID != 91 || len(readService.lastBatchRefs) != 2 {
		t.Fatalf("unexpected batch scope: org=%d refs=%+v", readService.lastBatchOrgID, readService.lastBatchRefs)
	}
	if readService.lastBatchRefs[0].Type != domainStatistics.ContentTypeQuestionnaire || readService.lastBatchRefs[1].Type != domainStatistics.ContentTypeScale {
		t.Fatalf("unexpected typed refs: %+v", readService.lastBatchRefs)
	}
	if !readService.lastBatchAccess.Questionnaire || !readService.lastBatchAccess.Scale {
		t.Fatalf("unexpected content access: %+v", readService.lastBatchAccess)
	}
}

func TestStatisticsHandlerBatchContentStatisticsMapsIAMCapabilitiesByType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		snapshot          *authzApp.Snapshot
		wantQuestionnaire bool
		wantScale         bool
	}{
		{
			name: "questionnaire manager",
			snapshot: &authzApp.Snapshot{Permissions: []authzApp.Permission{
				{Resource: "qs:questionnaires", Action: "statistics"},
			}},
			wantQuestionnaire: true,
		},
		{
			name: "assessment model manager",
			snapshot: &authzApp.Snapshot{Permissions: []authzApp.Permission{
				{Resource: "qs:assessment_models", Action: "update"},
			}},
			wantScale: true,
		},
		{
			name: "both managers",
			snapshot: &authzApp.Snapshot{Permissions: []authzApp.Permission{
				{Resource: "qs:questionnaires", Action: "statistics"},
				{Resource: "qs:assessment_models", Action: "update"},
			}},
			wantQuestionnaire: true,
			wantScale:         true,
		},
		{name: "administrator", snapshot: &authzApp.Snapshot{Roles: []string{"qs:admin"}}, wantQuestionnaire: true, wantScale: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readService := &stubStatisticsReadService{batchResult: &domainStatistics.ContentBatchStatisticsResponse{}}
			handler := newStatisticsHandlerForTest(readService)
			c, rec := newStatisticsTestContext(http.MethodPost, "/api/v1/statistics/contents/batch", []byte(`{"items":[{"type":"questionnaire","code":"Q-1"},{"type":"scale","code":"S-1"}]}`))
			c.Set(restmiddleware.OrgIDKey, uint64(91))
			c.Request = c.Request.WithContext(authzApp.WithSnapshot(c.Request.Context(), tt.snapshot))

			handler.BatchContentStatistics(c)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if readService.lastBatchAccess.Questionnaire != tt.wantQuestionnaire || readService.lastBatchAccess.Scale != tt.wantScale {
				t.Fatalf("access = %+v, want questionnaire=%v scale=%v", readService.lastBatchAccess, tt.wantQuestionnaire, tt.wantScale)
			}
		})
	}
}

func TestStatisticsHandlerBatchContentStatisticsMapsPermissionFailureToForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)

	readService := &stubStatisticsReadService{
		batchErr: componentErrors.WithCode(code.ErrPermissionDenied, "both content capabilities are required"),
	}
	handler := newStatisticsHandlerForTest(readService)
	c, rec := newStatisticsTestContext(http.MethodPost, "/api/v1/statistics/contents/batch", []byte(`{"items":[{"type":"questionnaire","code":"Q-1"},{"type":"scale","code":"S-1"}]}`))
	c.Set(restmiddleware.OrgIDKey, uint64(91))
	c.Request = c.Request.WithContext(authzApp.WithSnapshot(c.Request.Context(), &authzApp.Snapshot{Roles: []string{"qs:admin"}}))

	handler.BatchContentStatistics(c)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

var _ statisticsApp.ReadService = (*stubStatisticsReadService)(nil)
