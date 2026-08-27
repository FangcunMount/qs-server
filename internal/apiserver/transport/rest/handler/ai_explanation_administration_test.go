package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aiexplanationadministration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/administration"
	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	apprecovery "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/recovery"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/gin-gonic/gin"
)

func TestAIExplanationAdministrationReviewUsesProtectedActorAndOmitsRawOutputFromSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &aiAdministrationServiceStub{run: administrationReviewRun()}
	handler := NewAIExplanationAdministrationHandler(service)
	body := bytes.NewBufferString(`{"case_id":"PROMPT-EVAL-001","attempt":1,"role":"assessment_semantics","decision":"approve","reason":"facts match"}`)
	ctx, recorder := aiAdministrationContext(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/reviews", body)
	ctx.Params = gin.Params{{Key: "run_id", Value: "9"}}

	handler.RecordReview(ctx)
	if recorder.Code != http.StatusOK || service.lastActor != (aiexplanationadministration.Actor{OrgID: 12, OperatorUserID: 34}) || service.lastReview.CaseID != "PROMPT-EVAL-001" {
		t.Fatalf("response=%d actor=%#v review=%#v body=%s", recorder.Code, service.lastActor, service.lastReview, recorder.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload.Data)
	if bytes.Contains(encoded, []byte("raw_provider_output")) || bytes.Contains(encoded, []byte("assessment_input")) {
		t.Fatalf("evaluation summary leaked detailed evidence: %s", encoded)
	}
}

func TestAIExplanationAdministrationAttemptReturnsOnlyRequestedEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &aiAdministrationServiceStub{run: administrationReviewRun()}
	handler := NewAIExplanationAdministrationHandler(service)
	ctx, recorder := aiAdministrationContext(http.MethodGet, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/attempts/PROMPT-EVAL-001/1", nil)
	ctx.Params = gin.Params{{Key: "run_id", Value: "9"}, {Key: "case_id", Value: "PROMPT-EVAL-001"}, {Key: "attempt", Value: "1"}}

	handler.FindAttempt(ctx)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"raw_provider_output":"{\"summary\":\"synthetic\"}"`)) || !bytes.Contains(recorder.Body.Bytes(), []byte(`"assessment_input":{"context":{},"facts":{}}`)) {
		t.Fatalf("response=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAIExplanationAdministrationStartReturnsAcceptedAndTrustedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &aiAdministrationServiceStub{run: administrationReviewRun()}
	handler := NewAIExplanationAdministrationHandler(service)
	body := bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":70,"reason":"evaluate frozen v1 release"}`)
	ctx, recorder := aiAdministrationContext(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations", body)

	handler.StartEvaluation(ctx)
	if recorder.Code != http.StatusAccepted || service.lastActor != (aiexplanationadministration.Actor{OrgID: 12, OperatorUserID: 34}) || !service.lastStart.Confirm || service.lastStart.ExpectedProviderInvocations != 70 {
		t.Fatalf("response=%d actor=%#v start=%#v body=%s", recorder.Code, service.lastActor, service.lastStart, recorder.Body.String())
	}
}

func TestAIExplanationAdministrationCapacityReturnsTrustedOrganizationLedger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	service := &aiAdministrationServiceStub{capacity: &aiexplanationadministration.EvaluationCapacity{
		OrgID: 12, BudgetDay: day, MaxActiveRunsPerOrg: 1, ProviderInvocationsPerStart: 70,
		DailyProviderInvocationLimit: 140, ReservedProviderInvocations: 70,
		RemainingProviderInvocations: 70, AvailableFullRunStarts: 1,
		Reservations: []aiexplanationadministration.EvaluationCapacityReservation{{
			RunID: meta.ID(700), RequestedBy: "user:34", ProviderInvocations: 70, ReservedAt: day.Add(time.Hour),
		}},
	}}
	handler := NewAIExplanationAdministrationHandler(service)
	ctx, recorder := aiAdministrationContext(http.MethodGet, "/internal/v1/interpretation/ai-explanation/prompt-evaluation-capacity", nil)

	handler.FindEvaluationCapacity(ctx)
	if recorder.Code != http.StatusOK || service.lastActor != (aiexplanationadministration.Actor{OrgID: 12, OperatorUserID: 34}) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"daily_provider_invocation_limit":140`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"run_id":"700"`)) {
		t.Fatalf("response=%d actor=%#v body=%s", recorder.Code, service.lastActor, recorder.Body.String())
	}
}

