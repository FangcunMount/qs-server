package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type fakeWorkerInternalClient struct {
	calculateCalls                 int
	createCalls                    int
	generateReportCalls            int
	calls                          []string
	syncAssessmentAttentionCalls   int
	syncAssessmentAttentionRequest *pb.SyncAssessmentAttentionRequest
	questionnaireQRCodeCalls       int
	scaleQRCodeCalls               int
	calculateScoreSuccess          bool
	calculateScoreMessage          string
	createSuccess                  bool
	createMessage                  string
	ensureCreated                  *bool
	ensureAutoSubmitted            bool
}

var _ InternalClient = (*fakeWorkerInternalClient)(nil)

func (f *fakeWorkerInternalClient) EnsureAssessment(
	_ context.Context,
	_ *evalpb.EnsureAssessmentRequest,
) (*evalpb.EnsureAssessmentResponse, error) {
	f.createCalls++
	f.calls = append(f.calls, "create_assessment")
	if f.calculateScoreMessage != "" && !f.calculateScoreSuccess {
		return nil, errors.New(f.calculateScoreMessage)
	}
	success := true
	if f.createMessage != "" {
		success = f.createSuccess
	}
	if !success {
		return nil, errors.New(firstNonEmpty(f.createMessage, "assessment ensure failed"))
	}
	created := true
	if f.ensureCreated != nil {
		created = *f.ensureCreated
	}
	return &evalpb.EnsureAssessmentResponse{
		AssessmentId:  1001,
		Created:       created,
		AutoSubmitted: f.ensureAutoSubmitted,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (f *fakeWorkerInternalClient) ExecuteEvaluation(
	_ context.Context,
	_ uint64,
) (*evalpb.ExecuteEvaluationResponse, error) {
	return &evalpb.ExecuteEvaluationResponse{}, nil
}

func (f *fakeWorkerInternalClient) GenerateReportFromOutcome(
	_ context.Context,
	_ string,
) (*interpretationpb.GenerateReportFromAssessmentResponse, error) {
	f.generateReportCalls++
	f.calls = append(f.calls, "generate_report")
	return &interpretationpb.GenerateReportFromAssessmentResponse{Success: true}, nil
}

func (f *fakeWorkerInternalClient) SyncAssessmentAttention(
	_ context.Context,
	req *pb.SyncAssessmentAttentionRequest,
) (*pb.SyncAssessmentAttentionResponse, error) {
	f.syncAssessmentAttentionCalls++
	f.syncAssessmentAttentionRequest = req
	return &pb.SyncAssessmentAttentionResponse{}, nil
}

func (f *fakeWorkerInternalClient) GenerateQuestionnaireQRCode(
	_ context.Context,
	_, _ string,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	f.questionnaireQRCodeCalls++
	return &pb.GenerateQuestionnaireQRCodeResponse{}, nil
}

func (f *fakeWorkerInternalClient) HandleQuestionnairePublishedPostActions(
	_ context.Context,
	_, _ string,
) (*pb.GenerateQuestionnaireQRCodeResponse, error) {
	f.questionnaireQRCodeCalls++
	return &pb.GenerateQuestionnaireQRCodeResponse{Success: true}, nil
}

func (f *fakeWorkerInternalClient) GenerateScaleQRCode(
	_ context.Context,
	_ string,
) (*pb.GenerateScaleQRCodeResponse, error) {
	f.scaleQRCodeCalls++
	return &pb.GenerateScaleQRCodeResponse{}, nil
}

func (f *fakeWorkerInternalClient) HandleScalePublishedPostActions(
	_ context.Context,
	_ string,
) (*pb.GenerateScaleQRCodeResponse, error) {
	f.scaleQRCodeCalls++
	return &pb.GenerateScaleQRCodeResponse{Success: true}, nil
}

func (f *fakeWorkerInternalClient) SendTaskOpenedMiniProgramNotification(
	_ context.Context,
	_ int64,
	_ string,
	_ uint64,
	_ string,
	_ time.Time,
) (*pb.SendTaskOpenedMiniProgramNotificationResponse, error) {
	return &pb.SendTaskOpenedMiniProgramNotificationResponse{}, nil
}

func TestHandleAnswerSheetSubmitted_LockedExecutesAndReleases(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))

	released := false
	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "token-1"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			released = true
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 123)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
	if !released {
		t.Fatalf("expected lock release to be called")
	}
}

func TestHandleAnswerSheetSubmitted_UsesSingleCreateAssessmentCall(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))
	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "token-order"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 124)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	want := []string{"create_assessment"}
	if len(client.calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", client.calls, want)
	}
	for i := range want {
		if client.calls[i] != want[i] {
			t.Fatalf("calls = %#v, want %#v", client.calls, want)
		}
	}
	if client.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", client.createCalls)
	}
}

