package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

func testSource(t *testing.T) {
	t.Helper()
	old := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = old })
}
func privateTestDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal("platform temporary directory resolution failed")
	}
	d, err := os.MkdirTemp(root, "qs-history-private-test-")
	if err != nil {
		t.Fatal("owned private directory create failed")
	}
	t.Cleanup(func() {
		if os.RemoveAll(d) != nil {
			t.Error("owned private directory cleanup failed")
		}
	})
	if os.Chmod(d, 0700) != nil {
		t.Fatal("owned private fixture mode failed")
	}
	return d
}
func writeFixtureJSON(t *testing.T, path string, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil || os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("private JSON fixture failed")
	}
	return rawHash(raw)
}
func requestFixture(t *testing.T) (string, string, historyRequest) {
	t.Helper()
	d := privateTestDir(t)
	mysqlID, mongoID := strings.Repeat("b", 64), strings.Repeat("c", 64)
	inv := inventoryRequest{FormatVersion: 2, Kind: "readonly_inventory_request", OperationID: "123-1", SourceSHA: sourceSHA, TargetHash: jsonHash(historyTargets), DatabaseScope: "mysql-and-mongodb", Identities: map[string]string{"mysql": mysqlID, "mongodb": mongoID}, Migrations: map[string]uint64{"mysql": 99, "mongodb": 38}, Limits: inventoryLimits{30, 1500, 1_000_000, 2 << 30, 1000, 1001}, BoundaryRunID: "122-1", BoundaryReportHash: strings.Repeat("d", 64)}
	report := inventoryReport{FormatVersion: 2, Kind: "readonly_compatibility_inventory", SourceSHA: sourceSHA, OperationID: inv.OperationID, RunID: "124-1", TargetHash: inv.TargetHash, ObservedAt: "2026-10-09T01:02:03Z", Complete: true, DatabaseBindings: map[string]databaseInventory{}, SourceBytesProtocol: "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2", ConsistencySemantics: "two_equal_complete_passes_within_independently_approved_upper;sql_same_readonly_snapshot;mongo_homogeneous_bson_id_simple_collation;after_upper_next_cycle_not_fenced", ErrorCategory: "none", BoundaryReportHash: inv.BoundaryReportHash, DiagnosticOnly: true}
	for _, db := range []string{"mysql", "mongodb"} {
		identity := inv.Identities[db]
		b := databaseInventory{IdentityHash: identity, DatabaseAnchorHash: identity, ExpectedIdentityMatch: true, Version: inv.Migrations[db], ExpectedMigrationMatch: true, CatalogHash: strings.Repeat("e", 64), NonTargetSchemaHash: strings.Repeat("f", 64), MetadataComplete: true, Permissions: map[string]bool{}, DependencyScope: "diagnostic", ErrorCategory: "none"}
		if db == "mongodb" {
			b.MigrationGenerationHash = strings.Repeat("d", 64)
		}
		report.DatabaseBindings[db] = b
	}
	req := historyRequest{FormatVersion: 1, Kind: "readonly_compatibility_history_request", SourceSHA: sourceSHA, OperationID: inv.OperationID, RunID: "125-1"}
	for i, tgt := range historyTargets {
		kind := "uint64"
		if i == 1 || i == 2 {
			kind = "ascii_string"
		}
		if i == 3 {
			kind = ""
		}
		boundary := retirement.SourceBoundary{Database: tgt[0], Name: tgt[1], Kind: tgt[2], Present: true, Empty: true, PKType: kind, SchemaHash: strings.Repeat("e", 64), IdentityHash: strings.Repeat("f", 64)}
		inv.Boundaries = append(inv.Boundaries, boundary)
		filename := tgt[0] + "-" + tgt[1] + ".source"
		raw := []byte("private source fixture, not a source capability\n")
		if i == 3 {
			raw = nil
		}
		path := filepath.Join(d, filename)
		if os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("private asset fixture failed")
		}
		b := boundary
		report.Targets = append(report.Targets, inventorySnapshot{Database: tgt[0], Name: tgt[1], Kind: tgt[2], Present: true, Complete: true, SchemaHash: b.SchemaHash, DataHash: strings.Repeat("e", 64), IdentityHash: b.IdentityHash, Classification: map[string]uint64{}, SourceFile: filename, ErrorCategory: "none", Boundary: &b, Passes: 2})
		req.Assets = append(req.Assets, assetBinding{tgt[0], tgt[1], path, rawHash(raw), uint64(len(raw))})
	}
	invPath := filepath.Join(d, "inventory-request.json")
	invSHA := writeFixtureJSON(t, invPath, inv)
	report.RequestHash = invSHA
	reportPath := filepath.Join(d, "inventory.private.json")
	reportSHA := writeFixtureJSON(t, reportPath, report)
	req.InventoryRequest = fileBinding{invPath, invSHA}
	req.InventoryReport = fileBinding{reportPath, reportSHA}
	path := filepath.Join(d, "history-request.json")
	return path, writeFixtureJSON(t, path, req), req
}
func TestPrivateRequestFilesBoundAndEmptyMongoRetained(t *testing.T) {
	testSource(t)
	path, hash, _ := requestFixture(t)
	a, err := loadInputs(t.Context(), path, hash, "123-1", "125-1")
	if err != nil {
		t.Fatal(safeCategory(err))
	}
	defer func() {
		if a.close() != nil {
			t.Error("owned input close failed")
		}
	}()
	if !a.expected[3].Boundary.Present || !a.expected[3].Boundary.Empty || a.request.Assets[3].FullFileBytes != 0 {
		t.Fatal("present-empty source was treated as absent")
	}
	c, err := retirement.PrepareHistoricalCoordinator(t.Context(), retirement.HistoricalCoordinatorBinding{SourceSHA: sourceSHA, OperationID: "123-1"}, a.copies(), retirement.DefaultHistoricalCoordinatorLimits())
	if err == nil || c != nil {
		t.Fatal("hash-bound arbitrary assets fabricated source capability")
	}
	r := emptyReadiness(a)
	if r.DropReady || r.CompletedReadOnlyPipeline || r.IndependentProductionApprovalVerified || r.CASComplete || r.WriterFenceProven || r.FullExternalAIClosureVerified {
		t.Fatal("immutable input was mistaken for qualification/approval")
	}
}
func TestPrivateRequestAndAssetsFailClosed(t *testing.T) {
	for _, mutation := range []string{"source", "op", "run", "extra", "alias", "missing", "duplicate", "null", "badrawhash", "asset_changed", "asset_symlink", "asset_hardlink", "asset_world_read", "inventory_v1", "report_drop_ready", "report_head", "report_protocol", "unapproved_advance"} {
		t.Run(mutation, func(t *testing.T) {
			testSource(t)
			path, hash, r := requestFixture(t)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal("fixture read failed")
			}
			switch mutation {
			case "source":
				r.SourceSHA = strings.Repeat("b", 40)
				hash = writeFixtureJSON(t, path, r)
			case "op":
				r.OperationID = "123-2"
				hash = writeFixtureJSON(t, path, r)
			case "run":
				r.RunID = "125-2"
				hash = writeFixtureJSON(t, path, r)
			case "extra":
				raw = append(raw[:len(raw)-1], []byte(`,"complete":true}`)...)
			case "alias":
				raw = []byte(strings.Replace(string(raw), `"source_sha"`, `"SOURCE_SHA"`, 1))
			case "missing":
				var obj map[string]json.RawMessage
				if json.Unmarshal(raw, &obj) != nil {
					t.Fatal("fixture JSON")
				}
				delete(obj, "assets")
				raw, _ = json.Marshal(obj)
			case "duplicate":
				raw = append(raw[:len(raw)-1], []byte(`,"source_sha":"`+sourceSHA+`"}`)...)
			case "null":
				raw = []byte(strings.Replace(string(raw), `"format_version":1`, `"format_version":null`, 1))
			case "badrawhash":
				hash = strings.Repeat("0", 64)
			case "asset_changed":
				if os.WriteFile(r.Assets[0].Path, []byte("sensitive changed body"), 0600) != nil {
					t.Fatal("fixture mutation")
				}
			case "asset_symlink":
				if os.Remove(r.Assets[0].Path) != nil || os.Symlink(r.Assets[1].Path, r.Assets[0].Path) != nil {
					t.Fatal("fixture symlink")
				}
			case "asset_hardlink":
				if os.Link(r.Assets[0].Path, r.Assets[0].Path+".link") != nil {
					t.Fatal("fixture hardlink")
				}
			case "asset_world_read":
				if os.Chmod(r.Assets[0].Path, 0644) != nil {
					t.Fatal("fixture mode")
				}
			case "inventory_v1":
				var inv inventoryRequest
				if privateJSON(r.InventoryRequest, &inv) != nil {
					t.Fatal("fixture inventory")
				}
				inv.FormatVersion = 1
				r.InventoryRequest.SHA256 = writeFixtureJSON(t, r.InventoryRequest.Path, inv)
				hash = writeFixtureJSON(t, path, r)
			default:
				var report inventoryReport
				if privateJSON(r.InventoryReport, &report) != nil {
					t.Fatal("fixture report")
				}
				switch mutation {
				case "report_drop_ready":
					report.DropReady = true
				case "report_head":
					b := report.DatabaseBindings["mysql"]
					b.Version--
					report.DatabaseBindings["mysql"] = b
				case "report_protocol":
					report.SourceBytesProtocol = "v1"
				case "unapproved_advance":
					report.Targets[0].NextCycleRequired = true
				}
				r.InventoryReport.SHA256 = writeFixtureJSON(t, r.InventoryReport.Path, report)
				hash = writeFixtureJSON(t, path, r)
			}
			if mutation == "extra" || mutation == "alias" || mutation == "missing" || mutation == "duplicate" || mutation == "null" {
				if os.WriteFile(path, raw, 0600) != nil {
					t.Fatal("fixture JSON write")
				}
				hash = rawHash(raw)
			}
			a, e := loadInputs(t.Context(), path, hash, "123-1", "125-1")
			if e == nil || a != nil {
				if a != nil {
					_ = a.close()
				}
				t.Fatal("changed input was admitted")
			}
			if strings.Contains(e.Error(), "sensitive") {
				t.Fatal("private body leaked")
			}
		})
	}
}
func TestOpenedAssetInPlaceMutationRefused(t *testing.T) {
	testSource(t)
	path, hash, r := requestFixture(t)
	a, err := loadInputs(t.Context(), path, hash, "123-1", "125-1")
	if err != nil {
		t.Fatal(safeCategory(err))
	}
	defer func() { _ = a.close() }()
	if os.WriteFile(r.Assets[0].Path, []byte("private body changed"), 0600) != nil {
		t.Fatal("owned mutation failed")
	}
	if a.verifyFullFiles(t.Context()) == nil {
		t.Fatal("open FD stale content accepted")
	}
}
func TestCLIRejectsWriteModesAndLeaksNoArguments(t *testing.T) {
	for _, args := range [][]string{{"--cas", "secret-token"}, {"--drop", "private-password"}, {"--request", "private", "--request", "other"}, {"--request=private"}, {"--source-sha", "bad"}, {"--request", "x"}} {
		if _, err := parseFlags(args); err == nil || safeCategory(err) != "history_arguments_rejected" {
			t.Fatal("unsafe/malformed CLI accepted")
		}
	}
	if safeCategory(errors.New("private-password in DSN")) != "history_unclassified_failure" {
		t.Fatal("untrusted driver error leaked")
	}
	r, err := runCLI(context.Background(), []string{"--drop", "private-password"})
	if err == nil || r.DropReady || r.CASComplete || r.WriterFenceProven {
		t.Fatal("write entrypoint created")
	}
	raw, e := json.Marshal(r)
	if e != nil || strings.Contains(string(raw), "private-password") {
		t.Fatal("receipt contains raw argument")
	}
}
func TestReadinessOExclAndFalseBoundaries(t *testing.T) {
	d := privateTestDir(t)
	r := emptyReadiness(nil)
	if writeReadiness(d, r) != nil {
		t.Fatal("private receipt write")
	}
	if writeReadiness(d, r) == nil {
		t.Fatal("receipt overwrite permitted")
	}
	raw, err := os.ReadFile(filepath.Join(d, "history.readiness.json"))
	if err != nil {
		t.Fatal("receipt read")
	}
	var decoded readiness
	if strictDecode(raw, &decoded) != nil {
		t.Fatal("receipt schema")
	}
	if decoded.DistributedAtomicSnapshot || decoded.MutationBackendEnabled || decoded.DropReady || decoded.BackupRestoreQualified || decoded.HostProcessBudgetProven || decoded.OrderedMongoSourceMetadataApproved {
		t.Fatal("readonly receipt promoted final gates")
	}
}

