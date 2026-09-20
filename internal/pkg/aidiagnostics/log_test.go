package aidiagnostics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecordDropsNonUUIDBusinessFieldsAndPreservesCorrelation(t *testing.T) {
	fields := record("delivery_rejected", "conflict", "version_conflict", "http-request-7", "SECRET_REPORT", "SECRET_TOKEN")
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SECRET") {
		t.Fatal("unsafe business identifier escaped")
	}
	if fields["correlation_id"] != "http-request-7" || fields["level"] != "WARNING" {
		t.Fatal(fields)
	}
	if fields["event"] == "delivery_confirmed" {
		t.Fatal("conflict cannot be acknowledged")
	}
}
