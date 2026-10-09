package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func prepareFactsRequestFixture(t *testing.T) prepareFactsRequest {
	t.Helper()
	previous := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = previous })
	return prepareFactsRequest{FormatVersion: 1, Kind: "readonly_prepare_facts_request", SourceSHA: sourceSHA,
		OperationID: "123-1", ActualRunID: "789-1", TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb",
		ObservationApprovalSHA256: strings.Repeat("b", 64), Inventory: prepareFactsProducer{OperationID: "123-1", RunID: "456-1", SourceSHA: strings.Repeat("c", 40), ReportSHA256: strings.Repeat("d", 64), RequestSHA256: strings.Repeat("e", 64)},
		RestoreEngines: &lifecycleRestoreEngines{MySQLImageID: "sha256:" + strings.Repeat("1", 64), MongoImageID: "sha256:" + strings.Repeat("2", 64), Architecture: runtime.GOARCH}, ArchiveDirectory: "/opt/backups/qs-server/compatibility-retirement/123-1/temporary-archive"}
}

func TestPrepareFactsKeepsOriginalProducerAndRejectsAuthorityFields(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	if r.SourceSHA == r.Inventory.SourceSHA || validatePrepareFactsRequest(r, "123-1", "789-1") != nil {
		t.Fatal("separate producer binding rejected")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`"drop_ready":true`, `"ordered_mongo_schema_sha256":"approved"`, `"manifest_sha256":"approved"`} {
		var got prepareFactsRequest
		if decodePrepareFacts(append(raw[:len(raw)-1], []byte(","+extra+"}")...), &got) == nil {
			t.Fatal("authority field accepted")
		}
	}
	var got prepareFactsRequest
	if decodePrepareFacts([]byte(strings.Replace(string(raw), `"source_sha"`, `"Source_SHA"`, 1)), &got) == nil {
		t.Fatal("case alias accepted")
	}
	if decodePrepareFacts([]byte(strings.Replace(string(raw), `"format_version":1`, `"format_version":1,"format_version":1`, 1)), &got) == nil {
		t.Fatal("duplicate accepted")
	}
	mutations := []func(*prepareFactsRequest){
		func(v *prepareFactsRequest) { v.Inventory.SourceSHA = "unknown" },
		func(v *prepareFactsRequest) { v.Inventory.RunID = v.ActualRunID },
		func(v *prepareFactsRequest) { v.Inventory.OperationID = "999-1" },
		func(v *prepareFactsRequest) {
			v.RestoreEngines = &lifecycleRestoreEngines{MySQLImageID: "mysql:8", MongoImageID: "mongo:7", Architecture: runtime.GOARCH}
		},
		func(v *prepareFactsRequest) {
			v.ArchiveDirectory = "/opt/backups/qs-server/compatibility-retirement/123-1/inventory-456-1/child"
		},
	}
	for _, mutate := range mutations {
		got := r
		mutate(&got)
		if validatePrepareFactsRequest(got, "123-1", "789-1") == nil {
			t.Fatal("invalid binding accepted")
		}
	}
}