func TestAIExplanationAdministrationParticipantCapacityReturnsTrustedOrganizationLedger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	service := &aiAdministrationServiceStub{participantCapacity: &aiexplanationadministration.ParticipantCapacity{
		OrgID: 12, BudgetDay: day, ProviderInvocationsPerGeneration: 1,
		DailyProviderInvocationLimitPerOrg: 500, DailyProviderInvocationLimitPerUser: 5,
		DailyProviderInvocationLimitPerAssessment: 3, ReservedProviderInvocations: 2, RedactedProviderInvocations: 1,
		MaxActiveProviderExecutionsPerOrg: 10, MaxActiveProviderExecutionsPerUser: 2,
		MaxActiveProviderExecutionsPerAssessment: 1, RemainingOrgProviderInvocations: 499,
		ActiveProviderExecutions: 1, RemainingOrgActiveProviderExecutions: 9,
		Reservations: []aiexplanationadministration.ParticipantCapacityReservation{{
			ReservationID: "ai-explanation:900:attempt:1", GenerationID: meta.ID(900), Attempt: 1,
			Origin: retrygovernance.AttemptOriginInitial, UserID: "user-34", AssessmentID: meta.ID(501), ProviderInvocations: 1,
			ReservedAt: day.Add(time.Hour),
		}},
		ActiveReservations: []aiexplanationadministration.ParticipantActiveCapacityReservation{{
			GenerationID: meta.ID(900), RunID: meta.ID(901), UserID: "user-34", AssessmentID: meta.ID(501),
			AcquiredAt: day.Add(2 * time.Hour),
		}},
	}}
	handler := NewAIExplanationAdministrationHandler(service)
	ctx, recorder := aiAdministrationContext(http.MethodGet, "/internal/v1/interpretation/ai-explanation/participant-capacity", nil)

	handler.FindParticipantCapacity(ctx)
	if recorder.Code != http.StatusOK || service.lastActor != (aiexplanationadministration.Actor{OrgID: 12, OperatorUserID: 34}) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"daily_provider_invocation_limit_per_user":5`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"redacted_provider_invocations":1`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"generation_id":"900"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"assessment_id":"501"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"max_active_provider_executions_per_org":10`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"run_id":"901"`)) {
		t.Fatalf("response=%d actor=%#v body=%s", recorder.Code, service.lastActor, recorder.Body.String())
	}
}

func TestAIExplanationAdministrationParticipantRetryUsesTrustedActorAndCostConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &aiAdministrationServiceStub{}
	handler := NewAIExplanationAdministrationHandler(service)
	body := bytes.NewBufferString(`{"expected_attempt":1,"request_id":"retry-request-1","confirm":true,"expected_provider_invocations":1,"accept_result_unknown_risk":true,"reason":"manual recovery"}`)
	ctx, recorder := aiAdministrationContext(http.MethodPost, "/internal/v1/interpretation/ai-explanation/generations/900/retry", body)
	ctx.Params = gin.Params{{Key: "generation_id", Value: "900"}}

	handler.RetryParticipantGeneration(ctx)
	if recorder.Code != http.StatusAccepted || service.lastActor != (aiexplanationadministration.Actor{OrgID: 12, OperatorUserID: 34}) ||
		service.lastRetry.GenerationID != meta.ID(900) || service.lastRetry.ExpectedAttempt != 1 || service.lastRetry.RequestID != "retry-request-1" ||
		!service.lastRetry.Confirm || service.lastRetry.ExpectedProviderInvocations != 1 || !service.lastRetry.AcceptResultUnknownRisk {
		t.Fatalf("response=%d actor=%#v retry=%#v body=%s", recorder.Code, service.lastActor, service.lastRetry, recorder.Body.String())
	}
}

func TestAIExplanationAdministrationRecoverReturnsAcceptedWithRemainingCostConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &aiAdministrationServiceStub{run: administrationReviewRun()}
	handler := NewAIExplanationAdministrationHandler(service)
	body := bytes.NewBufferString(`{"confirm":true,"expected_provider_invocations":68,"reason":"recover expired dispatch"}`)
	ctx, recorder := aiAdministrationContext(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/recover", body)
	ctx.Params = gin.Params{{Key: "run_id", Value: "9"}}

	handler.RecoverEvaluation(ctx)
	if recorder.Code != http.StatusAccepted || service.lastActor != (aiexplanationadministration.Actor{OrgID: 12, OperatorUserID: 34}) ||
		service.lastRunID != meta.ID(9) || !service.lastRecover.Confirm || service.lastRecover.ExpectedProviderInvocations != 68 || service.lastRecover.Reason != "recover expired dispatch" {
		t.Fatalf("response=%d actor=%#v run=%s recover=%#v body=%s", recorder.Code, service.lastActor, service.lastRunID, service.lastRecover, recorder.Body.String())
	}
}

func TestAIExplanationAdministrationCancelUsesTrustedActorAndReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &aiAdministrationServiceStub{run: administrationReviewRun()}
	handler := NewAIExplanationAdministrationHandler(service)
	body := bytes.NewBufferString(`{"reason":"stop before provider dispatch"}`)
	ctx, recorder := aiAdministrationContext(http.MethodPost, "/internal/v1/interpretation/ai-explanation/prompt-evaluations/9/cancel", body)
	ctx.Params = gin.Params{{Key: "run_id", Value: "9"}}

	handler.CancelEvaluation(ctx)
	if recorder.Code != http.StatusOK || service.lastActor != (aiexplanationadministration.Actor{OrgID: 12, OperatorUserID: 34}) ||
		service.lastRunID != meta.ID(9) || service.lastCancelReason != "stop before provider dispatch" {
		t.Fatalf("response=%d actor=%#v run=%s reason=%q body=%s", recorder.Code, service.lastActor, service.lastRunID, service.lastCancelReason, recorder.Body.String())
	}
}

