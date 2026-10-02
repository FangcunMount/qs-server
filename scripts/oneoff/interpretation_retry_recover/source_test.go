package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domaininterp "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	interp "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/google/uuid"
)

func validSource(t *testing.T) (sourceSnapshot, time.Time) {
	t.Helper()
	now := time.Now().Truncate(time.Millisecond)
	due, finished, confirmed := now.Add(-time.Minute), now.Add(-2*time.Minute), now.Add(-30*time.Second)
	evt := domaininterp.NewInterpretationRetryRequestedEvent(domaininterp.RetryRequestedEventInput{OrgID: 7, GenerationID: "1001", RunID: "1002", AssessmentID: "1003", OutcomeID: "1004", TesteeID: 1005, ExpectedAttempt: 1, AttemptOrigin: "automatic", Mode: "next_attempt", RequestedAt: due})
	wire, err := standardoutbox.EncodeWire(evt, "apiserver")
	if err != nil {
		t.Fatal(err)
	}
	m, err := message.New(message.Input{Producer: "qs-server", ID: evt.EventID(), Destination: retryDestination, EventType: retryType, SchemaVersion: "v1", Scope: "org:7", ContentType: "application/json", OccurredAt: due.In(time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339Nano), Payload: wire})
	if err != nil {
		t.Fatal(err)
	}
	var source sourceSnapshot
	source.Run.DomainID = 1002
	source.Run.GenerationID = 1001
	source.Run.Attempt = 1
	source.Run.Status = "failed"
	source.Run.RetryDisposition = "automatic"
	source.Run.RetryPolicyVersion = "business-retry/v1"
	source.Run.NextAttemptAt = &due
	source.Run.FinishedAt = &finished
	source.Run.RetryEventID = evt.EventID()
	source.Run.Failure = &interp.InterpretationFailurePO{Kind: "build", Code: "controlled_build_failure", SafeMessage: "controlled failure", Retryable: true}
	source.Generation.DomainID = 1001
	source.Generation.OutcomeID = 1004
	source.Generation.LatestRunID = 1002
	source.Generation.Status = "failed"
	source.Generation.TemplateVersion = "frozen-v1"
	source.Generation.ReportType = "standard"
	source.Generation.Version = 3
	source.Generation.TransactionSchemaVersion = 1
	i := m.Input()
	fp := m.Fingerprint()
	source.Intent = originalIntent{Producer: i.Producer, ID: i.ID, Destination: i.Destination, EventType: i.EventType, SchemaVersion: i.SchemaVersion, Scope: i.Scope, ContentType: i.ContentType, OccurredAt: i.OccurredAt, Payload: i.Payload, Fingerprint: fp[:], State: "published", TransportConfirmedAt: &confirmed}
	return source, now
}

