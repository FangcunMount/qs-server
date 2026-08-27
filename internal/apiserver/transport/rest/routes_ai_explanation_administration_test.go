package rest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	apprecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/gin-gonic/gin"
)

func TestAIExplanationAdministrationRoutesSeparateAuditReadsFromGovernanceWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &routeAIAdministrationStub{run: &appevaluation.ReviewRun{RunID: meta.ID(9), Status: domainevaluation.StatusAwaitingReview}}
	router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIExplanationAdministration: service}})

	auditEngine := gin.New()
	auditEngine.Use(aiRouteSnapshotMiddleware(false))
	router.registerInterpretationInternalRoutes(auditEngine.Group("/internal/v1"))
	read := httptest.NewRecorder()
	auditEngine.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9", nil))
	if read.Code != http.StatusOK || service.findCalls != 1 {
		t.Fatalf("audit read status/calls = %d/%d body=%s", read.Code, service.findCalls, read.Body.String())
	}
	write := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/reviews", bytes.NewBufferString(`{"case_id":"PROMPT-EVAL-001","attempt":1,"role":"assessment_semantics","decision":"approve","reason":"reviewed"}`))
	request.Header.Set("Content-Type", "application/json")
	auditEngine.ServeHTTP(write, request)
	if write.Code != http.StatusForbidden || service.reviewCalls != 0 {
		t.Fatalf("audit-only write status/calls = %d/%d body=%s", write.Code, service.reviewCalls, write.Body.String())
	}
	auditStart := httptest.NewRecorder()
	auditStartRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":70,"reason":"must require governance"}`))
	auditStartRequest.Header.Set("Content-Type", "application/json")
	auditEngine.ServeHTTP(auditStart, auditStartRequest)
	if auditStart.Code != http.StatusForbidden || service.startCalls != 0 {
		t.Fatalf("audit-only start status/calls = %d/%d body=%s", auditStart.Code, service.startCalls, auditStart.Body.String())
	}
	auditCapacity := httptest.NewRecorder()
	auditEngine.ServeHTTP(auditCapacity, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/prompt-evaluation-capacity", nil))
	if auditCapacity.Code != http.StatusForbidden || service.capacityCalls != 0 {
		t.Fatalf("audit-only capacity status/calls = %d/%d body=%s", auditCapacity.Code, service.capacityCalls, auditCapacity.Body.String())
	}
	auditParticipantCapacity := httptest.NewRecorder()
	auditEngine.ServeHTTP(auditParticipantCapacity, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/participant-capacity", nil))
	if auditParticipantCapacity.Code != http.StatusForbidden || service.participantCapacityCalls != 0 {
		t.Fatalf("audit-only participant capacity status/calls = %d/%d body=%s", auditParticipantCapacity.Code, service.participantCapacityCalls, auditParticipantCapacity.Body.String())
	}
	auditRetry := httptest.NewRecorder()
	auditRetryRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/generations/900/retry", bytes.NewBufferString(`{"expected_attempt":1,"request_id":"retry-request-1","confirm":true,"expected_provider_invocations":1,"reason":"manual recovery"}`))
	auditRetryRequest.Header.Set("Content-Type", "application/json")
	auditEngine.ServeHTTP(auditRetry, auditRetryRequest)
	if auditRetry.Code != http.StatusForbidden || service.retryCalls != 0 {
		t.Fatalf("audit-only participant retry status/calls = %d/%d body=%s", auditRetry.Code, service.retryCalls, auditRetry.Body.String())
	}

	adminEngine := gin.New()
	adminEngine.Use(aiRouteSnapshotMiddleware(true))
	router.registerInterpretationInternalRoutes(adminEngine.Group("/internal/v1"))
	capacity := httptest.NewRecorder()
	adminEngine.ServeHTTP(capacity, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/prompt-evaluation-capacity", nil))
	if capacity.Code != http.StatusOK || service.capacityCalls != 1 {
		t.Fatalf("admin capacity status/calls = %d/%d body=%s", capacity.Code, service.capacityCalls, capacity.Body.String())
	}
	participantCapacity := httptest.NewRecorder()
	adminEngine.ServeHTTP(participantCapacity, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/participant-capacity", nil))
	if participantCapacity.Code != http.StatusOK || service.participantCapacityCalls != 1 {
		t.Fatalf("admin participant capacity status/calls = %d/%d body=%s", participantCapacity.Code, service.participantCapacityCalls, participantCapacity.Body.String())
	}
	adminWrite := httptest.NewRecorder()
	adminRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/reviews", bytes.NewBufferString(`{"case_id":"PROMPT-EVAL-001","attempt":1,"role":"assessment_semantics","decision":"approve","reason":"reviewed"}`))
	adminRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(adminWrite, adminRequest)
	if adminWrite.Code != http.StatusOK || service.reviewCalls != 1 {
		t.Fatalf("admin write status/calls = %d/%d body=%s", adminWrite.Code, service.reviewCalls, adminWrite.Body.String())
	}
	start := httptest.NewRecorder()
	startRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":70,"reason":"evaluate frozen release"}`))
	startRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(start, startRequest)
	if start.Code != http.StatusAccepted || service.startCalls != 1 {
		t.Fatalf("admin start status/calls = %d/%d body=%s", start.Code, service.startCalls, start.Body.String())
	}
	recover := httptest.NewRecorder()
	recoverRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/recover", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":68,"reason":"recover expired execution"}`))
	recoverRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(recover, recoverRequest)
	if recover.Code != http.StatusAccepted || service.recoverCalls != 1 {
		t.Fatalf("admin recover status/calls = %d/%d body=%s", recover.Code, service.recoverCalls, recover.Body.String())
	}
	cancel := httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/cancel", bytes.NewBufferString(`{"reason":"stop before dispatch"}`))
	cancelRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(cancel, cancelRequest)
	if cancel.Code != http.StatusOK || service.cancelCalls != 1 {
		t.Fatalf("admin cancel status/calls = %d/%d body=%s", cancel.Code, service.cancelCalls, cancel.Body.String())
	}
	retry := httptest.NewRecorder()
	retryRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/generations/900/retry", bytes.NewBufferString(`{"expected_attempt":1,"request_id":"retry-request-1","confirm":true,"expected_provider_invocations":1,"reason":"manual recovery"}`))
	retryRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(retry, retryRequest)
	if retry.Code != http.StatusAccepted || service.retryCalls != 1 {
		t.Fatalf("admin participant retry status/calls = %d/%d body=%s", retry.Code, service.retryCalls, retry.Body.String())
	}
}

func aiRouteSnapshotMiddleware(admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		permissions := []authz.Permission{{Resource: "qs:evaluation:collection:reports", Action: "audit", Mode: authz.AuthorizationModeUnconditional}}
		if admin {
			permissions = []authz.Permission{{Resource: "qs:*:*:*", Action: "*", Mode: authz.AuthorizationModeUnconditional}}
		}
		snapshot := &authz.Snapshot{Permissions: permissions}
		c.Set(restmiddleware.AuthzSnapshotKey, snapshot)
		c.Set(restmiddleware.OrgIDKey, uint64(12))
		c.Set(restmiddleware.UserIDKey, uint64(34))
		c.Request = c.Request.WithContext(authz.WithSnapshot(c.Request.Context(), snapshot))
		c.Next()
	}
}

type routeAIAdministrationStub struct {
	run                      *appevaluation.ReviewRun
	findCalls                int
	reviewCalls              int
	startCalls               int
	recoverCalls             int
	cancelCalls              int
	capacityCalls            int
	participantCapacityCalls int
	retryCalls               int
}

func (s *routeAIAdministrationStub) FindEvaluationCapacity(context.Context, aiexplanationadministration.Actor) (*aiexplanationadministration.EvaluationCapacity, error) {
	s.capacityCalls++
	return &aiexplanationadministration.EvaluationCapacity{OrgID: 12}, nil
}

func (s *routeAIAdministrationStub) FindParticipantCapacity(context.Context, aiexplanationadministration.Actor) (*aiexplanationadministration.ParticipantCapacity, error) {
	s.participantCapacityCalls++
	return &aiexplanationadministration.ParticipantCapacity{OrgID: 12}, nil
}

func (s *routeAIAdministrationStub) StartEvaluation(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.StartEvaluationCommand) (*appevaluation.ReviewRun, error) {
	s.startCalls++
	return s.run, nil
}
func (s *routeAIAdministrationStub) RecoverEvaluation(context.Context, aiexplanationadministration.Actor, meta.ID, aiexplanationadministration.RecoverEvaluationCommand) (*appevaluation.ReviewRun, error) {
	s.recoverCalls++
	return s.run, nil
}
func (s *routeAIAdministrationStub) CancelEvaluation(context.Context, aiexplanationadministration.Actor, meta.ID, string) (*appevaluation.ReviewRun, error) {
	s.cancelCalls++
	return s.run, nil
}
func (s *routeAIAdministrationStub) RetryParticipantGeneration(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.RetryParticipantGenerationCommand) (*apprecovery.Result, error) {
	s.retryCalls++
	return nil, nil
}

func (s *routeAIAdministrationStub) FindEvaluation(context.Context, aiexplanationadministration.Actor, meta.ID) (*appevaluation.ReviewRun, error) {
	s.findCalls++
	return s.run, nil
}
func (s *routeAIAdministrationStub) RecordReview(context.Context, aiexplanationadministration.Actor, meta.ID, aiexplanationadministration.ReviewCommand) (*appevaluation.ReviewRun, error) {
	s.reviewCalls++
	return s.run, nil
}
func (s *routeAIAdministrationStub) FinalizeEvaluation(context.Context, aiexplanationadministration.Actor, meta.ID, string) (*appevaluation.ReviewRun, error) {
	return s.run, nil
}
func (*routeAIAdministrationStub) CreateProfileDraft(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.CreateProfileDraftCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
func (*routeAIAdministrationStub) PublishProfile(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.PublishProfileCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
func (*routeAIAdministrationStub) DisableProfile(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.DisableProfileCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