func TestOpenedAssetPathReplacementAndMetadataMutationRefused(t *testing.T) {
	for _, mutation := range []string{"rename", "approved_metadata"} {
		t.Run(mutation, func(t *testing.T) {
			testSource(t)
			path, hash, r := requestFixture(t)
			a, err := loadInputs(t.Context(), path, hash, "123-1", "125-1")
			if err != nil {
				t.Fatal(safeCategory(err))
			}
			defer func() { _ = a.close() }()
			if mutation == "rename" {
				tmp := r.Assets[0].Path + ".replacement"
				original, err := os.ReadFile(r.Assets[0].Path)
				if err != nil || os.WriteFile(tmp, original, 0600) != nil || os.Rename(tmp, r.Assets[0].Path) != nil {
					t.Fatal("owned atomic replacement")
				}
			} else {
				if os.WriteFile(r.InventoryReport.Path, []byte(`{"complete":true,"private":"secret"}`), 0600) != nil {
					t.Fatal("owned metadata mutation")
				}
			}
			if a.verifyFullFiles(t.Context()) == nil {
				t.Fatal("old FD or previously parsed metadata bypassed current asset binding")
			}
		})
	}
}

func TestReadOnlyFinalReceiptIncludesCloseResults(t *testing.T) {
	for _, tc := range []struct {
		name, expected            string
		pipeline, database, input error
	}{
		{name: "success", expected: "none"},
		{name: "database", database: errors.New("private DSN"), expected: "history_connection_close_failed"},
		{name: "input", input: errors.New("private path"), expected: "history_private_close_failed"},
		{name: "both", database: errors.New("private DSN"), input: errors.New("private path"), expected: "history_connection_close_failed"},
		{name: "pipeline_and_database", pipeline: fixedError("history_joint_page_consumption_failed"), database: errors.New("private DSN"), expected: "history_joint_page_consumption_failed"},
		{name: "pipeline_and_input", pipeline: fixedError("history_joint_page_consumption_failed"), input: errors.New("private path"), expected: "history_joint_page_consumption_failed"},
		{name: "pipeline_and_both", pipeline: fixedError("history_joint_page_consumption_failed"), database: errors.New("private DSN"), input: errors.New("private path"), expected: "history_joint_page_consumption_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := privateTestDir(t)
			var order []string
			closeResource := func(name string, result error) func() error {
				return func() error {
					if _, err := os.Stat(filepath.Join(dir, "history.readiness.json")); !os.IsNotExist(err) {
						t.Fatal("private receipt preceded resource close")
					}
					order = append(order, name)
					return result
				}
			}
			cleanup := readOnlyCleanup(closeResource("database", tc.database), closeResource("inputs", tc.input))
			r, err := finishReadOnlyCLI(dir, emptyReadiness(nil), tc.pipeline, cleanup)
			if safeCategory(err) != tc.expected || r.ErrorCategory != tc.expected || strings.Join(order, ",") != "database,inputs" {
				t.Fatal("final classification/order lost close outcome or pipeline error")
			}
			if cleanup() != nil || strings.Join(order, ",") != "database,inputs" {
				t.Fatal("deferred fallback closed resources twice")
			}
			raw, readErr := os.ReadFile(filepath.Join(dir, "history.readiness.json"))
			var saved readiness
			if readErr != nil || strictDecode(raw, &saved) != nil || saved.ErrorCategory != tc.expected || saved.ErrorCategory != r.ErrorCategory {
				t.Fatal("private receipt disagrees with returned fixed classification")
			}
			if strings.Contains(string(raw), "private DSN") || strings.Contains(string(raw), "private path") || saved.DropReady || saved.CASComplete || saved.MutationBackendEnabled {
				t.Fatal("close failure exposed secrets or promoted authority")
			}
		})
	}
}

func TestReadOnlyCleanupEarlyReturnAndBothAttempts(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		primary, database, input error
		expected                 string
	}{
		{name: "primary_preserved", primary: fixedError("history_sql_connect_failed"), database: errors.New("private DSN"), input: errors.New("private path"), expected: "history_sql_connect_failed"},
		{name: "cleanup_database", database: errors.New("private DSN"), input: errors.New("private path"), expected: "history_connection_close_failed"},
		{name: "cleanup_input", input: errors.New("private path"), expected: "history_private_close_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var order []string
			cleanup := readOnlyCleanup(func() error { order = append(order, "database"); return tc.database }, func() error { order = append(order, "inputs"); return tc.input })
			err := func() (result error) {
				defer func() {
					if err := cleanup(); result == nil && err != nil {
						result = err
					}
				}()
				return tc.primary
			}()
			if safeCategory(err) != tc.expected || strings.Join(order, ",") != "database,inputs" || cleanup() != nil || strings.Join(order, ",") != "database,inputs" {
				t.Fatal("early return dropped primary/cleanup error or leaked/reclosed resources")
			}
		})
	}
}
