package main

import (
	"context"
	"encoding/json"
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
