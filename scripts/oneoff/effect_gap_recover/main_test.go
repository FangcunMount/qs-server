package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	reportpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	domainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/originaleffect"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/recoveryjournal"
	"github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

type captureStub struct {
	plan originaleffect.Plan
	err  error
}

func (s captureStub) Capture(context.Context, originaleffect.Request, time.Time) (originaleffect.Plan, error) {
	return s.plan, s.err
}

type effectStub struct {
	calls  int
	result effectResult
	err    error
	before func()
}

func (e *effectStub) Apply(context.Context, originaleffect.Plan) (effectResult, error) {
	e.calls++
	if e.before != nil {
		e.before()
	}
	return e.result, e.err
}

func reviewed(t *testing.T) (config, originaleffect.Plan) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	plan := originaleffect.Plan{EventID: "eval-retry:71:1:automatic", EventType: originaleffect.EvaluationRetry, AssessmentID: 71, OrgID: 3, PreviousRunID: "71:1", ExpectedAttempt: 1, SourceFingerprint: "reviewed"}
	cfg := config{mode: "apply", request: originaleffect.Request{EventID: plan.EventID, AssessmentID: 71, OrgID: 3, AcceptedBefore: time.Now().Add(-time.Minute)}, auditDir: dir, requestID: uuid.NewString(), operator: "test-operator", reason: "original loss checked", reviewed: true, fingerprint: plan.SourceFingerprint}
	return cfg, plan
}

func TestSourceChangeAfterReservationNeverInvokesOrReuses(t *testing.T) {
	for _, source := range []captureStub{{err: originaleffect.ErrUnsafeSource}, {plan: originaleffect.Plan{EventID: "event", SourceFingerprint: "changed"}}} {
		cfg, plan := reviewed(t)
		effect := &effectStub{}
		require.Equal(t, 2, applyOriginal(t.Context(), source, effect, cfg, plan, io.Discard, io.Discard))
		require.Zero(t, effect.calls)
		intent, receipt, err := recoveryjournal.Read[originaleffect.Plan](cfg.auditDir, cfg.requestID)
		require.NoError(t, err)
		require.Nil(t, receipt)
		require.Equal(t, plan.EventID, intent.Plan.EventID)
		cfg.requestID = uuid.NewString()
		require.Equal(t, 1, applyOriginal(t.Context(), captureStub{plan: plan}, effect, cfg, plan, io.Discard, io.Discard))
		require.Zero(t, effect.calls)
	}
}

func TestUnknownResultNeverRepeatsAndJournalPrecedesEffect(t *testing.T) {
	for _, reply := range []effectStub{{err: errors.New("response lost")}, {result: effectResult{RunID: "71:2"}}, {result: effectResult{Accepted: true, RunID: "71:2"}, err: errors.New("ambiguous response")}} {
		cfg, plan := reviewed(t)
		reply.before = func() {
			intent, receipt, err := recoveryjournal.Read[originaleffect.Plan](cfg.auditDir, cfg.requestID)
			require.NoError(t, err)
			require.Nil(t, receipt)
			require.Equal(t, plan.EventID, intent.Plan.EventID)
		}
		require.Equal(t, 2, applyOriginal(t.Context(), captureStub{plan: plan}, &reply, cfg, plan, io.Discard, io.Discard))
		require.Equal(t, 1, reply.calls)
		_, receipt, err := recoveryjournal.Read[originaleffect.Plan](cfg.auditDir, cfg.requestID)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		require.Equal(t, "unknown", receipt.EffectOutcome)
		require.Equal(t, "not_sent", receipt.TransportOutcome)
		require.False(t, receipt.BusinessCompletionProven)
		var output bytes.Buffer
		require.Equal(t, 2, runCLI(t.Context(), []string{"--mode=reconcile", "--audit-dir=" + cfg.auditDir, "--request-id=" + cfg.requestID}, &output, io.Discard))
		require.Contains(t, output.String(), `"no_repeat_effect":true`)
		cfg.requestID = uuid.NewString()
		require.Equal(t, 1, applyOriginal(t.Context(), captureStub{plan: plan}, &reply, cfg, plan, io.Discard, io.Discard))
		require.Equal(t, 1, reply.calls)
	}
}

