//go:build integration

package migration

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// Rehearse the reviewed original-DDL/metadata steps only in the owned test pair.
// This is not an installed recovery tool, force/down API or production approval.
func rehearseM5OriginalEmptyRestore(ctx context.Context, n nativeSchemaPair, ddl, ddlHash, sqlIdentity, mongoIdentity string, runtimeStarted bool) error {
	if runtimeStarted || !strings.HasPrefix(n.cfg.MySQLDatabase, "qs_m5_restore_test_") || retirementHash([]byte(ddl)) != ddlHash {
		return errors.New("owned original material and no runtime start required")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	a, b, err := pairIdentity(ctx, n.db, n.client, n.cfg.MySQLDatabase, n.cfg.MongoDatabase)
	if err != nil || a != sqlIdentity || b != mongoIdentity {
		return errors.New("original owner identity changed")
	}
	var sqlState struct {
		version uint
		dirty   bool
	}
	var rows int
	if err := n.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&rows); err != nil || rows != 1 {
		return errors.New("exact original metadata cardinality required")
	}
	if err := n.db.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&sqlState.version, &sqlState.dirty); err != nil || sqlState.version != 111 {
		return errors.New("exact interrupted version required")
	}
	var mongoHead struct {
		Version uint `bson:"version"`
		Dirty   bool `bson:"dirty"`
	}
	if count, err := n.mongoDB.Collection("schema_migrations").CountDocuments(ctx, map[string]any{}); err != nil || count != 1 {
		return errors.New("exact original Mongo metadata cardinality required")
	}
	if err := n.mongoDB.Collection("schema_migrations").FindOne(ctx, map[string]any{}).Decode(&mongoHead); err != nil || mongoHead.Version != 44 || mongoHead.Dirty {
		return errors.New("original Mongo head required")
	}
	var present int
	if err := n.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name='ai_messaging_outbox'", n.cfg.MySQLDatabase).Scan(&present); err != nil {
		return err
	}
	if present == 0 {
		if _, err := n.db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	var name, actual string
	var count int
	if err := n.db.QueryRowContext(ctx, "SHOW CREATE TABLE ai_messaging_outbox").Scan(&name, &actual); err != nil {
		return err
	}
	if retirementHash([]byte(actual)) != ddlHash {
		return errors.New("original structure not restored")
	}
	if err := n.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_outbox").Scan(&count); err != nil || count != 0 {
		return errors.New("original empty stream not restored")
	}
	// The concrete operator plan must separately approve this exact guarded state
	// repair after re-reading server activity; never call a generic Force method.
	result, err := n.db.ExecContext(ctx, "UPDATE schema_migrations SET version=110,dirty=FALSE WHERE version=111 AND dirty=?", sqlState.dirty)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return errors.New("exact metadata repair not confirmed")
	}
	return nil
}