func TestOriginalAuthorityRetainsFullWireAndFrozenSource(t *testing.T) {
	source, now := validSource(t)
	plan, err := validateOriginal(source, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plan.message.Input().Payload, source.Intent.Payload) || plan.EventID != source.Run.RetryEventID || plan.ActionRequestID != "" {
		t.Fatal("original authority changed")
	}
	source.Generation.TemplateVersion = "changed-v2"
	changed, err := validateOriginal(source, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SourceFingerprint == changed.SourceFingerprint {
		t.Fatal("frozen version change did not alter approval fingerprint")
	}
}

func TestUnsafeAuthorityCannotProduceRecoveryPlan(t *testing.T) {
	cases := map[string]func(*sourceSnapshot){
		"pending transport":    func(s *sourceSnapshot) { s.Intent.State = "retry_wait" },
		"missing confirmation": func(s *sourceSnapshot) { s.Intent.TransportConfirmedAt = nil },
		"foreign organization": func(s *sourceSnapshot) { s.Intent.Scope = "org:8" },
		"corrupt wire":         func(s *sourceSnapshot) { s.Intent.Payload = append(s.Intent.Payload, 'x') },
		"wrong fingerprint":    func(s *sourceSnapshot) { s.Intent.Fingerprint = []byte("corrupt") },
		"running original":     func(s *sourceSnapshot) { s.Run.Status = "running" },
		"missing authority":    func(s *sourceSnapshot) { s.Run.RetryEventID = "" },
		"manual required":      func(s *sourceSnapshot) { s.Run.RetryDisposition = "manual_required" },
		"terminal":             func(s *sourceSnapshot) { s.Run.RetryDisposition = "terminal" },
		"unknown model":        func(s *sourceSnapshot) { s.Run.Failure.Code = "result_unknown" },
		"timeout model":        func(s *sourceSnapshot) { s.Run.Failure.Kind = "timeout" },
		"not due":              func(s *sourceSnapshot) { at := time.Now().Add(time.Hour); s.Run.NextAttemptAt = &at },
		"deleted predecessor":  func(s *sourceSnapshot) { at := time.Now(); s.Run.DeletedAt = &at },
		"legacy transaction":   func(s *sourceSnapshot) { s.Generation.TransactionSchemaVersion = 0 },
		"successor projection": func(s *sourceSnapshot) { s.Generation.LatestRunID = 1006 },
		"generated":            func(s *sourceSnapshot) { s.Generation.Status = "generated"; s.Generation.ReportID = 1007 },
		"foreign outcome":      func(s *sourceSnapshot) { s.Generation.OutcomeID = 2004 },
		"wrong attempt":        func(s *sourceSnapshot) { s.Run.Attempt = 2 },
		"new authorization":    func(s *sourceSnapshot) { s.Run.ActionRequestID = "new-authority" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			s, now := validSource(t)
			change(&s)
			if _, err := validateOriginal(s, 7, now); err == nil {
				t.Fatal("unsafe source accepted")
			}
		})
	}
}

func TestJournalBlocksConcurrentAndUnknownRepublish(t *testing.T) {
	source, now := validSource(t)
	plan, err := validateOriginal(source, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	intent := operationIntent{Plan: plan, RequestID: uuid.NewString(), Operator: "operator", Reason: "reviewed original broker loss", ExternalResultReviewed: true}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if reserveOperation(dir, intent) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("reservation winners=%d", wins.Load())
	}
	stored, receipt, err := readOperation(dir, intent.RequestID)
	if err != nil || receipt != nil || stored.Plan.EventID != plan.EventID {
		t.Fatalf("uncertain durable intent missing: %v", err)
	}
	// Even changing request identity cannot bypass an uncertain original.
	other := intent
	other.RequestID = uuid.NewString()
	if reserveOperation(dir, other) == nil {
		t.Fatal("new request bypassed original uncertainty")
	}
	unknown := operationReceipt{RequestID: intent.RequestID, EventID: plan.EventID, TransportOutcome: "unknown"}
	if err := persistExclusive(dir, intent.RequestID+".receipt.json", unknown); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if got := runCLI(context.Background(), []string{"--mode=reconcile", "--audit-dir=" + dir, "--request-id=" + intent.RequestID}, &output, io.Discard); got != 2 {
		t.Fatalf("unknown reconcile exit=%d", got)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"no_republish":true`)) {
		t.Fatal("reconciliation did not retain no-republish boundary")
	}
	info, err := os.Stat(filepath.Join(dir, intent.RequestID+".intent.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("intent is not private")
	}
	if err := persistExclusive(dir, intent.RequestID+".receipt.json", operationReceipt{RequestID: intent.RequestID, EventID: plan.EventID, TransportOutcome: "confirmed"}); err == nil {
		t.Fatal("unknown receipt was overwritten")
	}
}

func TestApplyRequiresExplicitExternalResultReviewAndExactSource(t *testing.T) {
	args := []string{"--mode=apply", "--run-id=1002", "--org-id=7", "--source-fingerprint=" + string(bytes.Repeat([]byte("a"), 64)), "--audit-dir=/private/audit", "--request-id=" + uuid.NewString(), "--operator=operator", "--reason=reviewed"}
	if _, err := parseConfig(args, io.Discard); err == nil {
		t.Fatal("apply without external review accepted")
	}
	if _, err := parseConfig(append(args, "--external-result-reviewed"), io.Discard); err != nil {
		t.Fatal(err)
	}
	source, now := validSource(t)
	plan, err := validateOriginal(source, 7, now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, source.Intent.Payload) || bytes.Contains(encoded, []byte("SafeMessage")) {
		t.Fatal("inspect disclosed payload or report content")
	}
}
