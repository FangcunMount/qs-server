package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestOnlyConfirmedOriginalRetryWithoutSuccessorIsASuspectedGap(t *testing.T) {
	at := time.Date(2026, 10, 1, 6, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	row := runRecord{ID: 639343386469347886, GenerationID: 42, Attempt: 1, Status: "failed",
		Disposition: "automatic", RetryEventID: "original-retry", NextAttemptAt: &at}
	generation := generationRecord{Status: "failed", LatestRunID: row.ID}
	confirmed := outboxRecord{EventType: "interpretation.retry.requested", State: "published", TransportConfirmedAt: &at}
	for _, test := range []struct {
		name       string
		generation generationRecord
		intents    []outboxRecord
		want       string
	}{
		{"original confirmed", generation, []outboxRecord{confirmed}, suspectedDeliveryGap},
		{"missing intent", generation, nil, manualRequired},
		{"duplicate intent", generation, []outboxRecord{confirmed, confirmed}, manualRequired},
		{"wrong event", generation, []outboxRecord{{EventType: "interpretation.report.failed", State: "published", TransportConfirmedAt: &at}}, manualRequired},
		{"unconfirmed publication", generation, []outboxRecord{{EventType: confirmed.EventType, State: "published"}}, manualRequired},
		{"pending relay", generation, []outboxRecord{{EventType: confirmed.EventType, State: "pending"}}, relayPending},
		{"generation advanced", generationRecord{Status: "generating", LatestRunID: 43}, []outboxRecord{confirmed}, manualRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyHandoff(row, test.generation, test.intents); got != test.want {
				t.Fatalf("category=%s, want %s", got, test.want)
			}
		})
	}
}

func TestBoundedInputsAndPrivateIdentity(t *testing.T) {
	cutoff := time.Now().Add(-11 * time.Minute).UTC().Format(time.RFC3339)
	input := `{"mongo_uri":"mongodb://localhost","mongo_db":"qs"}`
	base := []string{"--connections-stdin", "--after-id=639343386469347885",
		"--upper-id=639343386469347886", "--due-before=" + cutoff}
	cfg, err := parseConfig(base, strings.NewReader(input), &bytes.Buffer{})
	if err != nil || cfg.upperID != 639343386469347886 {
		t.Fatalf("large ID input: %+v, %v", cfg, err)
	}
	for _, args := range [][]string{
		{"--connections-stdin", "--after-id=3", "--upper-id=3", "--due-before=" + cutoff},
		{"--connections-stdin", "--after-id=3", "--upper-id=4", "--due-before=" + time.Now().UTC().Format(time.RFC3339)},
		append(append([]string(nil), base...), "--max-rows=1001"),
	} {
		if _, err := parseConfig(args, strings.NewReader(input), &bytes.Buffer{}); err == nil {
			t.Fatalf("unsafe input accepted: %v", args)
		}
	}
	result := report{AfterID: fmt.Sprint(cfg.afterID), UpperID: fmt.Sprint(cfg.upperID),
		NextAfterID: fmt.Sprint(cfg.upperID), Counts: map[string]int{suspectedDeliveryGap: 1},
		Findings: []finding{{RunID: fmt.Sprint(cfg.upperID), Category: suspectedDeliveryGap}}}
	result.AfterID, result.UpperID, result.NextAfterID, result.Findings = "", "", "", nil
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), fmt.Sprint(cfg.upperID)) || !strings.Contains(string(encoded), suspectedDeliveryGap) {
		t.Fatalf("public output leaked identity or lost category: %s, %v", encoded, err)
	}
}

func TestRuntimeMongoURIEncodesCredentialsWithoutLoggingThem(t *testing.T) {
	t.Setenv("QS_APISERVER_MONGODB_HOST", "mongo:27017")
	t.Setenv("QS_APISERVER_MONGODB_USERNAME", "user@qs")
	t.Setenv("QS_APISERVER_MONGODB_PASSWORD", "password:/?#")
	t.Setenv("QS_APISERVER_MONGODB_DATABASE", "qs_server")
	uri, database := qsRuntimeMongoConnection()
	if database != "qs_server" || !strings.Contains(uri, "user%40qs:password%3A%2F%3F%23@mongo:27017") ||
		!strings.Contains(uri, "authSource=qs_server") {
		t.Fatal("runtime Mongo connection was not safely encoded")
	}
}
