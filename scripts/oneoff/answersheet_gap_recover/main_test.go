package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/recoveryjournal"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/google/uuid"
)

type captureStub struct {
	plan answersheetgap.RecoveryPlan
	err  error
}

func (s captureStub) CaptureOriginalRecovery(context.Context, uint64, uint64, time.Time, time.Time) (answersheetgap.RecoveryPlan, error) {
	return s.plan, s.err
}

type effectStub struct {
	calls    int
	response *pb.EnsureAssessmentResponse
	err      error
}

func (p *effectStub) EnsureOriginal(context.Context, message.Message) (*pb.EnsureAssessmentResponse, error) {
	p.calls++
	return p.response, p.err
}

func reviewedConfig(t *testing.T) config {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return config{mode: "apply", sheetID: 42, orgID: 501, cutoff: time.Now().Add(-time.Minute), auditDir: dir, requestID: uuid.NewString(), operator: "test operator", reason: "original loss reviewed", fingerprint: "reviewed", reviewed: true}
}

func TestSourceChangesAfterReservationNeverCall(t *testing.T) {
	for _, rejected := range []captureStub{{err: answersheetgap.ErrUnsafeRecoverySource}, {plan: answersheetgap.RecoveryPlan{EventID: "original", SourceFingerprint: "changed"}}} {
		cfg := reviewedConfig(t)
		plan := answersheetgap.RecoveryPlan{EventID: "original", SourceFingerprint: "reviewed"}
		caller := &effectStub{response: &pb.EnsureAssessmentResponse{AssessmentId: 77}}
		if got := applyOriginal(context.Background(), rejected, caller, cfg, plan, io.Discard, io.Discard); got != 2 || caller.calls != 0 {
			t.Fatalf("changed source called: exit=%d calls=%d", got, caller.calls)
		}
		intent, receipt, err := recoveryjournal.Read[answersheetgap.RecoveryPlan](cfg.auditDir, cfg.requestID)
		if err != nil || receipt != nil || intent.Plan.EventID != plan.EventID {
			t.Fatal("reserved operation disappeared")
		}
	}
}

func TestUnknownCallRetainsReceiptAndBlocksAnotherOperation(t *testing.T) {
	cfg := reviewedConfig(t)
	plan := answersheetgap.RecoveryPlan{EventID: "original", SourceFingerprint: "reviewed"}
	source := captureStub{plan: plan}
	caller := &effectStub{err: errors.New("response lost")}
	if got := applyOriginal(context.Background(), source, caller, cfg, plan, io.Discard, io.Discard); got != 2 || caller.calls != 1 {
		t.Fatal("unknown call was not retained")
	}
	_, receipt, err := recoveryjournal.Read[answersheetgap.RecoveryPlan](cfg.auditDir, cfg.requestID)
	if err != nil || receipt == nil || receipt.EffectOutcome != "unknown" || receipt.TransportOutcome != "not_sent" || receipt.BusinessCompletionProven {
		t.Fatal("unknown RPC acceptance converted to business success")
	}
	cfg.requestID = uuid.NewString()
	if got := applyOriginal(context.Background(), source, caller, cfg, plan, io.Discard, io.Discard); got != 1 || caller.calls != 1 {
		t.Fatal("new request bypassed original uncertainty")
	}
}

func TestJournalFailurePreventsAnyCall(t *testing.T) {
	cfg := reviewedConfig(t)
	cfg.auditDir += "/missing"
	plan := answersheetgap.RecoveryPlan{EventID: "original", SourceFingerprint: "reviewed"}
	caller := &effectStub{}
	if got := applyOriginal(context.Background(), captureStub{err: errors.New("not reached")}, caller, cfg, plan, io.Discard, io.Discard); got != 1 || caller.calls != 0 {
		t.Fatal("RPC happened without durable reservation")
	}
}

func TestReconcileIsOfflineAndNeverClaimsBusinessCompletion(t *testing.T) {
	cfg := reviewedConfig(t)
	plan := answersheetgap.RecoveryPlan{EventID: "original", SourceFingerprint: "reviewed"}
	if err := recoveryjournal.Reserve(cfg.auditDir, recoveryjournal.Intent[answersheetgap.RecoveryPlan]{Plan: plan, RequestID: cfg.requestID, Operator: cfg.operator, Reason: cfg.reason, ExternalResultReviewed: true}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if got := runCLI(context.Background(), []string{"--mode=reconcile", "--audit-dir=" + cfg.auditDir, "--request-id=" + cfg.requestID}, &output, io.Discard); got != 2 {
		t.Fatalf("uncertain reconcile exit=%d", got)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"no_repeat_effect":true`)) {
		t.Fatal("no-recall boundary lost")
	}
}

func TestApplyRequiresFixedReviewedScopeAndPastCutoff(t *testing.T) {
	now := time.Now()
	args := []string{"--mode=apply", "--answersheet-id=42", "--org-id=501", "--accepted-before=" + now.Add(-time.Minute).Format(time.RFC3339Nano), "--source-fingerprint=" + string(bytes.Repeat([]byte("a"), 64)), "--audit-dir=/private/recovery", "--request-id=" + uuid.NewString(), "--operator=operator", "--reason=reviewed"}
	if _, err := parseConfig(args, io.Discard, now); err == nil {
		t.Fatal("missing review accepted")
	}
	if _, err := parseConfig(append(args, "--external-result-reviewed"), io.Discard, now); err != nil {
		t.Fatal(err)
	}
	if _, err := parseConfig([]string{"--answersheet-id=42", "--org-id=501", "--accepted-before=" + now.Add(time.Minute).Format(time.RFC3339Nano)}, io.Discard, now); err == nil {
		t.Fatal("future acceptance cutoff admitted")
	}
}
