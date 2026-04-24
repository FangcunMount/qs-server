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

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

type fakeWorkerInternalClient struct {
	calculateCalls           int
	createCalls              int
	questionnaireQRCodeCalls int
	scaleQRCodeCalls         int
}

var _ InternalClient = (*fakeWorkerInternalClient)(nil)

func (f *fakeWorkerInternalClient) CreateAssessmentFromAnswerSheet(
	_ context.Context,
	_ *pb.CreateAssessmentFromAnswerSheetRequest,
) (*pb.CreateAssessmentFromAnswerSheetResponse, error) {
	f.createCalls++
	return &pb.CreateAssessmentFromAnswerSheetResponse{
		AssessmentId:  1001,
		Created:       true,
		AutoSubmitted: false,
		Message:       "ok",
	}, nil
}

func (f *fakeWorkerInternalClient) CalculateAnswerSheetScore(
	_ context.Context,
	_ *pb.CalculateAnswerSheetScoreRequest,
) (*pb.CalculateAnswerSheetScoreResponse, error) {
	f.calculateCalls++
	return &pb.CalculateAnswerSheetScoreResponse{
		Success:    true,
		Message:    "ok",
		TotalScore: 42,
	}, nil
}

func (f *fakeWorkerInternalClient) EvaluateAssessment(
	_ context.Context,
	_ uint64,
) (*pb.EvaluateAssessmentResponse, error) {
	return &pb.EvaluateAssessmentResponse{}, nil
}

func (f *fakeWorkerInternalClient) TagTestee(
	_ context.Context,
	_ *pb.TagTesteeRequest,
) (*pb.TagTesteeResponse, error) {
	return &pb.TagTesteeResponse{}, nil
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

func (f *fakeWorkerInternalClient) ProjectBehaviorEvent(
	_ context.Context,
	_ *pb.ProjectBehaviorEventRequest,
) (*pb.ProjectBehaviorEventResponse, error) {
	return &pb.ProjectBehaviorEventResponse{Status: "completed"}, nil
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
		acquire: func(context.Context, *Dependencies, uint64) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "k", Token: "token-1"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redislock.Lease) error {
			released = true
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 123)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.calculateCalls != 1 {
		t.Fatalf("expected 1 score call, got %d", client.calculateCalls)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
	if !released {
		t.Fatalf("expected lock release to be called")
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
		LockKeyBuilder: rediskey.NewBuilderWithNamespace(
			rediskey.ComposeNamespace("worker-test", "cache:lock"),
		),
	}
	if got := answerSheetProcessingLockKey(deps, 42); got != "worker-test:cache:lock:answersheet:processing:42" {
		t.Fatalf("unexpected namespaced lock key: %s", got)
	}
}

func TestHandleAnswerSheetSubmitted_DegradedWithoutRedisContinues(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, nil)

	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redislock.Lease, bool, error) {
			t.Fatal("acquire should not be called when redis client is nil")
			return nil, false, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redislock.Lease) error {
			t.Fatal("release should not be called when redis client is nil")
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 789)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.calculateCalls != 1 {
		t.Fatalf("expected 1 score call, got %d", client.calculateCalls)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
}

func TestHandleAnswerSheetSubmitted_DegradedOnAcquireErrorContinues(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))

	releaseCalled := false
	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redislock.Lease, bool, error) {
			return nil, false, errors.New("boom")
		},
		release: func(context.Context, *Dependencies, uint64, *redislock.Lease) error {
			releaseCalled = true
			return nil
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 900)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.calculateCalls != 1 {
		t.Fatalf("expected 1 score call, got %d", client.calculateCalls)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
	if releaseCalled {
		t.Fatalf("release should not be called when acquire fails")
	}
}

func TestHandleAnswerSheetSubmitted_ReleaseErrorDoesNotFail(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, newAnswerSheetTestRedisClient(t))

	handler := handleAnswerSheetSubmittedWithHooks(deps, answerSheetProcessingGateHooks{
		acquire: func(context.Context, *Dependencies, uint64) (*redislock.Lease, bool, error) {
			return &redislock.Lease{Key: "k", Token: "token-2"}, true, nil
		},
		release: func(context.Context, *Dependencies, uint64, *redislock.Lease) error {
			return errors.New("release failed")
		},
	})

	if err := handler(context.Background(), "answersheet.submitted", mustBuildAnswerSheetSubmittedPayload(t, 901)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.calculateCalls != 1 {
		t.Fatalf("expected 1 score call, got %d", client.calculateCalls)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", client.createCalls)
	}
}

func newAnswerSheetHandlerTestDeps(client InternalClient, redisClient redis.UniversalClient) *Dependencies {
	lockBuilder := rediskey.NewBuilderWithNamespace(
		rediskey.ComposeNamespace("worker-test", "cache:lock"),
	)
	var lockManager *redislock.Manager
	if redisClient != nil {
		lockManager = redislock.NewManager("worker", "lock_lease", &redisplane.Handle{
			Client:  redisClient,
			Builder: lockBuilder,
		})
	}
	return &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
		LockManager:    lockManager,
		LockKeyBuilder: lockBuilder,
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