func TestUnavailableOrUncertainJournalDoesNotPermitAnotherEffect(t *testing.T) {
	cfg, plan := reviewed(t)
	effect := &effectStub{result: effectResult{Accepted: true, RunID: "71:2"}}
	goodDir := cfg.auditDir
	cfg.auditDir += "/missing"
	require.Equal(t, 1, applyOriginal(t.Context(), captureStub{plan: plan}, effect, cfg, plan, io.Discard, io.Discard))
	require.Zero(t, effect.calls)
	cfg.auditDir = goodDir
	// The effect may have happened; a failed receipt write must remain uncertain.
	effect.before = func() { require.NoError(t, os.Mkdir(filepath.Join(cfg.auditDir, cfg.requestID+".receipt.json"), 0700)) }
	require.Equal(t, 2, applyOriginal(t.Context(), captureStub{plan: plan}, effect, cfg, plan, io.Discard, io.Discard))
	require.Equal(t, 1, effect.calls)
	cfg.requestID = uuid.NewString()
	require.Equal(t, 1, applyOriginal(t.Context(), captureStub{plan: plan}, effect, cfg, plan, io.Discard, io.Discard))
	require.Equal(t, 1, effect.calls)
}

type evaluationClient struct {
	response *evalpb.ExecuteEvaluationResponse
	err      error
	inspect  func(metadata.MD, uint64)
}

func (c evaluationClient) ExecuteEvaluation(ctx context.Context, id uint64) (*evalpb.ExecuteEvaluationResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	if c.inspect != nil {
		c.inspect(md, id)
	}
	return c.response, c.err
}

type reportClient struct {
	response *reportpb.GenerateReportFromAssessmentResponse
	err      error
	inspect  func(metadata.MD, string)
}

func (c reportClient) GenerateReportFromOutcome(ctx context.Context, id string) (*reportpb.GenerateReportFromAssessmentResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	if c.inspect != nil {
		c.inspect(md, id)
	}
	return c.response, c.err
}

func TestNormalWorkerPreservesOriginalRetryAuthorizationAndVerifiesSuccessor(t *testing.T) {
	_, plan := reviewed(t)
	evt := domainevent.NewRetryRequestedEvent(domainevent.RequestedInput{OrgID: 3, AssessmentID: 71, EventID: plan.EventID, ModelKind: "scale", ModelCode: "M1", ModelVersion: "v1", ExpectedAttempt: 1, AttemptOrigin: "manual", ActionRequestID: "original-action", Mode: "next_attempt"})
	payload, err := domain.EncodeEvent(evt)
	require.NoError(t, err)
	wire := legacy.Envelope{UUID: evt.EventID(), Payload: payload}
	for _, reply := range []*evalpb.ExecuteEvaluationResponse{
		{Status: "evaluated", RunId: "71:2", CurrentAttempt: 2, OutcomeId: "91"},
		{Status: "failed", RunId: "71:2", CurrentAttempt: 2, RetryDisposition: "manual_required"},
		{Status: "failed", RunId: "71:1", CurrentAttempt: 1, RetryDisposition: "automatic"},
		{Status: "evaluated", RunId: "72:2", CurrentAttempt: 2}, nil,
	} {
		client := evaluationClient{response: reply, inspect: func(md metadata.MD, id uint64) {
			require.EqualValues(t, 71, id)
			require.Equal(t, []string{plan.EventID}, md.Get("x-retry-event-id"))
			require.Equal(t, []string{"1"}, md.Get("x-retry-expected-attempt"))
			require.Equal(t, []string{"manual"}, md.Get("x-retry-origin"))
			require.Equal(t, []string{"original-action"}, md.Get("x-retry-action-request-id"))
			require.Equal(t, []string{"next_attempt"}, md.Get("x-retry-mode"))
		}}
		result, _ := (workerEffect{evaluation: client}).applyWire(t.Context(), plan, wire)
		want := reply != nil && reply.RunId == "71:2" && reply.CurrentAttempt == 2
		require.Equal(t, want, result.Accepted)
	}
}

