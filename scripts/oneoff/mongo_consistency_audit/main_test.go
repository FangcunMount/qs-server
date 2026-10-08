package main

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	mongoconsistency "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
)

func TestParseScope(t *testing.T) {
	t.Parallel()

	got, err := parseScope(" generation_run, retry_outbox,generation_run ")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	want := []mongoconsistency.Phase{
		mongoconsistency.PhaseGenerationRun,
		mongoconsistency.PhaseRetryOutbox,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
}

type boundedScanner struct {
	requests           []mongoconsistency.BatchRequest
	results            []mongoconsistency.BatchResult
	numericUpperCalled bool
}

func (s *boundedScanner) UpperBound(context.Context, mongoconsistency.Phase, time.Duration) (uint64, error) {
	s.numericUpperCalled = true
	return 10, nil
}
func (s *boundedScanner) OutboxUpperBound(context.Context, time.Duration) ([]byte, error) {
	return []byte("original-bson-upper"), nil
}
func (s *boundedScanner) ScanBatch(_ context.Context, r mongoconsistency.BatchRequest) (mongoconsistency.BatchResult, error) {
	s.requests = append(s.requests, r)
	if len(s.results) == 0 {
		return mongoconsistency.BatchResult{}, fmt.Errorf("unexpected page")
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result, nil
}

func TestOneoffReverseUsesBSONBoundsAndResumesEveryPage(t *testing.T) {
	scanner := &boundedScanner{results: []mongoconsistency.BatchResult{{Scanned: 2, NextOutboxCursor: []byte("original-bson-page-1")}, {Scanned: 1, NextOutboxCursor: []byte("original-bson-upper"), Exhausted: true, EvidenceClasses: map[string]int64{"standard_reference": 1}}}}
	stats, err := scanScope(t.Context(), scanner, mongoconsistency.PhaseOutboxAnswerSheet, config{batchSize: 2, batchTimeout: time.Second, maxSamples: 2})
	if err != nil {
		t.Fatal(err)
	}
	if scanner.numericUpperCalled || len(scanner.requests) != 2 || scanner.requests[1].AfterID != 0 || !bytes.Equal(scanner.requests[1].OutboxCursor, []byte("original-bson-page-1")) || !bytes.Equal(scanner.requests[1].OutboxUpperBound, []byte("original-bson-upper")) || stats.Scanned != 3 || stats.EvidenceClasses["standard_reference"] != 1 {
		t.Fatalf("reverse traversal lost BSON identity: %#v", scanner.requests)
	}
}
func TestOneoffRejectsNonprogressingPage(t *testing.T) {
	scanner := &boundedScanner{results: []mongoconsistency.BatchResult{{Scanned: 1}}}
	if _, err := scanScope(t.Context(), scanner, mongoconsistency.PhaseOutboxAnswerSheet, config{batchSize: 2, batchTimeout: time.Second}); err == nil {
		t.Fatal("reverse empty-token page silently looped")
	}
}
func TestOneoffRequiresFactsForCompleteBusinessCoverageBeforeOpeningMongo(t *testing.T) {
	for _, scope := range []string{"all", "generated_terminal", "retry_outbox", "outbox_answersheet"} {
		if got := run(config{mongoURI: "unusable-test-uri", scope: scope, batchSize: 1, batchTimeout: time.Second, timeout: time.Second}); got != 1 {
			t.Fatalf("%s skipped missing MYSQL_DSN", scope)
		}
	}
	if requiresOutcomeFacts([]mongoconsistency.Phase{mongoconsistency.PhaseModelRelease}) {
		t.Fatal("model-only scope unnecessarily borrowed SQL facts")
	}
}

func TestParseScopeRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	if _, err := parseScope("repair"); err == nil {
		t.Fatal("expected unknown scope to be rejected")
	}
}
