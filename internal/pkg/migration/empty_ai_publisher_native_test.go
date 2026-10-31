//go:build integration

package migration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmysql "github.com/go-sql-driver/mysql"
)

func TestM5EmptyAIPublisherOwnedNativeMySQL(t *testing.T) {
	name, port := os.Getenv("MYSQL_DATABASE"), os.Getenv("MYSQL_PORT")
	if os.Getenv("QS_METADATA_PROFILE_NATIVE_REQUIRED") != "1" || os.Getenv("MYSQL_HOST") != "127.0.0.1" || os.Getenv("MYSQL_USERNAME") != "root" || port == "3306" || !regexp.MustCompile(`^[0-9]{2,5}$`).MatchString(port) || !regexp.MustCompile(`^qs_profile_owned_[a-z0-9]{8,24}$`).MatchString(name) || os.Getenv("MYSQL_CONTAINER") == "" || os.Getenv("QS_M5_NATIVE_REPORT") == "" {
		t.Fatal("explicit owned native fixture/report required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cfg := sqlmysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd = "tcp", "127.0.0.1:"+port, "root", os.Getenv("MYSQL_PASSWORD")
	cfg.MultiStatements = true
	connector, err := sqlmysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	admin := sql.OpenDB(connector)
	defer func() { _ = admin.Close() }()
	var uuid, version string
	if err = admin.QueryRowContext(ctx, "SELECT @@server_uuid,VERSION()").Scan(&uuid, &version); err != nil || uuid == "" || uuid != os.Getenv("QS_METADATA_PROFILE_NATIVE_UUID") || version != "8.0.36" {
		t.Fatal("owned native identity mismatch")
	}
	owned := "qs_m5_empty_" + strings.TrimPrefix(name, "qs_profile_owned_")
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+owned+"`"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE `"+owned+"`"); err != nil {
			t.Error("owned database cleanup failed")
		}
	}()
	cfg.DBName = owned
	connector, err = sqlmysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	report := struct {
		FixtureOnly           bool     `json:"fixture_only"`
		ExecutionSource       string   `json:"execution_source_sha"`
		MySQLVersion          string   `json:"mysql_version"`
		OriginalDDL           string   `json:"original_fixture_ddl"`
		OriginalHash          string   `json:"original_show_create_sha256"`
		RestoredHash          string   `json:"restored_show_create_sha256"`
		UnboundRefused        bool     `json:"unbound_sql_refused"`
		NonemptyRejected      []string `json:"nonempty_rejected"`
		ProtectedPreserved    bool     `json:"protected_fixture_facts_preserved"`
		LockContentionRefused bool     `json:"lock_contention_refused"`
		TriggerPreserved      bool     `json:"unreviewed_trigger_preserved"`
		ViewPreserved         bool     `json:"dependent_view_preserved"`
		Passed                bool     `json:"passed"`
	}{FixtureOnly: true, ExecutionSource: os.Getenv("GITHUB_SHA"), MySQLVersion: version, NonemptyRejected: []string{}}
	t.Cleanup(func() {
		report.Passed = !t.Failed()
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			err = os.WriteFile(os.Getenv("QS_M5_NATIVE_REPORT"), append(data, '\n'), 0600)
		}
		if err != nil {
			t.Error("native report unavailable")
		}
	})
	kept := []string{"rm_tx_witness", "ai_messaging_operations", "ai_messaging_inbox", "ai_messaging_archive", "business_retry_schedule"}
	for _, table := range kept {
		if _, err = db.ExecContext(ctx, "CREATE TABLE `"+table+"` (id INT PRIMARY KEY,payload VARBINARY(128) NOT NULL)"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, "INSERT INTO `"+table+"` VALUES (1,?)", []byte("original-"+table)); err != nil {
			t.Fatal(err)
		}
	}
	up91, err := migrations.ReadFile("migrations/mysql/000091_ai_messaging.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	ddl := strings.SplitN(string(up91), ";", 2)[0] + ";"
	raw, err := migrations.ReadFile("migrations/mysql/000111_retire_empty_ai_publisher.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	create := func(t *testing.T) {
		t.Helper()
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	drop := func(t *testing.T) {
		t.Helper()
		if _, err := db.ExecContext(ctx, "DROP TABLE ai_messaging_outbox"); err != nil {
			t.Fatal(err)
		}
	}
	show := func(t *testing.T) string {
		t.Helper()
		var table, definition string
		if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE ai_messaging_outbox").Scan(&table, &definition); err != nil {
			t.Fatal(err)
		}
		return definition
	}
	run := func(t *testing.T) error {
		t.Helper()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		var beforeWait uint64
		if err := conn.QueryRowContext(ctx, "SELECT @@SESSION.lock_wait_timeout").Scan(&beforeWait); err != nil {
			t.Fatal(err)
		}
		d := &retirementDatabaseDriver{backend: BackendMySQL, version: 111, ctx: ctx, sqlConn: conn, database: owned}
		err = d.runEmptyAIPublisherStep(raw)
		var guard, statement any
		var afterWait uint64
		if queryErr := conn.QueryRowContext(ctx, "SELECT @qs_m5_empty_ai_publisher_guard,@qs_m5_ai_publisher_sql,@@SESSION.lock_wait_timeout").Scan(&guard, &statement, &afterWait); queryErr != nil || guard != nil || statement != nil || beforeWait != afterWait {
			t.Fatal("session was not restored", queryErr)
		}
		return err
	}
	t.Run("empty_drop_and_exact_structure_restore", func(t *testing.T) {
		create(t)
		report.OriginalDDL = show(t)
		report.OriginalHash = retirementHash([]byte(report.OriginalDDL))
		if _, err := db.ExecContext(ctx, string(raw)); err == nil {
			t.Fatal("unbound SQL was accepted")
		}
		report.UnboundRefused = true
		if err := run(t); err != nil {
			t.Fatal(err)
		}
		var present int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='ai_messaging_outbox'").Scan(&present); err != nil || present != 0 {
			t.Fatal("target was not retired", err)
		}
		if _, err := db.ExecContext(ctx, report.OriginalDDL); err != nil {
			t.Fatal(err)
		}
		report.RestoredHash = retirementHash([]byte(show(t)))
		if report.OriginalHash != report.RestoredHash {
			t.Fatal("restored original DDL differs")
		}
		drop(t)
	})
	for _, state := range []string{"pending", "confirmed", "future", "NULL"} {
		t.Run("preserve_"+state, func(t *testing.T) {
			create(t)
			defer drop(t)
			var stage any = state
			if state == "NULL" {
				stage = nil
				if _, err := db.ExecContext(ctx, "ALTER TABLE ai_messaging_outbox MODIFY stage VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL"); err != nil {
					t.Fatal(err)
				}
			}
			_, err := db.ExecContext(ctx, `INSERT INTO ai_messaging_outbox(producer,destination,message_id,body_sha256,body,wire,wire_sha256,kind,organization_id,topic,aggregate_key,aggregate_sequence,ordered,requires_receipt,stage,attempts,available_at,created_at) VALUES('fixture','ai','original',?,?,?,?,1,1,'topic','aggregate',1,TRUE,TRUE,?,0,NOW(6),NOW(6))`, strings.Repeat("a", 64), []byte("original-body"), []byte("original-wire"), strings.Repeat("b", 64), stage)
			if err != nil {
				t.Fatal(err)
			}
			if err := run(t); err == nil {
				t.Fatal("nonempty table accepted")
			}
			var body, wire []byte
			var actual sql.NullString
			var count int
			if err := db.QueryRowContext(ctx, "SELECT body,wire,stage FROM ai_messaging_outbox WHERE message_id='original'").Scan(&body, &wire, &actual); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_outbox").Scan(&count); err != nil || count != 1 || !bytes.Equal(body, []byte("original-body")) || !bytes.Equal(wire, []byte("original-wire")) || actual.Valid != (state != "NULL") || actual.Valid && actual.String != state {
				t.Fatal("original record changed", err)
			}
			report.NonemptyRejected = append(report.NonemptyRejected, state)
		})
	}
	t.Run("dependent_view_preserved", func(t *testing.T) {
		create(t)
		defer drop(t)
		if _, err := db.ExecContext(ctx, "CREATE VIEW m5_owned_view AS SELECT producer FROM ai_messaging_outbox"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := db.ExecContext(ctx, "DROP VIEW m5_owned_view"); err != nil {
				t.Error(err)
			}
		}()
		if err := run(t); err == nil {
			t.Fatal("dependent view was accepted")
		}
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM m5_owned_view").Scan(&count); err != nil || count != 0 {
			t.Fatal("dependent view broken", err)
		}
		report.ViewPreserved = true
	})
	t.Run("unreviewed_trigger_preserved", func(t *testing.T) {
		create(t)
		defer drop(t)
		if _, err := db.ExecContext(ctx, "CREATE TRIGGER m5_owned_probe BEFORE INSERT ON ai_messaging_outbox FOR EACH ROW SET NEW.error_code='owned-original'"); err != nil {
			t.Fatal(err)
		}
		if err := run(t); err == nil {
			t.Fatal("unreviewed trigger was accepted")
		}
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema=DATABASE() AND trigger_name='m5_owned_probe'").Scan(&count); err != nil || count != 1 {
			t.Fatal("original trigger lost", err)
		}
		report.TriggerPreserved = true
	})
	t.Run("bounded_external_read_lock", func(t *testing.T) {
		create(t)
		defer drop(t)
		other, err := admin.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = other.Close() }()
		if _, err := other.ExecContext(ctx, "LOCK TABLES `"+owned+"`.`ai_messaging_outbox` READ"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := other.ExecContext(context.Background(), "UNLOCK TABLES"); err != nil {
				t.Error(err)
			}
		}()
		start := time.Now()
		if err := run(t); err == nil || time.Since(start) > 6*time.Second {
			t.Fatal("bounded contention refusal failed", err)
		}
		report.LockContentionRefused = true
	})
	for _, table := range kept {
		var payload []byte
		if err := db.QueryRowContext(ctx, "SELECT payload FROM `"+table+"` WHERE id=1").Scan(&payload); err != nil || !bytes.Equal(payload, []byte("original-"+table)) {
			t.Fatal("protected fact changed", table, err)
		}
	}
	report.ProtectedPreserved = true
}
