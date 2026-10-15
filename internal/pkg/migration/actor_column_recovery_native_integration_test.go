//go:build integration

package migration

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"go.mongodb.org/mongo-driver/bson"
)

// These fixed test-only materials are not a production restore API. No business
// runtime starts in this fixture; a real maintenance window must prove that
// boundary independently before replaying any captured object or ledger.
type actorColumnObjectBackup struct {
	SQLIdentity, MongoIdentity, InitialResources, FailedResources string
	MigrationDDL, MigrationRows                                   [][]sql.NullString
	TableDDLs, Values                                             [][][]sql.NullString
	Retained                                                      []historicalTableFacts
	Archives                                                      [][][]sql.NullString
	MongoHead                                                     []byte
}

func actorColumnRetainedFacts(t *testing.T, n nativeSchemaPair) []historicalTableFacts {
	t.Helper()
	var facts []historicalTableFacts
	for _, item := range []struct{ table, column string }{{"testee", "tags"}, {"operators", "effective_roles"}} {
		var fact historicalTableFacts
		var selectColumns []string
		for _, row := range historicalQuery(t, n.db, "SELECT COLUMN_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='"+item.table+"' ORDER BY ORDINAL_POSITION") {
			if row[0].String == item.column {
				continue
			}
			fact.Columns = append(fact.Columns, row[0].String)
			selectColumns = append(selectColumns, "`"+strings.ReplaceAll(row[0].String, "`", "``")+"`")
		}
		fact.Rows = historicalQuery(t, n.db, "SELECT "+strings.Join(selectColumns, ",")+" FROM `"+item.table+"` ORDER BY id")
		facts = append(facts, fact)
	}
	return facts
}
func actorColumnArchiveFacts(t *testing.T, n nativeSchemaPair) [][][]sql.NullString {
	t.Helper()
	return [][][]sql.NullString{historicalQuery(t, n.db, "SELECT * FROM operator_retirement_tasks ORDER BY operator_id"), historicalQuery(t, n.db, "SELECT * FROM operator_recovery_archives ORDER BY request_id")}
}
func captureActorColumnObjects(t *testing.T, n nativeSchemaPair, failed *migrationResources) []byte {
	t.Helper()
	initial := migrationResourcesThrough(t, 101, 40)
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
	b := actorColumnObjectBackup{
		SQLIdentity: sqlIdentity, MongoIdentity: mongoIdentity, InitialResources: initial.sha256, FailedResources: failed.sha256,
		MigrationDDL: historicalQuery(t, n.db, "SHOW CREATE TABLE schema_migrations"), MigrationRows: historicalQuery(t, n.db, "SELECT * FROM schema_migrations ORDER BY version"),
		TableDDLs: [][][]sql.NullString{historicalQuery(t, n.db, "SHOW CREATE TABLE testee"), historicalQuery(t, n.db, "SHOW CREATE TABLE operators")},
		Values:    [][][]sql.NullString{historicalQuery(t, n.db, "SELECT id,CAST(tags AS CHAR) FROM testee ORDER BY id"), historicalQuery(t, n.db, "SELECT id,CAST(effective_roles AS CHAR) FROM operators ORDER BY id")},
		Retained:  actorColumnRetainedFacts(t, n), Archives: actorColumnArchiveFacts(t, n), MongoHead: head,
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func replayActorColumnObjects(t *testing.T, n nativeSchemaPair, raw []byte, approved [32]byte, failed *migrationResources, fault historicalRestoreFault) (err error) {
	t.Helper()
	if sha256.Sum256(raw) != approved {
		return errors.New("actor object backup fingerprint rejected")
	}
	var b actorColumnObjectBackup
	if json.Unmarshal(raw, &b) != nil || b.InitialResources != migrationResourcesThrough(t, 101, 40).sha256 || b.FailedResources != failed.sha256 {
		return errors.New("actor object backup resources rejected")
	}
	sqlIdentity, mongoIdentity, err := pairIdentity(t.Context(), n.db, n.client, n.cfg.MySQLDatabase, n.cfg.MongoDatabase)
	if err != nil || sqlIdentity != b.SQLIdentity || mongoIdentity != b.MongoIdentity {
		return errors.New("actor object backup identity rejected")
	}
	head, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Raw()
	sqlVersion, dirty, mongoVersion, mongoDirty := n.heads(t)
	if err != nil || sqlVersion != 102 || !dirty || mongoVersion != 40 || mongoDirty || !bytes.Equal(head, b.MongoHead) || len(historicalQuery(t, n.db, "SELECT * FROM schema_migrations")) != 1 {
		return errors.New("actor failed pair rejected")
	}
	if !reflect.DeepEqual(b.Retained, actorColumnRetainedFacts(t, n)) || !reflect.DeepEqual(b.Archives, actorColumnArchiveFacts(t, n)) || !reflect.DeepEqual(b.MigrationDDL, historicalQuery(t, n.db, "SHOW CREATE TABLE schema_migrations")) {
		return errors.New("actor retained facts or ledger DDL changed")
	}
	// Admit only the genuine second-ALTER failure. The original operators column
	// is still present; never drop/recreate it or an entire business table.
	assertActorColumns(t, n, false, true)
	var definition string
	for _, line := range strings.Split(b.TableDDLs[0][0][1].String, "\n") {
		if strings.HasPrefix(line, "  `tags` ") {
			definition = strings.TrimSuffix(strings.TrimSpace(line), ",")
		}
	}
	if definition == "" {
		return errors.New("captured original tags definition unavailable")
	}
	if _, err = n.db.ExecContext(t.Context(), "ALTER TABLE testee ADD COLUMN "+definition+" AFTER birthday"); err != nil {
		return err
	}
	if fault == historicalRestoreRejectValue {
		// A genuine JSON failure occurs before any ledger replay.
		_, err = n.db.ExecContext(t.Context(), "UPDATE testee SET tags='{',updated_at=updated_at WHERE id=7")
		if err == nil {
			return errors.New("invalid JSON unexpectedly accepted")
		}
		return err
	}
	for i, item := range []struct{ table, column string }{{"testee", "tags"}, {"operators", "effective_roles"}} {
		for _, row := range b.Values[i] {
			if _, err = n.db.ExecContext(t.Context(), "UPDATE `"+item.table+"` SET `"+item.column+"`=?,updated_at=updated_at WHERE id=?", row[1], row[0].String); err != nil {
				return err
			}
		}
		if !reflect.DeepEqual(b.Values[i], historicalQuery(t, n.db, "SELECT id,CAST(`"+item.column+"` AS CHAR) FROM `"+item.table+"` ORDER BY id")) || !reflect.DeepEqual(b.TableDDLs[i], historicalQuery(t, n.db, "SHOW CREATE TABLE `"+item.table+"`")) {
			return errors.New("actor original DDL or complete values differ")
		}
	}
	if !reflect.DeepEqual(b.Retained, actorColumnRetainedFacts(t, n)) || !reflect.DeepEqual(b.Archives, actorColumnArchiveFacts(t, n)) {
		return errors.New("actor object replay changed retained facts")
	}
	// The complete real original ledger is the last replayed object. There is no
	// chosen version/dirty argument, Force, SetVersion or UPDATE-dirty operation.
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
	for _, row := range b.MigrationRows {
		if _, err = tx.ExecContext(t.Context(), "INSERT INTO schema_migrations(version,dirty) VALUES(?,?)", row[0].String, row[1].String); err != nil {
			return err
		}
	}
	if fault == historicalRestoreDuplicateLedgerRow {
		row := b.MigrationRows[0]
		_, err = tx.ExecContext(t.Context(), "INSERT INTO schema_migrations(version,dirty) VALUES(?,?)", row[0].String, row[1].String)
		if err == nil {
			return errors.New("duplicate original ledger unexpectedly accepted")
		}
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	if !reflect.DeepEqual(b.MigrationRows, historicalQuery(t, n.db, "SELECT * FROM schema_migrations ORDER BY version")) {
		return errors.New("restored captured actor ledger differs")
	}
	return nil
}

func TestActorColumnsNativeDirtyObjectBackupRestore(t *testing.T) {
	for _, test := range []struct {
		name               string
		fault              historicalRestoreFault
		tamper, factChange bool
	}{
		{"restore", historicalRestoreNoFault, false, false},
		{"wrong_backup", historicalRestoreNoFault, true, false},
		{"retained_fact_changed", historicalRestoreNoFault, false, true},
		{"value_restore_failed", historicalRestoreRejectValue, false, false},
		{"ledger_replay_failed", historicalRestoreDuplicateLedgerRow, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			n := nativeActorColumnPair(t)
			release := migrationResourcesThrough(t, 102, 40)
			source := release.source.(fstest.MapFS)
			source[actorColumnsUp].Data = bytes.Replace(source[actorColumnsUp].Data, []byte("DROP COLUMN `effective_roles`"), []byte("DROP COLUMN `actor_missing_column`"), 1)
			failed, err := readMigrationResources(source, release.pairs)
			if err != nil {
				t.Fatal(err)
			}
			// Freeze the approval SHA once, before the actual first DDL. It is never
			// recomputed from the input offered to the replay operation.
			original := captureActorColumnObjects(t, n, failed)
			approved := sha256.Sum256(original)
			p := n.preflight(t, failed)
			if _, _, err = NewMigrator(n.db, p.MySQLConfig(false)).Run(); err == nil {
				t.Fatal("genuine second ALTER did not fail")
			}
			assertActorColumns(t, n, false, true)
			if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 102 || !dirty || mongo != 40 || mongoDirty {
				t.Fatal("failed DDL did not retain dirty102")
			}
			old := migrationResourcesThrough(t, 101, 40)
			if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, old); err == nil {
				t.Fatal("old source accepted dirty future DDL")
			}
			input := append([]byte(nil), original...)
			if test.tamper {
				input = append(input, byte(' '))
			}
			if test.factChange {
				if _, err = n.db.ExecContext(t.Context(), "UPDATE testee SET name='new fact' WHERE id=7"); err != nil {
					t.Fatal(err)
				}
			}
			err = replayActorColumnObjects(t, n, input, approved, failed, test.fault)
			if test.tamper || test.factChange || test.fault != historicalRestoreNoFault {
				if err == nil {
					t.Fatal("failed/unauthorized replay accepted")
				}
				if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 102 || !dirty || mongo != 40 || mongoDirty {
					t.Fatal("failed replay restored the ledger early")
				}
				if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, old); err == nil {
					t.Fatal("failed replay became restartable")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertActorColumns(t, n, true, true)
			restored, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, old)
			if err != nil || restored.VerifyComplete(t.Context()) != nil {
				t.Fatal("fresh101/40 rejected the actual restored objects/ledger", err)
			}
		})
	}
}