func TestReportAcceptanceRequiresOriginalOutcomeEventAndDurableIDs(t *testing.T) {
	evt := domainevent.NewOutcomeCommittedEvent(3, 71, 29, "91", "71:1", time.Now())
	payload, err := domain.EncodeEvent(evt)
	require.NoError(t, err)
	plan := originaleffect.Plan{EventID: evt.EventID(), EventType: originaleffect.ReportInitial, OutcomeID: "91"}
	wire := legacy.Envelope{UUID: evt.EventID(), Payload: payload}
	for _, test := range []struct {
		response *reportpb.GenerateReportFromAssessmentResponse
		accepted bool
	}{
		{&reportpb.GenerateReportFromAssessmentResponse{Success: true, Status: "generated", GenerationId: "100", RunId: "101"}, true},
		{&reportpb.GenerateReportFromAssessmentResponse{Success: false, Status: "failed", GenerationId: "100", RunId: "101", RetryDisposition: "manual_required"}, true},
		{&reportpb.GenerateReportFromAssessmentResponse{Success: true, Status: "blocked", GenerationId: "100", RunId: "101"}, false},
		{&reportpb.GenerateReportFromAssessmentResponse{Success: false, Status: "admission_rejected", RetryDisposition: "terminal"}, false},
		{&reportpb.GenerateReportFromAssessmentResponse{Success: true, Status: "generated", GenerationId: "0", RunId: "101"}, false},
		{nil, false},
	} {
		client := reportClient{response: test.response, inspect: func(md metadata.MD, id string) {
			require.Equal(t, "91", id)
			require.Equal(t, []string{plan.EventID}, md.Get("x-event-id"))
			require.Empty(t, md.Get("x-retry-event-id"))
		}}
		result, _ := (workerEffect{report: client}).applyWire(t.Context(), plan, wire)
		require.Equal(t, test.accepted, result.Accepted)
	}
}

func TestBoundedInitialMySQLHandshakeLeavesNoReservation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		close(accepted)
		defer connection.Close()
		_, _ = io.Copy(io.Discard, connection)
	}()
	t.Setenv("MYSQL_DSN", "root:test@tcp("+listener.Addr().String()+")/isolated?parseTime=true")
	t.Setenv("MONGO_URI", "mongodb://127.0.0.1:27001/?directConnection=true")
	t.Setenv("MONGO_DB", "isolated")
	started := time.Now()
	require.Equal(t, 1, runCLI(t.Context(), []string{"--event-id=original", "--assessment-id=71", "--org-id=3", "--accepted-before=" + started.Add(-time.Minute).Format(time.RFC3339Nano), "--timeout=1s"}, io.Discard, io.Discard))
	require.Less(t, time.Since(started), 1800*time.Millisecond)
	select {
	case <-accepted:
	default:
		t.Fatal("never reached real MySQL handshake")
	}
}

func TestApplyAcceptsDeterministicEventButRequiresCanonicalOperation(t *testing.T) {
	now := time.Now()
	args := []string{"--mode=apply", "--event-id=eval-retry:71:1:automatic", "--assessment-id=71", "--org-id=3", "--accepted-before=" + now.Add(-time.Minute).Format(time.RFC3339Nano), "--source-fingerprint=" + string(bytes.Repeat([]byte("a"), 64)), "--audit-dir=/private/recovery", "--request-id=" + uuid.NewString(), "--operator=operator", "--reason=reviewed"}
	_, err := parseConfig(args, io.Discard, now)
	require.Error(t, err)
	_, err = parseConfig(append(args, "--external-result-reviewed"), io.Discard, now)
	require.NoError(t, err)
	_, err = parseConfig(append(args, "--external-result-reviewed", "--request-id=opaque-event"), io.Discard, now)
	require.Error(t, err)
}

func TestSharedReceiptPreservesOldJSONWhenNewIDsAbsent(t *testing.T) {
	old := recoveryjournal.Receipt{RequestID: "op", EventID: "evt", TransportOutcome: "unknown", RecordedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	encoded, err := json.Marshal(old)
	require.NoError(t, err)
	require.JSONEq(t, `{"recovery_request_id":"op","event_id":"evt","transport_outcome":"unknown","business_completion_proven":false,"recorded_at":"2026-10-03T00:00:00Z"}`, string(encoded))
}