func TestSchemaPairNativeM5OriginalEmptyRestoreAndOldCompatibility(t *testing.T) {
	source := make(fstest.MapFS)
	if err := fs.WalkDir(migrations, "migrations", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if strings.HasPrefix(path, "migrations/mysql/000111_") {
			return nil
		}
		raw, err := migrations.ReadFile(path)
		if err == nil {
			source[path] = &fstest.MapFile{Data: raw}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pairs := make([]schemaVersionPair, 0, len(declaredSchemaPairs))
	for _, pair := range declaredSchemaPairs {
		if pair.SQL <= 110 {
			pairs = append(pairs, pair)
		}
	}
	old, err := readMigrationResources(source, pairs)
	if err != nil {
		t.Fatal(err)
	}
	current, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	if old.sql.head != 110 || old.mongo.head != 44 || current.sql.head != 111 {
		t.Fatal("reviewed resource boundary changed")
	}
	cases := []string{"dirty_before_ddl", "dirty_after_drop", "clean_before_runtime"}
	report := map[string]any{"fixture_only": true, "old_resources_sha256": old.sha256, "current_resources_sha256": current.sha256, "cases": cases, "production_restore_or_approval": false, "generic_force_or_down_used": false}
	for _, state := range cases {
		t.Run(state, func(t *testing.T) {
			n := openNativeSchemaPairWithPrefix(t, "qs_m5_restore_test")
			n.approve(t, old)
			n.runBoth(t, n.preflight(t, old))
			var name, ddl string
			if err := n.db.QueryRowContext(t.Context(), "SHOW CREATE TABLE ai_messaging_outbox").Scan(&name, &ddl); err != nil {
				t.Fatal(err)
			}
			hash := retirementHash([]byte(ddl))
			a, b, err := pairIdentity(t.Context(), n.db, n.client, n.cfg.MySQLDatabase, n.cfg.MongoDatabase)
			if err != nil {
				t.Fatal(err)
			}
			protected := map[string]int64{}
			for _, table := range []string{"rm_tx_binding", "rm_tx_execution", "rm_tx_witness", "ai_messaging_archive", "ai_messaging_inbox", "ai_messaging_admission", "task_opened_reminder_batch", "task_opened_reminder_delivery", "rm_outbox"} {
				var count int64
				if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); err != nil {
					t.Fatal(err)
				}
				protected[table] = count
			}
			if state == "dirty_before_ddl" {
				if _, err := n.db.ExecContext(t.Context(), "UPDATE schema_migrations SET version=111,dirty=TRUE WHERE version=110 AND dirty=FALSE"); err != nil {
					t.Fatal(err)
				}
			} else {
				n.runBoth(t, n.preflight(t, current))
				if state == "dirty_after_drop" {
					if _, err := n.db.ExecContext(t.Context(), "UPDATE schema_migrations SET dirty=TRUE WHERE version=111 AND dirty=FALSE"); err != nil {
						t.Fatal(err)
					}
				}
			}
			// Refusals are read-only and must leave the original interrupted head intact.
			for _, negative := range []struct {
				ddl, hash, a, b string
				started         bool
			}{{ddl, hash, a, b, true}, {ddl + " ", hash, a, b, false}, {ddl, hash, strings.Repeat("0", 64), b, false}} {
				if err := rehearseM5OriginalEmptyRestore(t.Context(), n, negative.ddl, negative.hash, negative.a, negative.b, negative.started); err == nil {
					t.Fatal("invalid recovery evidence accepted")
				}
				var version uint
				if err := n.db.QueryRowContext(t.Context(), "SELECT version FROM schema_migrations").Scan(&version); err != nil || version != 111 {
					t.Fatal("refusal changed metadata", err)
				}
			}
			if state == "dirty_before_ddl" {
				_, err := n.db.ExecContext(t.Context(), `INSERT INTO ai_messaging_outbox(producer,destination,message_id,body_sha256,body,wire,wire_sha256,kind,organization_id,topic,aggregate_key,aggregate_sequence,ordered,requires_receipt,stage,attempts,available_at,created_at) VALUES('owned-recovery-fixture','ai','original',?,?,?,?,1,1,'topic','aggregate',1,TRUE,TRUE,'unknown',0,NOW(6),NOW(6))`, strings.Repeat("a", 64), []byte("owned-original-body"), []byte("owned-original-wire"), strings.Repeat("b", 64))
				if err != nil {
					t.Fatal(err)
				}
				if err := rehearseM5OriginalEmptyRestore(t.Context(), n, ddl, hash, a, b, false); err == nil {
					t.Fatal("nonempty restoration overwrote original facts")
				}
				var body, wire []byte
				if err := n.db.QueryRowContext(t.Context(), "SELECT body,wire FROM ai_messaging_outbox WHERE producer='owned-recovery-fixture'").Scan(&body, &wire); err != nil || string(body) != "owned-original-body" || string(wire) != "owned-original-wire" {
					t.Fatal("nonempty refusal changed fixture", err)
				}
				// Remove only this test's own negative fixture; no production responsibility is cleared.
				if _, err := n.db.ExecContext(t.Context(), "DELETE FROM ai_messaging_outbox WHERE producer='owned-recovery-fixture'"); err != nil {
					t.Fatal(err)
				}
			}
			if err := rehearseM5OriginalEmptyRestore(t.Context(), n, ddl, hash, a, b, false); err != nil {
				t.Fatal(err)
			}
			if err := n.preflight(t, old).VerifyComplete(t.Context()); err != nil {
				t.Fatal("old110/44 compatibility was not restored", err)
			}
			sqlVersion, sqlDirty, mongoVersion, mongoDirty := n.heads(t)
			if sqlVersion != 110 || sqlDirty || mongoVersion != 44 || mongoDirty {
				t.Fatal("original clean pair not restored")
			}
			for table, before := range protected {
				var after int64
				if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM `"+table+"`").Scan(&after); err != nil || after != before {
					t.Fatal("retained facts changed", table, err)
				}
			}
			report["original_and_restored_DDL_sha256"] = hash
		})
	}
	if t.Failed() {
		return
	}
	report["passed"] = true
	if path := os.Getenv("QS_M5_PAIR_RESTORE_PROOF"); path != "" {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