func TestPrepareFactsHashesPrivateBytesWithoutCopyOrDisclosure(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "source.private")
	body := []byte("PRIVATE_SOURCE_BODY_NOT_A_RECEIPT")
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	fact, kept, err := hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false)
	if err != nil || kept != nil || fact.SHA256 != digestRaw(body) || fact.Bytes != uint64(len(body)) {
		t.Fatalf("actual hash failed: %v", err)
	}
	encoded, err := json.Marshal(fact)
	if err != nil || strings.Contains(string(encoded), string(body)) {
		t.Fatal("body disclosed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("source copied")
	}
	if err = os.Link(path, filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("hardlink accepted")
	}
	if err = os.Remove(filepath.Join(dir, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = hashPrepareFactsFile(context.Background(), path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("public source accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = hashPrepareFactsFile(ctx, path, uint32(os.Getuid()), 4096, false); err == nil {
		t.Fatal("cancelled source read accepted")
	}
}

func TestPrepareFactsReportsActualCapacityWithoutReadiness(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	fact, err := observePrepareFactsFS("archive", dir)
	if err != nil || fact.Scope != "archive" || fact.PathSHA256 != digestRaw([]byte(dir)) || fact.TotalBytes == 0 || fact.FreeBytes > fact.TotalBytes || fact.AvailableBytes > fact.TotalBytes {
		t.Fatalf("actual statfs observation failed: %v", err)
	}
	parent, err := prepareFactsArchiveParent(filepath.Join(dir, "new", "leaf"))
	if err != nil || parent != dir {
		t.Fatal("wrong actual archive parent")
	}
	if err = os.Symlink(dir, filepath.Join(dir, "redirect")); err != nil {
		t.Fatal(err)
	}
	if _, err = observePrepareFactsFS("archive", filepath.Join(dir, "redirect")); err == nil {
		t.Fatal("symlink accepted")
	}
	receipt := prepareFactsReceipt{FormatVersion: 1, Kind: "readonly_prepare_facts_observation", DiagnosticOnly: true, FactsObservationComplete: true}
	if receipt.Complete || receipt.DropReady || receipt.ExecutionAllowed || receipt.OrderedMongoSchemaSHA256 != "" {
		t.Fatal("capacity observation minted authority")
	}
}

func TestPrepareFactsRequiresOriginalCompleteInventoryAndExactFourSources(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	req := request{FormatVersion: 2, Kind: "readonly_inventory_request", OperationID: r.OperationID, SourceSHA: r.Inventory.SourceSHA,
		TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Limits: productionLimits(),
		Identities: map[string]string{"mysql": strings.Repeat("1", 64), "mongodb": strings.Repeat("2", 64)},
		Migrations: map[string]uint64{"mysql": 99, "mongodb": 38}, BoundaryRunID: "222-1", BoundaryReportHash: strings.Repeat("3", 64)}
	observed := report{FormatVersion: 2, Kind: "readonly_compatibility_inventory", SourceSHA: req.SourceSHA, OperationID: r.OperationID,
		RunID: r.Inventory.RunID, RequestHash: r.Inventory.RequestSHA256, TargetHash: digest(targets), Complete: true, DiagnosticOnly: true,
		BoundaryReportHash: req.BoundaryReportHash, ErrorCategory: "none", DatabaseBindings: map[string]databaseInventory{}}
	for _, db := range []string{"mysql", "mongodb"} {
		observed.DatabaseBindings[db] = databaseInventory{IdentityHash: req.Identities[db], DatabaseAnchorHash: strings.Repeat("4", 64), Version: req.Migrations[db], MetadataComplete: true, ExpectedIdentityMatch: true, ExpectedMigrationMatch: true, CatalogHash: strings.Repeat("5", 64)}
	}
	for i, target := range targets {
		b := targetBoundary{Database: target[0], Name: target[1], Kind: target[2], Present: true, Empty: true, SchemaHash: strings.Repeat("6", 64), IdentityHash: strings.Repeat("7", 64)}
		req.Boundaries = append(req.Boundaries, b)
		observed.Targets = append(observed.Targets, snapshot{Database: target[0], Name: target[1], Kind: target[2], Present: true, Complete: true, ErrorCategory: "none", Boundary: &b, Passes: 2, SourceFile: lifecycleSourceNames[i+3]})
	}
	if e := validatePrepareFactsInventory(r, req, observed); e != nil {
		t.Fatal(e)
	}
	mutations := []func(*report){func(v *report) { v.Complete = false }, func(v *report) { v.SourceSHA = sourceSHA }, func(v *report) { v.RequestHash = strings.Repeat("8", 64) }, func(v *report) { v.Targets[3].SourceFile = "wrong.source" }, func(v *report) { b := v.DatabaseBindings["mongodb"]; b.Dirty = true; v.DatabaseBindings["mongodb"] = b }, func(v *report) { v.Targets[0].Passes = 1 }}
	for _, mutate := range mutations {
		encoded, e := json.Marshal(observed)
		if e != nil {
			t.Fatal(e)
		}
		var changed report
		if e = json.Unmarshal(encoded, &changed); e != nil {
			t.Fatal(e)
		}
		mutate(&changed)
		if validatePrepareFactsInventory(r, req, changed) == nil {
			t.Fatal("unproven inventory accepted")
		}
	}
	req.Migrations["mysql"] = 100
	if validatePrepareFactsInventory(r, req, observed) == nil {
		t.Fatal("different schema phase accepted")
	}
}

func TestPrepareFactsRequestPathBindsActualRunWithoutChangingProducer(t *testing.T) {
	r := prepareFactsRequestFixture(t)
	path := "/opt/backups/qs-server/compatibility-retirement/123-1/prepare-facts-request-789-1.json"
	if !prepareFactsBoundRequestPath(path, r.OperationID, r.ActualRunID) {
		t.Fatal("exact actual run path rejected")
	}
	for _, invalid := range []struct{ path, op, run string }{
		{path, "123-1", "790-1"},
		{path, "124-1", "789-1"},
		{"/opt/backups/qs-server/compatibility-retirement/123-1/prepare-facts-request.json", "123-1", "789-1"},
		{path, "123-1", "../789-1"},
		{"/opt/backups/qs-server/compatibility-retirement/123-1/./prepare-facts-request-789-1.json", "123-1", "789-1"},
	} {
		if prepareFactsBoundRequestPath(invalid.path, invalid.op, invalid.run) {
			t.Fatal("conflicting run/path accepted")
		}
	}
	original := r.Inventory
	r.ActualRunID = "790-1"
	if validatePrepareFactsRequest(r, "123-1", "790-1") != nil || r.Inventory != original {
		t.Fatal("new actual run changed original inventory producer")
	}
	if validatePrepareFactsRequest(r, "123-1", "789-1") == nil {
		t.Fatal("request actual run conflict accepted")
	}
}