func TestHandleAnswerSheetSubmitted_AcceptsExistingAutoSubmittedAssessment(t *testing.T) {
	created := false
	client := &fakeWorkerInternalClient{
		ensureCreated:       &created,
		ensureAutoSubmitted: true,
	}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))
	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "token-replay"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 125)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if client.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1 EnsureAssessment call for idempotent replay", client.createCalls)
	}
}

func TestHandleAnswerSheetSubmitted_DuplicateSkip(t *testing.T) {
	mr := miniredis.RunT(t)
	redisClient := newAnswerSheetTestRedisClientWithAddr(t, mr.Addr())
	client := &fakeWorkerInternalClient{}

	answerSheetID := uint64(456)
	if err := mr.Set(answerSheetProcessingLockKey(newAnswerSheetHandlerTestDeps(client, redisClient), answerSheetID), "busy"); err != nil {
		t.Fatalf("set lock: %v", err)
	}

	deps := newAnswerSheetHandlerTestDeps(client, redisClient)
	handler := handleAnswerSheetSubmitted(deps)

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, answerSheetID)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.calculateCalls != 0 {
		t.Fatalf("expected no score calls, got %d", client.calculateCalls)
	}
	if client.createCalls != 0 {
		t.Fatalf("expected no create calls, got %d", client.createCalls)
	}
	if !mr.Exists(answerSheetProcessingLockKey(deps, answerSheetID)) {
		t.Fatalf("expected duplicate lock key to remain set")
	}
}

func TestAnswerSheetProcessingLockKeyUsesNamespace(t *testing.T) {
	deps := &Dependencies{
		LockKeyBuilder: keyspace.NewBuilderWithNamespace(
			keyspace.ComposeNamespace("worker-test", "cache:lock"),
		),
	}
	if got := answerSheetProcessingLockKey(deps, 42); got != "worker-test:cache:lock:answersheet:processing:42" {
		t.Fatalf("unexpected namespaced lock key: %s", got)
	}
}

func TestHandleAnswerSheetSubmitted_DegradedWithoutRedisContinues(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, nil)
	observer := &workerGateRecordingObserver{}

	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		observer: observer,
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			t.Fatal("acquire should not be called when redis client is nil")
			return nil, false, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			t.Fatal("release should not be called when redis client is nil")
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 789)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
	if !observer.has(resilienceplane.OutcomeDegradedOpen) {
		t.Fatal("expected degraded_open outcome")
	}
}

func TestHandleAnswerSheetSubmitted_DegradedOnAcquireErrorContinues(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))
	observer := &workerGateRecordingObserver{}

	releaseCalled := false
	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		observer: observer,
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return nil, false, errors.New("boom")
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			releaseCalled = true
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 900)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
	if releaseCalled {
		t.Fatalf("release should not be called when acquire fails")
	}
	if !observer.has(resilienceplane.OutcomeDegradedOpen) {
		t.Fatal("expected degraded_open outcome")
	}
}

func TestAnswerSheetRunnerAcquireFailureRemainsDegradedOpen(t *testing.T) {
	bodyCalls := 0
	deps := &Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		LockRunner: runnerStub{run: func(context.Context, locklease.WorkloadID, string, time.Duration, func(context.Context) error) (locklease.RunResult, error) {
			return locklease.RunResult{}, locklease.ErrLeaseAcquireFailed
		}},
	}
	gate := newAnswerSheetDuplicateSuppressionGate(answerSheetProcessingGateHooks{})
	if err := gate.Run(context.Background(), deps, "event-1", 42, func(context.Context) error {
		bodyCalls++
		return nil
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bodyCalls != 1 {
		t.Fatalf("body calls = %d, want degraded-open execution", bodyCalls)
	}
}

func TestAnswerSheetRunnerLeaseLossFailsForMessageRetry(t *testing.T) {
	deps := &Dependencies{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		LockRunner: runnerStub{run: func(context.Context, locklease.WorkloadID, string, time.Duration, func(context.Context) error) (locklease.RunResult, error) {
			return locklease.RunResult{Acquired: true}, locklease.ErrLeaseLost
		}},
	}
	gate := newAnswerSheetDuplicateSuppressionGate(answerSheetProcessingGateHooks{})
	err := gate.Run(context.Background(), deps, "event-1", 42, func(context.Context) error { return nil })
	if !errors.Is(err, locklease.ErrLeaseLost) {
		t.Fatalf("Run() error = %v, want ErrLeaseLost", err)
	}
}

type runnerStub struct {
	run func(context.Context, locklease.WorkloadID, string, time.Duration, func(context.Context) error) (locklease.RunResult, error)
}

func (r runnerStub) Run(ctx context.Context, workload locklease.WorkloadID, key string, ttl time.Duration, body func(context.Context) error) (locklease.RunResult, error) {
	return r.run(ctx, workload, key, ttl, body)
}

func TestHandleAnswerSheetSubmitted_DuplicateSkipUsesInjectedObserver(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))
	observer := &workerGateRecordingObserver{}

	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		observer: observer,
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return nil, false, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			t.Fatal("release should not be called when lock is not acquired")
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 902)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if client.calculateCalls != 0 {
		t.Fatalf("expected no score calls, got %d", client.calculateCalls)
	}
	if client.createCalls != 0 {
		t.Fatalf("expected no create calls, got %d", client.createCalls)
	}
	if !observer.has(resilienceplane.OutcomeDuplicateSkipped) {
		t.Fatal("expected duplicate_skipped outcome")
	}
}

