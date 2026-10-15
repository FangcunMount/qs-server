package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func offlineApprovalFixture(t *testing.T, snapshot OfflineSnapshot) (OfflineConfig, offlineAuthorization) {
	t.Helper()
	dir := t.TempDir()
	path, err := filepath.EvalSymlinks(filepath.Join(dir, ".."))
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(path, filepath.Base(dir), "approval.json")
	a := offlineAuthorization{
		FormatVersion: 1, Kind: "qs_schema_pair_offline_upgrade", OperationID: "123456789-1",
		ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), WindowStartedAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		ApprovedSourceSHA: snapshot.SourceSHA, MigrationResourcesSHA256: snapshot.MigrationResourcesSHA256,
		MySQLDatabase: snapshot.MySQLDatabase, MongoDatabase: snapshot.MongoDatabase,
		MySQLIdentitySHA256: snapshot.MySQLIdentitySHA256, MongoClusterSHA256: snapshot.MongoClusterSHA256,
		InitialMySQLVersion: snapshot.MySQLVersion, InitialMongoVersion: snapshot.MongoVersion,
		TargetMySQLVersion: snapshot.TargetMySQLVersion, TargetMongoVersion: snapshot.TargetMongoVersion,
		ApprovalSummarySHA256: strings.Repeat("b", 64), FreezeScopeSHA256: strings.Repeat("c", 64),
		ProtectionScope: offlineProtectionFullDatabasePair, ProtectionManifestSHA256: strings.Repeat("d", 64), RestoreReceiptSHA256: strings.Repeat("e", 64), StartupReceiptSHA256: strings.Repeat("e", 64), ResponsibilitySHA256: strings.Repeat("f", 64),
		ProtectionLocation: "isolated-fixture://not-production-evidence", DataOwner: "fixture-owner", RecoveryOwner: "fixture-recovery",
		RuntimeFrozen: true, RuntimeNotStarted: true, RestoreDurationSeconds: 60, StartupReserveSeconds: 300, BackupRetentionDays: 30,
	}
	cfg := OfflineConfig{AuthorizationFile: path}
	writeOfflineApproval(t, &cfg, a)
	return cfg, a
}

func writeOfflineApproval(t *testing.T, cfg *OfflineConfig, a offlineAuthorization) {
	t.Helper()
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.AuthorizationFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.AuthorizationSHA256 = retirementHash(raw)
}

func TestOfflineApprovalRequiresBoundScopeAndRecovery(t *testing.T) {
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	s := OfflineSnapshot{SourceSHA: strings.Repeat("a", 40), MigrationResourcesSHA256: r.sha256, MySQLDatabase: "qs", MongoDatabase: "qs", MySQLIdentitySHA256: strings.Repeat("b", 64), MongoClusterSHA256: strings.Repeat("c", 64), MySQLVersion: 100, MongoVersion: 39, TargetMySQLVersion: r.sql.head, TargetMongoVersion: r.mongo.head}
	cfg, a := offlineApprovalFixture(t, s)
	if _, err := readOfflineAuthorization(cfg, s); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(*offlineAuthorization)
	}{
		{"source", func(a *offlineAuthorization) { a.ApprovedSourceSHA = strings.Repeat("d", 40) }},
		{"resources", func(a *offlineAuthorization) { a.MigrationResourcesSHA256 = strings.Repeat("d", 64) }},
		{"identity", func(a *offlineAuthorization) { a.MySQLIdentitySHA256 = strings.Repeat("d", 64) }},
		{"mongo_identity", func(a *offlineAuthorization) { a.MongoClusterSHA256 = strings.Repeat("d", 64) }},
		{"database", func(a *offlineAuthorization) { a.MongoDatabase = "another" }},
		{"initial", func(a *offlineAuthorization) { a.InitialMySQLVersion++ }},
		{"target", func(a *offlineAuthorization) { a.TargetMongoVersion++ }},
		{"purpose", func(a *offlineAuthorization) { a.Kind = "qs_schema_pair_pristine_bootstrap" }},
		{"format", func(a *offlineAuthorization) { a.FormatVersion = 2 }},
		{"expired", func(a *offlineAuthorization) { a.ExpiresAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339) }},
		{"stale_window", func(a *offlineAuthorization) {
			a.WindowStartedAt = time.Now().Add(-31 * time.Minute).UTC().Format(time.RFC3339)
		}},
		{"future_window", func(a *offlineAuthorization) {
			a.WindowStartedAt = time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
		}},
		{"unfrozen", func(a *offlineAuthorization) { a.RuntimeFrozen = false }},
		{"runtime_started", func(a *offlineAuthorization) { a.RuntimeNotStarted = false }},
		{"location", func(a *offlineAuthorization) { a.ProtectionLocation = "unknown" }},
		{"data_owner", func(a *offlineAuthorization) { a.DataOwner = "" }},
		{"recovery_owner", func(a *offlineAuthorization) { a.RecoveryOwner = "pending" }},
		{"protection_manifest", func(a *offlineAuthorization) { a.ProtectionManifestSHA256 = "" }},
		{"restore", func(a *offlineAuthorization) { a.RestoreReceiptSHA256 = strings.Repeat("0", 64) }},
		{"freeze", func(a *offlineAuthorization) { a.FreezeScopeSHA256 = "" }},
		{"responsibility", func(a *offlineAuthorization) { a.ResponsibilitySHA256 = "" }},
		{"duration_missing", func(a *offlineAuthorization) { a.RestoreDurationSeconds = 0 }},
		{"duration_over_budget", func(a *offlineAuthorization) { a.RestoreDurationSeconds = 3601 }},
		{"retention", func(a *offlineAuthorization) { a.BackupRetentionDays = 29 }},
		{"startup_receipt", func(a *offlineAuthorization) { a.StartupReceiptSHA256 = "" }},
		{"startup_reserve", func(a *offlineAuthorization) { a.StartupReserveSeconds = 0 }},
		{"startup_budget", func(a *offlineAuthorization) { a.StartupReserveSeconds = 1800 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := a
			test.edit(&changed)
			writeOfflineApproval(t, &cfg, changed)
			if _, err := readOfflineAuthorization(cfg, s); err == nil {
				t.Fatal("unsafe approval accepted")
			}
		})
	}
}