func administrationReviewRun() *appevaluation.ReviewRun {
	return &appevaluation.ReviewRun{
		RunID: meta.ID(9), Version: 37, Status: domainevaluation.StatusAwaitingReview, CanReview: true,
		Progress: appevaluation.ReviewProgress{GenerationAttempts: 35, RequiredReviews: 70, MissingReviews: 70},
		Attempts: []appevaluation.ReviewAttempt{{
			CaseID: "PROMPT-EVAL-001", Attempt: 1,
			AssessmentInput: []byte(`{"context":{},"facts":{}}`), RawProviderOutput: []byte(`{"summary":"synthetic"}`),
			NormalizedOutput: []byte(`{"summary":"synthetic"}`), MissingRoles: []domainevaluation.ReviewRole{domainevaluation.ReviewRoleAssessmentSemantics, domainevaluation.ReviewRoleSafetyProduct},
		}},
	}
}

func aiAdministrationContext(method, path string, body *bytes.Buffer) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if body == nil {
		ctx.Request = httptest.NewRequest(method, path, nil)
	} else {
		ctx.Request = httptest.NewRequest(method, path, body)
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set(restmiddleware.OrgIDKey, uint64(12))
	ctx.Set(restmiddleware.UserIDKey, uint64(34))
	return ctx, recorder
}

type aiAdministrationServiceStub struct {
	run                 *appevaluation.ReviewRun
	capacity            *aiexplanationadministration.EvaluationCapacity
	participantCapacity *aiexplanationadministration.ParticipantCapacity
	lastActor           aiexplanationadministration.Actor
	lastReview          aiexplanationadministration.ReviewCommand
	lastStart           aiexplanationadministration.StartEvaluationCommand
	lastRecover         aiexplanationadministration.RecoverEvaluationCommand
	lastRetry           aiexplanationadministration.RetryParticipantGenerationCommand
	lastRunID           meta.ID
	lastCancelReason    string
}

func (s *aiAdministrationServiceStub) FindEvaluationCapacity(_ context.Context, actor aiexplanationadministration.Actor) (*aiexplanationadministration.EvaluationCapacity, error) {
	s.lastActor = actor
	return s.capacity, nil
}

func (s *aiAdministrationServiceStub) FindParticipantCapacity(_ context.Context, actor aiexplanationadministration.Actor) (*aiexplanationadministration.ParticipantCapacity, error) {
	s.lastActor = actor
	return s.participantCapacity, nil
}

func (s *aiAdministrationServiceStub) StartEvaluation(_ context.Context, actor aiexplanationadministration.Actor, command aiexplanationadministration.StartEvaluationCommand) (*appevaluation.ReviewRun, error) {
	s.lastActor, s.lastStart = actor, command
	return s.run, nil
}
func (s *aiAdministrationServiceStub) RecoverEvaluation(_ context.Context, actor aiexplanationadministration.Actor, runID meta.ID, command aiexplanationadministration.RecoverEvaluationCommand) (*appevaluation.ReviewRun, error) {
	s.lastActor, s.lastRunID, s.lastRecover = actor, runID, command
	return s.run, nil
}
func (s *aiAdministrationServiceStub) CancelEvaluation(_ context.Context, actor aiexplanationadministration.Actor, runID meta.ID, reason string) (*appevaluation.ReviewRun, error) {
	s.lastActor, s.lastRunID, s.lastCancelReason = actor, runID, reason
	return s.run, nil
}
func (s *aiAdministrationServiceStub) RetryParticipantGeneration(_ context.Context, actor aiexplanationadministration.Actor, command aiexplanationadministration.RetryParticipantGenerationCommand) (*apprecovery.Result, error) {
	s.lastActor, s.lastRetry = actor, command
	return nil, nil
}

func (s *aiAdministrationServiceStub) FindEvaluation(_ context.Context, actor aiexplanationadministration.Actor, _ meta.ID) (*appevaluation.ReviewRun, error) {
	s.lastActor = actor
	return s.run, nil
}
func (s *aiAdministrationServiceStub) RecordReview(_ context.Context, actor aiexplanationadministration.Actor, _ meta.ID, command aiexplanationadministration.ReviewCommand) (*appevaluation.ReviewRun, error) {
	s.lastActor, s.lastReview = actor, command
	return s.run, nil
}
func (s *aiAdministrationServiceStub) FinalizeEvaluation(context.Context, aiexplanationadministration.Actor, meta.ID, string) (*appevaluation.ReviewRun, error) {
	return s.run, nil
}
func (*aiAdministrationServiceStub) CreateProfileDraft(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.CreateProfileDraftCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
func (*aiAdministrationServiceStub) PublishProfile(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.PublishProfileCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
func (*aiAdministrationServiceStub) DisableProfile(context.Context, aiexplanationadministration.Actor, aiexplanationadministration.DisableProfileCommand) (*domainprofile.AIExplanationProfile, error) {
	return nil, nil
}
