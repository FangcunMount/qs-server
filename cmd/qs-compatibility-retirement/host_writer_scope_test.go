package main

import (
	"context"
	"strings"
	"testing"
)

func TestHostWriterScopeStrictBindingAndNoEffects(t *testing.T) {
	previous := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	defer func() { sourceSHA = previous }()
	r := hostWriterScopeRequest{FormatVersion: 1, Kind: "readonly_host_writer_scope_request", SourceSHA: sourceSHA, OperationID: "123-1", ActualRunID: "124-1", HostRole: "server_a", TargetHash: digest(targets), ObservationApprovalSHA256: strings.Repeat("b", 64)}
	if e := validateHostWriterScopeRequest(r, "123-1", "124-1"); e != nil {
		t.Fatal(e)
	}
	r.HostRole = "server_d"
	if validateHostWriterScopeRequest(r, "123-1", "124-1") == nil {
		t.Fatal("A route adopted D role")
	}
	r.HostRole = "server_a"
	r.ActualRunID = "125-1"
	if validateHostWriterScopeRequest(r, "123-1", "124-1") == nil {
		t.Fatal("foreign run")
	}
	for _, path := range []string{"/tmp/request.json", "/opt/backups/qs-server/compatibility-retirement/123-1/host-writer-scope-request-125-1.json", "/opt/backups/qs-server/compatibility-retirement/123-1/../request.json"} {
		if hostWriterScopeBoundPath(path, "123-1", "124-1") {
			t.Fatal("arbitrary source path accepted")
		}
	}
	var decoded hostWriterScopeRequest
	if decodePrepareFacts([]byte(`{"format_version":1,"kind":"readonly_host_writer_scope_request","source_sha":"a","operation_id":"123-1","actual_run_id":"124-1","host_role":"server_a","target_hash":"b","observation_approval_sha256":"c","drop_ready":false}`), &decoded) == nil {
		t.Fatal("unknown capability field accepted")
	}
	if decodePrepareFacts([]byte(`{"format_version":1,"format_version":1}`), &decoded) == nil {
		t.Fatal("duplicate field accepted")
	}
}

func TestHostWriterScopeActualEarlyRejectionRetainsCategory(t *testing.T) {
	// The invalid fixed path fails before any host file observation, even when
	// this test runs with an actual Linux root UID. No root identity is mocked.
	r, err := runHostWriterScope(context.Background(), "/unbound-request", strings.Repeat("a", 64), "123-1", "124-1")
	if err == nil || r.HostRole != "server_a" || r.ErrorCategory != "host_scope_root_once_required" || r.ObservationComplete || r.MachineIDSHA256 != "" || r.BootIDSHA256 != "" || r.NamespaceSHA256 != "" || r.CatalogSHA256 != "" {
		t.Fatal("early rejection discarded category or invented actual host identity")
	}
	if r.Complete || r.ExecutionAllowed || r.WriterScopeComplete || r.DropReady {
		t.Fatal("early rejection acquired a writer capability")
	}
}
