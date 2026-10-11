package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDBWriterCensusStrictRequestAndActualRoot(t *testing.T) {
	original := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	defer func() { sourceSHA = original }()
	req := dbCensusRequest{FormatVersion: 1, Kind: "readonly_db_writer_census_request", SourceSHA: sourceSHA, OperationID: "123-1", ActualRunID: "456-1", TargetHash: digest(targets), ObservationApprovalSHA256: strings.Repeat("b", 64), Identity: prepareFactsProducer{OperationID: "123-1", RunID: "124-2", SourceSHA: strings.Repeat("c", 40), ReportSHA256: strings.Repeat("d", 64), RequestSHA256: strings.Repeat("e", 64)}}
	if !dbCensusRequestValid(req, "123-1", "456-1") {
		t.Fatal("valid original producer binding rejected")
	}
	raw, _ := json.Marshal(req)
	for _, bad := range [][]byte{append(raw[:len(raw)-1], []byte(`,"DROP":true}`)...), append(raw[:len(raw)-1], []byte(`,"source_sha":"duplicate"}`)...)} {
		var r dbCensusRequest
		if decodePrepareFacts(bad, &r) == nil {
			t.Fatal("ambiguous request accepted")
		}
	}
	req.Identity.RunID = req.ActualRunID
	if dbCensusRequestValid(req, "123-1", "456-1") {
		t.Fatal("original producer relabeled current")
	}
	receipt, e := runDBWriterCensus(context.Background(), "/tmp/not-approved", strings.Repeat("f", 64), "123-1", "456-1")
	if e == nil || receipt.Complete || receipt.ObservationComplete || receipt.WriterScopeComplete || receipt.DropReady || receipt.ExecutionAllowed {
		t.Fatal("fixture/import minted root or writer authority")
	}
}

func TestDBWriterCensusConnectionFailureKeepsOnlySafeCategories(t *testing.T) {
	for _, tc := range []struct {
		cause, category string
	}{
		{"lifecycle_sql_connect_failed", "db_census_original_sql_connection_failed"},
		{"lifecycle_mongo_connect_failed", "db_census_original_mongo_connection_failed"},
		{"lifecycle_connection_input_rejected", "db_census_original_connection_input_rejected"},
		{"unclassified_private_failure", "db_census_original_connection_failed"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			err := fmt.Errorf("private connection detail must not leave: %w", lifecycleError(tc.cause))
			if got := dbCensusOriginalConnectionCategory(err); got != tc.category {
				t.Fatalf("safe category = %q, want %q", got, tc.category)
			}
		})
	}
	if dbCensusOriginalConnectionCategory(fmt.Errorf("lifecycle_sql_connect_failed: private text")) != "db_census_original_connection_failed" {
		t.Fatal("dynamic driver text was interpreted as an authenticated category")
	}
}
