//go:build integration

package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	operatorDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	testeeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	actor "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	driver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const maintenanceTablesUp = "migrations/mysql/000103_retire_legacy_maintenance_tables.up.sql"

func nativeLegacyMaintenancePair(t *testing.T) nativeSchemaPair {
	t.Helper()
	n := nativeActorColumnPair(t)
	// One owned SQL session keeps temporal capture/replay in the same zone.
	n.db.SetMaxOpenConns(1)
	n.db.SetMaxIdleConns(1)
	r := migrationResourcesThrough(t, 102, 40)
	n.runBoth(t, n.preflight(t, r))
	seedHistoricalEvidenceRows(t, n)
	for _, statement := range []string{
		`INSERT INTO testee_store_migration_manifests(migration_id,state,before_hash,after_hash,rollback_hash,items_hash,item_count,actor_id,created_at,completed_at,rolled_back_at) VALUES('owned','rolled_back',REPEAT('a',64),REPEAT('b',64),REPEAT('c',64),REPEAT('d',64),2,900009,'2026-10-13','2026-10-13','2026-10-13')`,
		`INSERT INTO testee_store_migration_items(migration_id,testee_id,org_id,before_store_id,target_store_id,before_version,applied_version,history_id,disposition,reason) VALUES('owned',7,1,NULL,1,1,2,1001,'candidate','synthetic'),('owned',8,1,NULL,NULL,1,1,NULL,'deferred','synthetic')`,
		`INSERT INTO testee_store_migration_reversions(migration_id,testee_id,org_id,from_store_id,original_history_id,restored_version,actor_id,created_at) VALUES('owned',7,1,1,1001,3,900009,'2026-10-13')`,
		`INSERT INTO operator_retirement_manifests(migration_id,stage,payload,payload_sha256,created_at,updated_at) VALUES('owned','applied_unchanged','{"synthetic":"original bytes"}',REPEAT('e',64),'2026-10-13','2026-10-13')`,
		`INSERT INTO clinician_operator_binding_archive(clinician_id,org_id,operator_id,migration_id,before_record,record_sha256,archived_at) VALUES(70,1,7,'owned','{"id":70,"org_id":1,"operator_id":7}',REPEAT('f',64),'2026-10-13')`,
	} {
		if _, err := n.db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

// Fixed disposable-fixture material only. This is not a production backup or
// restore entrypoint; its original bytes are frozen before the first real DROP.
type maintenanceTableMaterial struct {
	Name    string
	DDL     [][]sql.NullString
	Columns []string
	Rows    [][]sql.NullString
}

func maintenanceMaterial(t *testing.T, n nativeSchemaPair, name string) maintenanceTableMaterial {
	t.Helper()
	m := maintenanceTableMaterial{Name: name, DDL: historicalQuery(t, n.db, "SHOW CREATE TABLE `"+name+"`")}
	var selected []string
	for _, row := range historicalQuery(t, n.db, "SELECT COLUMN_NAME,DATA_TYPE FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='"+name+"' ORDER BY ORDINAL_POSITION") {
		m.Columns = append(m.Columns, row[0].String)
		column := "`" + row[0].String + "`"
		// Preserve the server's original temporal text. parseTime would turn it
		// into client-zone time.Time/RFC3339, which is not an exact SQL replay.
		switch row[1].String {
		case "date", "datetime", "timestamp", "time":
			column = "CAST(" + column + " AS CHAR)"
		}
		selected = append(selected, column)
	}
	var primary []string
	for _, row := range historicalQuery(t, n.db, "SELECT COLUMN_NAME FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='"+name+"' AND index_name='PRIMARY' ORDER BY SEQ_IN_INDEX") {
		primary = append(primary, "`"+row[0].String+"`")
	}
	if len(primary) == 0 {
		t.Fatal("fixed maintenance fixture requires deterministic primary keys", name)
	}
	m.Rows = historicalQuery(t, n.db, "SELECT "+strings.Join(selected, ",")+" FROM `"+name+"` ORDER BY "+strings.Join(primary, ","))
	return m
}

func maintenanceRetained(t *testing.T, n nativeSchemaPair) []maintenanceTableMaterial {
	t.Helper()
	excluded := map[string]bool{"schema_migrations": true}
	for _, name := range retiredMaintenanceSQLTables {
		excluded[name] = true
	}
	var materials []maintenanceTableMaterial
	for _, row := range historicalQuery(t, n.db, "SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY TABLE_NAME") {
		if name := row[0].String; !excluded[name] {
			materials = append(materials, maintenanceMaterial(t, n, name))
		}
	}
	return materials
}

func assertMaintenanceTables(t *testing.T, n nativeSchemaPair, present int) {
	t.Helper()
	for i, name := range retiredMaintenanceSQLTables {
		var count int
		if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", name).Scan(&count); err != nil || (count == 1) != (i >= len(retiredMaintenanceSQLTables)-present) {
			t.Fatal("maintenance table presence differs", name, err)
		}
	}
}

func maintenanceOfflineApproval(t *testing.T, n nativeSchemaPair, r *migrationResources) OfflineConfig {
	t.Helper()
	cfg := offlineNativeApproval(t, n, r)
	_, snapshot, err := inspectOffline(t.Context(), n.db, n.client, cfg.Pair, r)
	if err != nil {
		t.Fatal(err)
	}
	a, err := readOfflineAuthorization(cfg, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	a.ProtectionScope = offlineProtectionLegacyMaintenanceTables
	writeOfflineApproval(t, &cfg, a)
	return cfg
}

func TestLegacyMaintenanceNativeUpgradeRestartAndCurrentWrites(t *testing.T) {
	n := nativeLegacyMaintenancePair(t)
	before := maintenanceRetained(t, n)
	var originalCount int
	if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE'").Scan(&originalCount); err != nil {
		t.Fatal(err)
	}
	head, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Raw()
	if err != nil {
		t.Fatal(err)
	}
	r := migrationResourcesThrough(t, 103, 40)
	cfg := maintenanceOfflineApproval(t, n, r)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
	if err != nil || !result.Verified || !result.AuthorizationConsumed || result.MySQLChanged == nil || !*result.MySQLChanged || result.MongoChanged == nil || *result.MongoChanged {
		t.Fatal("maintenance offline upgrade failed", result, err)
	}
	assertMaintenanceTables(t, n, 0)
	var currentCount int
	if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE'").Scan(&currentCount); err != nil || currentCount != originalCount-5 || !reflect.DeepEqual(before, maintenanceRetained(t, n)) {
		t.Fatal("maintenance retirement changed more than the five original tables", err)
	}
	after, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Raw()
	if err != nil || !bytes.Equal(head, after) {
		t.Fatal("SQL-only maintenance retirement changed Mongo metadata", err)
	}
	restart := n.preflight(t, r)
	n.runBoth(t, restart)
	old := migrationResourcesThrough(t, 102, 40)
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, old); err == nil {
		t.Fatal("old source accepted SQL103")
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: n.db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil || actor.RequireCurrentOperatorPlanSchema(t.Context(), db) != nil {
		t.Fatal("current plans rejected clean SQL103", err)
	}
	op := operatorDomain.NewOperator(1, 800010, "new synthetic identity")
	op.SetID(10)
	if err := actor.NewOperatorRepository(db).Save(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	subject := testeeDomain.NewTestee(1, "new synthetic fact", testeeDomain.GenderUnknown, nil)
	subject.SetID(10)
	if err := actor.NewTesteeRepository(db).Save(t.Context(), subject); err != nil {
		t.Fatal(err)
	}
	if err := n.preflight(t, r).VerifyComplete(t.Context()); err != nil {
		t.Fatal(err)
	}
	// These new facts are never covered by any prior object or ledger replay.
}

func failedMaintenanceResources(t *testing.T) *migrationResources {
	t.Helper()
	r := migrationResourcesThrough(t, 103, 40)
	source := r.source.(fstest.MapFS)
	source[maintenanceTablesUp].Data = bytes.Replace(source[maintenanceTablesUp].Data, []byte("DROP TABLE `testee_store_migration_manifests`"), []byte("DROP TABLE `owned_missing_maintenance_table`"), 1)
	failed, err := readMigrationResources(source, r.pairs)
	if err != nil {
		t.Fatal(err)
	}
	return failed
}

func failMaintenanceMigration(t *testing.T, n nativeSchemaPair, failed *migrationResources) *PairPreflight {
	t.Helper()
	p := n.preflight(t, failed)
	result, err := runOffline(t.Context(), n.db, n.client, maintenanceOfflineApproval(t, n, failed), failed)
	if err == nil || result.Verified || result.Stage != "mysql" || !result.AuthorizationConsumed {
		t.Fatal("genuine third DROP did not retain failure", result, err)
	}
	assertMaintenanceTables(t, n, 3)
	if sqlVersion, dirty, mongoVersion, mongoDirty := n.heads(t); sqlVersion != 103 || !dirty || mongoVersion != 40 || mongoDirty {
		t.Fatal("partial maintenance DROP did not retain103dirty/40clean")
	}
	return p
}

func TestLegacyMaintenanceNativePartialDDLRefusesRestartForceAndDown(t *testing.T) {
	n := nativeLegacyMaintenancePair(t)
	before := maintenanceRetained(t, n)
	failed := failedMaintenanceResources(t)
	p := failMaintenanceMigration(t, n, failed)
	if !reflect.DeepEqual(before, maintenanceRetained(t, n)) {
		t.Fatal("partial DROP changed retained facts")
	}
	for _, r := range []*migrationResources{migrationResourcesThrough(t, 102, 40), migrationResourcesThrough(t, 103, 40)} {
		if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
			t.Fatal("fresh source adopted dirty maintenance metadata")
		}
	}
	d := NewMySQLDriver(n.db)
	instance, err := d.CreateInstance(failed.source, ensureConfigDefaults(p.MySQLConfig(false)))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{102, 103} {
		if instance.Force(version) == nil {
			t.Fatal("force accepted dirty maintenance DDL")
		}
	}
	if err := d.finishRun(); err != nil {
		t.Fatal(err)
	}
	down, err := migrations.ReadFile("migrations/mysql/000103_retire_legacy_maintenance_tables.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.db.ExecContext(t.Context(), string(down))
	var refusal *driver.MySQLError
	if !errors.As(err, &refusal) || string(refusal.SQLState[:]) != "45000" {
		t.Fatal("maintenance down did not refuse", err)
	}
	if sqlVersion, dirty, _, _ := n.heads(t); sqlVersion != 103 || !dirty {
		t.Fatal("refused operations changed the dirty ledger")
	}
}

type maintenanceObjectBackup struct {
	SQLIdentity, MongoIdentity, InitialResources, FailedResources string
	SessionTimeZone                                               string
	Ledger, Objects                                               []maintenanceTableMaterial
	Retained                                                      []maintenanceTableMaterial
	MongoHead                                                     []byte
}

func captureMaintenanceObjects(t *testing.T, n nativeSchemaPair, failed *migrationResources) []byte {
	t.Helper()
	initial := migrationResourcesThrough(t, 102, 40)
	if err := n.preflight(t, initial).VerifyComplete(t.Context()); err != nil {
		t.Fatal(err)
	}
	sqlIdentity, mongoIdentity, err := pairIdentity(t.Context(), n.db, n.client, n.cfg.MySQLDatabase, n.cfg.MongoDatabase)
	if err != nil {
		t.Fatal(err)
	}
	head, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Raw()
	if err != nil {
		t.Fatal(err)
	}
	var zone string
	if err := n.db.QueryRowContext(t.Context(), "SELECT @@session.time_zone").Scan(&zone); err != nil || zone == "" {
		t.Fatal("owned temporal session zone unknown", err)
	}
	b := maintenanceObjectBackup{SessionTimeZone: zone, SQLIdentity: sqlIdentity, MongoIdentity: mongoIdentity, InitialResources: initial.sha256, FailedResources: failed.sha256, Ledger: []maintenanceTableMaterial{maintenanceMaterial(t, n, "schema_migrations")}, Retained: maintenanceRetained(t, n), MongoHead: head}
	for _, name := range retiredMaintenanceSQLTables {
		b.Objects = append(b.Objects, maintenanceMaterial(t, n, name))
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type maintenanceRestoreFault uint8

const (
	maintenanceRestoreNoFault maintenanceRestoreFault = iota
	maintenanceRestoreDuplicateObjectRow
	maintenanceRestoreBeforeLedger
	maintenanceRestoreDuplicateLedgerRow
)

func replayMaintenanceObjects(t *testing.T, n nativeSchemaPair, raw []byte, approved [32]byte, failed *migrationResources, runtimeNotStarted bool, fault maintenanceRestoreFault) (err error) {
	t.Helper()
	if !runtimeNotStarted || sha256.Sum256(raw) != approved {
		return errors.New("maintenance object replay scope or fingerprint rejected")
	}
	var b maintenanceObjectBackup
	if json.Unmarshal(raw, &b) != nil || b.InitialResources != migrationResourcesThrough(t, 102, 40).sha256 || b.FailedResources != failed.sha256 || len(b.Objects) != 5 || len(b.Ledger) != 1 {
		return errors.New("maintenance original material resources rejected")
	}
	sqlIdentity, mongoIdentity, err := pairIdentity(t.Context(), n.db, n.client, n.cfg.MySQLDatabase, n.cfg.MongoDatabase)
	if err != nil || sqlIdentity != b.SQLIdentity || mongoIdentity != b.MongoIdentity {
		return errors.New("maintenance backup identity rejected")
	}
	var zone string
	if err := n.db.QueryRowContext(t.Context(), "SELECT @@session.time_zone").Scan(&zone); err != nil || zone == "" || zone != b.SessionTimeZone {
		return errors.New("maintenance temporal session zone changed")
	}
	head, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Raw()
	sqlVersion, dirty, mongoVersion, mongoDirty := n.heads(t)
	ledger := maintenanceMaterial(t, n, "schema_migrations")
	if err != nil || sqlVersion != 103 || !dirty || mongoVersion != 40 || mongoDirty || !bytes.Equal(head, b.MongoHead) || !reflect.DeepEqual(ledger.DDL, b.Ledger[0].DDL) || !reflect.DeepEqual(ledger.Columns, b.Ledger[0].Columns) || len(ledger.Rows) != 1 || !reflect.DeepEqual(b.Retained, maintenanceRetained(t, n)) {
		return errors.New("maintenance failed pair or retained facts rejected")
	}
	assertMaintenanceTables(t, n, 3)
	for i, object := range b.Objects {
		if object.Name != retiredMaintenanceSQLTables[i] || len(object.DDL) != 1 || len(object.DDL[0]) != 2 {
			return errors.New("maintenance original table shape rejected")
		}
		if i >= 2 && !reflect.DeepEqual(object, maintenanceMaterial(t, n, object.Name)) {
			return errors.New("unretired maintenance object changed")
		}
	}
	for i, object := range b.Objects[:2] {
		if _, err = n.db.ExecContext(t.Context(), object.DDL[0][1].String); err != nil {
			return err
		}
		columns := "`" + strings.Join(object.Columns, "`,`") + "`"
		insert := "INSERT INTO `" + object.Name + "`(" + columns + ") VALUES(" + strings.TrimSuffix(strings.Repeat("?,", len(object.Columns)), ",") + ")"
		for _, row := range object.Rows {
			values := make([]any, len(row))
			for j, value := range row {
				values[j] = value
			}
			if _, err = n.db.ExecContext(t.Context(), insert, values...); err != nil {
				return err
			}
			if i == 0 && fault == maintenanceRestoreDuplicateObjectRow {
				_, err = n.db.ExecContext(t.Context(), insert, values...)
				return err
			}
		}
	}
	for _, object := range b.Objects {
		if !reflect.DeepEqual(object, maintenanceMaterial(t, n, object.Name)) {
			return errors.New("restored maintenance DDL or complete values differ")
		}
	}
	if !reflect.DeepEqual(b.Retained, maintenanceRetained(t, n)) {
		return errors.New("object replay changed retained facts")
	}
	if fault == maintenanceRestoreBeforeLedger {
		return errors.New("fixed interruption before original ledger replay")
	}
	// Replay the complete genuine original ledger last, within one explicit
	// transaction. No selected version, Force, SetVersion, retry or dirty edit.
	tx, err := n.db.BeginTx(t.Context(), nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	if _, err = tx.ExecContext(t.Context(), "DELETE FROM schema_migrations"); err != nil {
		return err
	}
	for _, row := range b.Ledger[0].Rows {
		if _, err = tx.ExecContext(t.Context(), "INSERT INTO schema_migrations(version,dirty) VALUES(?,?)", row[0], row[1]); err != nil {
			return err
		}
		if fault == maintenanceRestoreDuplicateLedgerRow {
			_, err = tx.ExecContext(t.Context(), "INSERT INTO schema_migrations(version,dirty) VALUES(?,?)", row[0], row[1])
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	if !reflect.DeepEqual(b.Ledger[0], maintenanceMaterial(t, n, "schema_migrations")) {
		return errors.New("captured original ledger replay differs")
	}
	return nil
}

func TestLegacyMaintenanceNativeDirtyObjectBackupRestore(t *testing.T) {
	for _, test := range []struct {
		name               string
		fault              maintenanceRestoreFault
		tamper, factChange bool
		runtimeStarted     bool
	}{
		{"restore", maintenanceRestoreNoFault, false, false, false},
		{"wrong_backup", maintenanceRestoreNoFault, true, false, false},
		{"retained_fact_changed", maintenanceRestoreNoFault, false, true, false},
		{"object_restore_failed", maintenanceRestoreDuplicateObjectRow, false, false, false},
		{"before_ledger_interrupted", maintenanceRestoreBeforeLedger, false, false, false},
		{"ledger_replay_failed", maintenanceRestoreDuplicateLedgerRow, false, false, false},
		{"runtime_started", maintenanceRestoreNoFault, false, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			n := nativeLegacyMaintenancePair(t)
			failed := failedMaintenanceResources(t)
			original := captureMaintenanceObjects(t, n, failed)
			approved := sha256.Sum256(original)
			failMaintenanceMigration(t, n, failed)
			input := append([]byte(nil), original...)
			if test.tamper {
				input = append(input, ' ')
			}
			if test.factChange {
				if _, err := n.db.ExecContext(t.Context(), "UPDATE testee SET name='new synthetic fact' WHERE id=7"); err != nil {
					t.Fatal(err)
				}
			}
			err := replayMaintenanceObjects(t, n, input, approved, failed, !test.runtimeStarted, test.fault)
			if test.tamper || test.factChange || test.runtimeStarted || test.fault != maintenanceRestoreNoFault {
				if err == nil {
					t.Fatal("unauthorized or incomplete object replay accepted")
				}
				if test.fault == maintenanceRestoreDuplicateObjectRow || test.fault == maintenanceRestoreDuplicateLedgerRow {
					var failure *driver.MySQLError
					if !errors.As(err, &failure) || failure.Number != 1062 {
						t.Fatal("expected real duplicate primary-key rejection", err)
					}
				}
				if sqlVersion, dirty, _, _ := n.heads(t); sqlVersion != 103 || !dirty {
					t.Fatal("failed replay restored metadata early")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertMaintenanceTables(t, n, 5)
			old := migrationResourcesThrough(t, 102, 40)
			p, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, old)
			if err != nil || p.VerifyComplete(t.Context()) != nil {
				t.Fatal("fresh102/40 rejected genuine restored objects and original ledger", err)
			}
		})
	}
}

func TestLegacyMaintenanceNativeWrongObjectScopesRefusedBeforeClaim(t *testing.T) {
	n := nativeLegacyMaintenancePair(t)
	r := migrationResourcesThrough(t, 103, 40)
	before := maintenanceRetained(t, n)
	for _, scope := range []string{offlineProtectionSQLHistoricalColumns, offlineProtectionMongoHistoricalIndexes, offlineProtectionActorColumns} {
		t.Run(scope, func(t *testing.T) {
			cfg := offlineNativeApproval(t, n, r)
			_, snapshot, err := inspectOffline(t.Context(), n.db, n.client, cfg.Pair, r)
			if err != nil {
				t.Fatal(err)
			}
			a, err := readOfflineAuthorization(cfg, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			a.ProtectionScope = scope
			writeOfflineApproval(t, &cfg, a)
			result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
			if err == nil || result.Stage != "approval" || result.AuthorizationConsumed || result.Verified {
				t.Fatal("other object scope authorized maintenance retirement", result, err)
			}
			if _, err := os.Lstat(cfg.AuthorizationFile + ".consumed"); !os.IsNotExist(err) {
				t.Fatal("wrong scope consumed approval", err)
			}
			assertMaintenanceTables(t, n, 5)
			if !reflect.DeepEqual(before, maintenanceRetained(t, n)) {
				t.Fatal("rejected scope changed retained facts")
			}
		})
	}
}

func TestLegacyMaintenanceNativeReappearedTableFailsClosed(t *testing.T) {
	n := nativeLegacyMaintenancePair(t)
	var originals []maintenanceTableMaterial
	for _, name := range retiredMaintenanceSQLTables {
		originals = append(originals, maintenanceMaterial(t, n, name))
	}
	r := migrationResourcesThrough(t, 103, 40)
	n.runBoth(t, n.preflight(t, r))
	for _, original := range originals {
		if _, err := n.db.ExecContext(t.Context(), original.DDL[0][1].String); err != nil {
			t.Fatal(err)
		}
		if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
			t.Fatal("retired maintenance table reappeared on clean SQL103")
		}
		if _, err := n.db.ExecContext(t.Context(), "DROP TABLE `"+original.Name+"`"); err != nil {
			t.Fatal(err)
		}
	}
	if err := n.preflight(t, r).VerifyComplete(context.Background()); err != nil {
		t.Fatal(err)
	}
}