func TestOfflineApprovalPrivateFileSchemaAndExclusiveConsumption(t *testing.T) {
	s := OfflineSnapshot{SourceSHA: strings.Repeat("a", 40), MigrationResourcesSHA256: strings.Repeat("b", 64)}
	cfg, a := offlineApprovalFixture(t, s)
	for _, suffix := range []string{`,"unexpected":true}`, `,"runtime_frozen":true}`, `,"backup_pair_manifest_sha256":"` + strings.Repeat("d", 64) + `"}`, `,"backup_pair_location":"old-pair"}`} {
		raw, _ := json.Marshal(a)
		raw = append(raw[:len(raw)-1], []byte(suffix)...)
		if err := os.WriteFile(cfg.AuthorizationFile, raw, 0600); err != nil {
			t.Fatal(err)
		}
		cfg.AuthorizationSHA256 = retirementHash(raw)
		if _, err := readOfflineAuthorization(cfg, s); err == nil {
			t.Fatal("unknown/duplicate field accepted")
		}
	}
	writeOfflineApproval(t, &cfg, a)
	if err := os.Chmod(cfg.AuthorizationFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readOfflineAuthorization(cfg, s); err == nil {
		t.Fatal("public approval accepted")
	}
	if err := os.Chmod(cfg.AuthorizationFile, 0600); err != nil {
		t.Fatal(err)
	}
	link := cfg.AuthorizationFile + ".link"
	if err := os.Link(cfg.AuthorizationFile, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readOfflineAuthorization(cfg, s); err == nil {
		t.Fatal("hard-linked approval accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := claimPrivateRetirementAuthorization(cfg.AuthorizationFile, a.OperationID, cfg.AuthorizationSHA256); err != nil {
		t.Fatal(err)
	}
	if _, err := readOfflineAuthorization(cfg, s); err == nil {
		t.Fatal("consumed approval accepted")
	}
	if err := claimPrivateRetirementAuthorization(cfg.AuthorizationFile, a.OperationID, cfg.AuthorizationSHA256); err == nil {
		t.Fatal("duplicate consumption accepted")
	}
}

func TestOfflineApprovalProtectionScopesBindExactVersionTransitions(t *testing.T) {
	for _, test := range []struct {
		name                     string
		scope                    string
		initialSQL, initialMongo uint
		targetSQL, targetMongo   uint
		accepted                 bool
	}{
		{"full_current_noop", offlineProtectionFullDatabasePair, 100, 39, 100, 39, true},
		{"full_combined_upgrade", offlineProtectionFullDatabasePair, 100, 39, 101, 40, true},
		{"sql_only", offlineProtectionSQLHistoricalColumns, 100, 39, 101, 39, true},
		{"mongo_only", offlineProtectionMongoHistoricalIndexes, 101, 39, 101, 40, true},
		{"maintenance_only", offlineProtectionLegacyMaintenanceTables, 102, 40, 103, 40, true},
		{"maintenance_noop", offlineProtectionLegacyMaintenanceTables, 103, 40, 103, 40, false},
		{"maintenance_before_actor", offlineProtectionLegacyMaintenanceTables, 101, 40, 103, 40, false},
		{"maintenance_before_mongo", offlineProtectionLegacyMaintenanceTables, 102, 39, 103, 40, false},
		{"maintenance_changes_mongo", offlineProtectionLegacyMaintenanceTables, 102, 40, 103, 41, false},
		{"maintenance_future_sql", offlineProtectionLegacyMaintenanceTables, 102, 40, 104, 40, false},
		{"maintenance_down", offlineProtectionLegacyMaintenanceTables, 103, 40, 102, 40, false},
		{"actor_cannot_authorize_maintenance", offlineProtectionActorColumns, 102, 40, 103, 40, false},
		{"sql_cannot_authorize_maintenance", offlineProtectionSQLHistoricalColumns, 102, 40, 103, 40, false},
		{"mongo_cannot_authorize_maintenance", offlineProtectionMongoHistoricalIndexes, 102, 40, 103, 40, false},
		{"actor_only", offlineProtectionActorColumns, 101, 40, 102, 40, true},
		{"actor_noop", offlineProtectionActorColumns, 102, 40, 102, 40, false},
		{"actor_before_mongo", offlineProtectionActorColumns, 101, 39, 102, 40, false},
		{"actor_skips_sql", offlineProtectionActorColumns, 100, 40, 102, 40, false},
		{"actor_changes_mongo", offlineProtectionActorColumns, 101, 40, 102, 41, false},
		{"actor_future_sql", offlineProtectionActorColumns, 101, 40, 103, 40, false},
		{"actor_down", offlineProtectionActorColumns, 102, 40, 101, 40, false},
		{"sql_cannot_authorize_actor", offlineProtectionSQLHistoricalColumns, 101, 40, 102, 40, false},
		{"mongo_cannot_authorize_actor", offlineProtectionMongoHistoricalIndexes, 101, 40, 102, 40, false},
		{"missing_scope", "", 100, 39, 101, 39, false},
		{"unknown_scope", "m4_actor", 100, 39, 101, 39, false},
		{"scope_whitespace", "full_database_pair ", 100, 39, 101, 39, false},
		{"sql_combined_upgrade", offlineProtectionSQLHistoricalColumns, 100, 39, 101, 40, false},
		{"sql_noop", offlineProtectionSQLHistoricalColumns, 100, 39, 100, 39, false},
		{"sql_changed_initial", offlineProtectionSQLHistoricalColumns, 101, 39, 102, 39, false},
		{"sql_skip_version", offlineProtectionSQLHistoricalColumns, 100, 39, 102, 39, false},
		{"sql_partial_pair", offlineProtectionSQLHistoricalColumns, 100, 38, 101, 39, false},
		{"sql_other_mongo", offlineProtectionSQLHistoricalColumns, 100, 40, 101, 40, false},
		{"mongo_wrong_initial", offlineProtectionMongoHistoricalIndexes, 100, 39, 101, 40, false},
		{"mongo_changes_sql", offlineProtectionMongoHistoricalIndexes, 101, 39, 102, 40, false},
		{"mongo_skip_version", offlineProtectionMongoHistoricalIndexes, 101, 39, 101, 41, false},
		{"mongo_noop", offlineProtectionMongoHistoricalIndexes, 101, 40, 101, 40, false},
		{"mongo_down", offlineProtectionMongoHistoricalIndexes, 101, 40, 101, 39, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Bind the candidate versions to the snapshot as well, so rejecting
			// an object scope cannot accidentally rely on a separate mismatch.
			s := OfflineSnapshot{
				SourceSHA: strings.Repeat("a", 40), MigrationResourcesSHA256: strings.Repeat("b", 64),
				MySQLDatabase: "qs", MongoDatabase: "qs", MySQLIdentitySHA256: strings.Repeat("c", 64), MongoClusterSHA256: strings.Repeat("d", 64),
				MySQLVersion: test.initialSQL, MongoVersion: test.initialMongo,
				TargetMySQLVersion: test.targetSQL, TargetMongoVersion: test.targetMongo,
			}
			cfg, a := offlineApprovalFixture(t, s)
			a.ProtectionScope = test.scope
			writeOfflineApproval(t, &cfg, a)
			_, err := readOfflineAuthorization(cfg, s)
			if (err == nil) != test.accepted {
				t.Fatalf("scope accepted=%v, want %v: %v", err == nil, test.accepted, err)
			}
			if _, err := os.Lstat(cfg.AuthorizationFile + ".consumed"); !os.IsNotExist(err) {
				t.Fatalf("approval validation consumed authorization: %v", err)
			}
		})
	}
}
