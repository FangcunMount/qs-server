package rest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appgovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	apprecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/gin-gonic/gin"
)

func TestAIExplanationAdministrationRoutesSeparateAuditReadsFromGovernanceWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &routeAIAdministrationStub{
		run: &appevaluation.ReviewRun{RunID: meta.ID(9), Status: domainevaluation.StatusAwaitingReview},
		runV2: &domainevaluation.PromptEvaluationEvidenceV2{
			SchemaVersion: domainevaluation.PromptEvaluationEvidenceSchemaVersionV2, RunID: meta.ID(10), Status: domainevaluation.EvidenceStatusCollecting,
			Audit:           domainevaluation.EvidenceRunAudit{OrganizationID: 12, RequestedBy: "user:34", RequestReason: "evaluate", CreatedAt: time.Now()},
			ExecutionPolicy: domainevaluation.CurrentEvaluationExecutionPolicy(), GatePolicy: domainevaluation.CurrentReleaseGatePolicy(),
		},
	}
	router := newRouterWithBudgets(Deps{Interpretation: InterpretationDeps{AIExplanationAdministration: service}})

	auditEngine := gin.New()
	auditEngine.Use(aiRouteSnapshotMiddleware(false))
	router.registerInterpretationInternalRoutes(auditEngine.Group("/internal/v1"))
	read := httptest.NewRecorder()
	auditEngine.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9", nil))
	if read.Code != http.StatusOK || service.findCalls != 1 {
		t.Fatalf("audit read status/calls = %d/%d body=%s", read.Code, service.findCalls, read.Body.String())
	}
	list := httptest.NewRecorder()
	auditEngine.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/prompt-evaluations?status=awaiting_review", nil))
	if list.Code != http.StatusOK || service.listCalls != 1 {
		t.Fatalf("audit list status/calls = %d/%d body=%s", list.Code, service.listCalls, list.Body.String())
	}
	profiles := httptest.NewRecorder()
	auditEngine.ServeHTTP(profiles, httptest.NewRequest(http.MethodGet, "/internal/v1/interpretation/ai-explanation/profiles?status=draft", nil))
	if profiles.Code != http.StatusOK || service.profileListCalls != 1 {
		t.Fatalf("audit Profile list status/calls = %d/%d body=%s", profiles.Code, service.profileListCalls, profiles.Body.String())
	}
	write := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/reviews", bytes.NewBufferString(`{"case_id":"PROMPT-EVAL-001","attempt":1,"role":"assessment_semantics","decision":"approve","reason":"reviewed"}`))
	request.Header.Set("Content-Type", "application/json")
	auditEngine.ServeHTTP(write, request)
	if write.Code != http.StatusNotFound || service.reviewCalls != 0 {
		t.Fatalf("audit-only write status/calls = %d/%d body=%s", write.Code, service.reviewCalls, write.Body.String())
	}
	auditStart := httptest.NewRecorder()
	auditStartRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":70,"reason":"must require governance"}`))
	auditStartRequest.Header.Set("Content-Type", "application/json")
	auditEngine.ServeHTTP(auditStart, auditStartRequest)
	if auditStart.Code != http.StatusNotFound || service.startCalls != 0 {
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
	if adminWrite.Code != http.StatusNotFound || service.reviewCalls != 0 {
		t.Fatalf("admin write status/calls = %d/%d body=%s", adminWrite.Code, service.reviewCalls, adminWrite.Body.String())
	}
	start := httptest.NewRecorder()
	startRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":70,"reason":"evaluate frozen release"}`))
	startRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(start, startRequest)
	if start.Code != http.StatusNotFound || service.startCalls != 0 {
		t.Fatalf("admin start status/calls = %d/%d body=%s", start.Code, service.startCalls, start.Body.String())
	}
	recover := httptest.NewRecorder()
	recoverRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/recover", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":68,"reason":"recover expired execution"}`))
	recoverRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(recover, recoverRequest)
	if recover.Code != http.StatusNotFound || service.recoverCalls != 0 {
		t.Fatalf("admin recover status/calls = %d/%d body=%s", recover.Code, service.recoverCalls, recover.Body.String())
	}
	cancel := httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/cancel", bytes.NewBufferString(`{"reason":"stop before dispatch"}`))
	cancelRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(cancel, cancelRequest)
	if cancel.Code != http.StatusNotFound || service.cancelCalls != 0 {
		t.Fatalf("admin cancel status/calls = %d/%d body=%s", cancel.Code, service.cancelCalls, cancel.Body.String())
	}
	retry := httptest.NewRecorder()
	retryRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/interpretation/ai-explanation/generations/900/retry", bytes.NewBufferString(`{"expected_attempt":1,"request_id":"retry-request-1","confirm":true,"expected_provider_invocations":1,"reason":"manual recovery"}`))
	retryRequest.Header.Set("Content-Type", "application/json")
	adminEngine.ServeHTTP(retry, retryRequest)
	if retry.Code != http.StatusAccepted || service.retryCalls != 1 {
		t.Fatalf("admin participant retry status/calls = %d/%d body=%s", retry.Code, service.retryCalls, retry.Body.String())
	}

	v2AuditEngine := gin.New()
	v2AuditEngine.Use(aiRouteSnapshotMiddleware(false))
	router.registerInterpretationInternalV2Routes(v2AuditEngine.Group("/internal/v2"))
	v2Read := httptest.NewRecorder()
	v2AuditEngine.ServeHTTP(v2Read, httptest.NewRequest(http.MethodGet, "/internal/v2/interpretation/ai-explanation/prompt-evaluations/10", nil))
	if v2Read.Code != http.StatusOK || service.findV2Calls != 1 {
		t.Fatalf("v2 audit read status/calls = %d/%d body=%s", v2Read.Code, service.findV2Calls, v2Read.Body.String())
	}
	v2CandidateRead := httptest.NewRecorder()
	v2AuditEngine.ServeHTTP(v2CandidateRead, httptest.NewRequest(http.MethodGet, "/internal/v2/interpretation/ai-explanation/prompt-evaluations/10/candidates/candidate:1", nil))
	if v2CandidateRead.Code != http.StatusNotFound || service.findV2Calls != 2 {
		t.Fatalf("v2 candidate audit read status/calls = %d/%d body=%s", v2CandidateRead.Code, service.findV2Calls, v2CandidateRead.Body.String())
	}
	v2AuditStart := httptest.NewRecorder()
	v2AuditStartRequest := httptest.NewRequest(http.MethodPost, "/internal/v2/interpretation/ai-explanation/prompt-evaluations", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":140,"reason":"evaluate frozen v2 release"}`))
	v2AuditStartRequest.Header.Set("Content-Type", "application/json")
	v2AuditEngine.ServeHTTP(v2AuditStart, v2AuditStartRequest)
	if v2AuditStart.Code != http.StatusForbidden || service.startV2Calls != 0 {
		t.Fatalf("v2 audit start status/calls = %d/%d body=%s", v2AuditStart.Code, service.startV2Calls, v2AuditStart.Body.String())
	}
	v2AuditRecheck := httptest.NewRecorder()
	v2AuditRecheckRequest := httptest.NewRequest(http.MethodPost, "/internal/v2/interpretation/ai-explanation/legacy-prompt-evaluations/9/attempts/PROMPT-EVAL-002/3/rechecks", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":2,"reason":"verify one failed record"}`))
	v2AuditRecheckRequest.Header.Set("Content-Type", "application/json")
	v2AuditEngine.ServeHTTP(v2AuditRecheck, v2AuditRecheckRequest)
	if v2AuditRecheck.Code != http.StatusForbidden || service.recheckCalls != 0 {
		t.Fatalf("v2 audit recheck status/calls = %d/%d body=%s", v2AuditRecheck.Code, service.recheckCalls, v2AuditRecheck.Body.String())
	}
	v2AuditBatch := httptest.NewRecorder()
	v2AuditBatchRequest := httptest.NewRequest(http.MethodPost, "/internal/v2/interpretation/ai-explanation/prompt-evaluations/10/reviews/batch", bytes.NewBufferString(`{"role":"assessment_semantics","reviews":[{"candidate_id":"candidate:1","decision":"approve","reason":"reviewed"}]}`))
	v2AuditBatchRequest.Header.Set("Content-Type", "application/json")
	v2AuditEngine.ServeHTTP(v2AuditBatch, v2AuditBatchRequest)
	if v2AuditBatch.Code != http.StatusForbidden || service.batchReviewV2Calls != 0 {
		t.Fatalf("v2 audit batch-review status/calls = %d/%d body=%s", v2AuditBatch.Code, service.batchReviewV2Calls, v2AuditBatch.Body.String())
	}

	v2AdminEngine := gin.New()
	v2AdminEngine.Use(aiRouteSnapshotMiddleware(true))
	router.registerInterpretationInternalV2Routes(v2AdminEngine.Group("/internal/v2"))
	v2Start := httptest.NewRecorder()
	v2StartRequest := httptest.NewRequest(http.MethodPost, "/internal/v2/interpretation/ai-explanation/prompt-evaluations", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":140,"reason":"evaluate frozen v2 release"}`))
	v2StartRequest.Header.Set("Content-Type", "application/json")
	v2AdminEngine.ServeHTTP(v2Start, v2StartRequest)
	if v2Start.Code != http.StatusAccepted || service.startV2Calls != 1 {
		t.Fatalf("v2 admin start status/calls = %d/%d body=%s", v2Start.Code, service.startV2Calls, v2Start.Body.String())
	}
	v2Recheck := httptest.NewRecorder()
	v2RecheckRequest := httptest.NewRequest(http.MethodPost, "/internal/v2/interpretation/ai-explanation/legacy-prompt-evaluations/9/attempts/PROMPT-EVAL-002/3/rechecks", bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":2,"reason":"verify one failed record"}`))
	v2RecheckRequest.Header.Set("Content-Type", "application/json")
	v2AdminEngine.ServeHTTP(v2Recheck, v2RecheckRequest)
	if v2Recheck.Code != http.StatusAccepted || service.recheckCalls != 1 {
		t.Fatalf("v2 admin recheck status/calls = %d/%d body=%s", v2Recheck.Code, service.recheckCalls, v2Recheck.Body.String())
	}
	v2Batch := httptest.NewRecorder()
	v2BatchRequest := httptest.NewRequest(http.MethodPost, "/internal/v2/interpretation/ai-explanation/prompt-evaluations/10/reviews/batch", bytes.NewBufferString(`{"role":"assessment_semantics","reviews":[{"candidate_id":"candidate:1","decision":"approve","reason":"reviewed"}]}`))
	v2BatchRequest.Header.Set("Content-Type", "application/json")
	v2AdminEngine.ServeHTTP(v2Batch, v2BatchRequest)
	if v2Batch.Code != http.StatusOK || service.batchReviewV2Calls != 1 {
		t.Fatalf("v2 admin batch-review status/calls = %d/%d body=%s", v2Batch.Code, service.batchReviewV2Calls, v2Batch.Body.String())
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
	runV2                    *domainevaluation.PromptEvaluationEvidenceV2
	findCalls                int
	reviewCalls              int
	startCalls               int
	recoverCalls             int
	cancelCalls              int
	capacityCalls            int
	participantCapacityCalls int
	retryCalls               int
	listCalls                int
	profileListCalls         int
	findV2Calls              int
	startV2Calls             int
	recheckCalls             int
	batchReviewV2Calls       int
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
func (s *routeAIAdministrationStub) StartEvaluationV2(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.StartEvaluationV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.startV2Calls++
	return s.runV2, nil
}
func (s *routeAIAdministrationStub) FindEvaluationV2(context.Context, aiexplanationadministration.Actor, meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.findV2Calls++
	return s.runV2, nil
}
func (s *routeAIAdministrationStub) RecordReviewV2(context.Context, aiexplanationadministration.Actor, meta.ID, aiexplanationadministration.ReviewV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.runV2, nil
}
func (s *routeAIAdministrationStub) RecordReviewsV2(context.Context, aiexplanationadministration.Actor, meta.ID, aiexplanationadministration.ReviewV2BatchCommand) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	s.batchReviewV2Calls++
	return s.runV2, nil
}
func (s *routeAIAdministrationStub) FinalizeEvaluationV2(context.Context, aiexplanationadministration.Actor, meta.ID, string) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.runV2, nil
}
func (s *routeAIAdministrationStub) ResolveResultUnknownV2(context.Context, aiexplanationadministration.Actor, meta.ID, aiexplanationadministration.ResolveResultUnknownV2Command) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	return s.runV2, nil
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

func (s *routeAIAdministrationStub) ListEvaluations(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.EvaluationListQuery) (*appevaluation.ReviewRunPage, error) {
	s.listCalls++
	return &appevaluation.ReviewRunPage{Items: []*appevaluation.ReviewRun{s.run}}, nil
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
func (s *routeAIAdministrationStub) StartEvaluationRecheck(context.Context, aiexplanationadministration.Actor, meta.ID, string, int, aiexplanationadministration.StartEvaluationRecheckCommand) (*domainevaluation.PromptEvaluationRecheck, error) {
	s.recheckCalls++
	return nil, nil
}
func (*routeAIAdministrationStub) ListEvaluationRechecks(context.Context, aiexplanationadministration.Actor, meta.ID, string, int, int) ([]*domainevaluation.PromptEvaluationRecheck, error) {
	return nil, nil
}
func (*routeAIAdministrationStub) FindEvaluationRecheck(context.Context, aiexplanationadministration.Actor, meta.ID, string, int, meta.ID) (*domainevaluation.PromptEvaluationRecheck, error) {
	return nil, nil
}
func (s *routeAIAdministrationStub) ListProfiles(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.ProfileListQuery) (*appgovernance.ProfilePage, error) {
	s.profileListCalls++
	return &appgovernance.ProfilePage{}, nil
}
func (*routeAIAdministrationStub) FindProfile(context.Context, aiexplanationadministration.Actor, string, string) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
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