func TestHandleAnswerSheetSubmitted_ReleaseErrorDoesNotFail(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))

	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "token-2"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			return errors.New("release failed")
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 901)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
}

func TestHandleAnswerSheetSubmitted_ScoringFailureStopsBeforeCreate(t *testing.T) {
	client := &fakeWorkerInternalClient{
		calculateScoreSuccess: false,
		calculateScoreMessage: "invalid answers",
	}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))
	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "token-score-fail"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			return nil
		},
	})

	err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 1001))
	if err == nil {
		t.Fatal("expected scoring failure error")
	}
	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call after scoring failure, got %d", client.createCalls)
	}
}

func TestHandleAnswerSheetSubmitted_CreateFailureAfterSuccessfulScore(t *testing.T) {
	client := &fakeWorkerInternalClient{
		createSuccess: false,
		createMessage: "binding not found",
	}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))
	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redisadapter.Lease, bool, error) {
			return &redisadapter.Lease{Key: "k", Token: "token-create-fail"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redisadapter.Lease) error {
			return nil
		},
	})

	err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 1002))
	if err == nil {
		t.Fatal("expected assessment creation failure error")
	}
	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
}

type workerTestClient interface {
	InternalClient
	AssessmentIntakeClient
	EvaluationWorkerClient
	InterpretationAutomationClient
}

func newAnswerSheetHandlerTestDeps(client workerTestClient, redisClient redis.UniversalClient) *Dependencies {
	lockBuilder := keyspace.NewBuilderWithNamespace(
		keyspace.ComposeNamespace("worker-test", "cache:lock"),
	)
	var lockManager *redisadapter.Manager
	if redisClient != nil {
		lockManager = redisadapter.NewManager("worker", "lock_lease", &redisruntime.Handle{
			Client:  redisClient,
			Builder: lockBuilder,
		})
	}
	return &Dependencies{
		Logger:                         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient:                 client,
		AssessmentIntakeClient:         client,
		EvaluationWorkerClient:         client,
		InterpretationAutomationClient: client,
		LockManager:                    lockManager,
		LockKeyBuilder:                 lockBuilder,
	}
}

func newAnswerSheetTestRedisClient(t *testing.T) redis.UniversalClient {
	t.Helper()
	return newAnswerSheetTestRedisClientWithAddr(t, miniredis.RunT(t).Addr())
}

func newAnswerSheetTestRedisClientWithAddr(t *testing.T, addr string) redis.UniversalClient {
	t.Helper()

	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

func mustBuildAnswerSheetSubmittedPayload(t *testing.T, answerSheetID uint64) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	answerSheetIDStr := strconv.FormatUint(answerSheetID, 10)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-answersheet",
		"eventType":     "answersheet.submitted",
		"occurredAt":    now,
		"aggregateType": "AnswerSheet",
		"aggregateID":   answerSheetIDStr,
		"data": map[string]any{
			"answersheet_id":        answerSheetIDStr,
			"questionnaire_code":    "QNR-001",
			"questionnaire_version": "1.0.0",
			"testee_id":             99,
			"org_id":                18,
			"filler_id":             77,
			"filler_type":           "testee",
			"task_id":               "",
			"submitted_at":          now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

type workerGateRecordingObserver struct {
	decisions []resilienceplane.Decision
}

func (r *workerGateRecordingObserver) ObserveDecision(_ context.Context, decision resilienceplane.Decision) {
	r.decisions = append(r.decisions, decision)
}

func (r *workerGateRecordingObserver) has(outcome resilienceplane.Outcome) bool {
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}
