//go:build integration

package main

import (
	"context"
	"database/sql"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func TestRetirementInventoryOwnedNativeMySQL(t *testing.T) {
	name := os.Getenv("MYSQL_DATABASE")
	port := os.Getenv("MYSQL_PORT")
	if os.Getenv("QS_METADATA_PROFILE_NATIVE_REQUIRED") != "1" || os.Getenv("MYSQL_HOST") != "127.0.0.1" || os.Getenv("MYSQL_USERNAME") != "root" || port == "3306" || !regexp.MustCompile(`^[0-9]{2,5}$`).MatchString(port) || !regexp.MustCompile(`^qs_profile_owned_[a-z0-9]{8,24}$`).MatchString(name) || os.Getenv("MYSQL_CONTAINER") == "" {
		t.Fatal("explicit owned loopback fixture required")
	}
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd = "tcp", "127.0.0.1:"+port, "root", os.Getenv("MYSQL_PASSWORD")
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	admin := sql.OpenDB(connector)
	defer func() { _ = admin.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var uuid, version string
	if err = admin.QueryRowContext(ctx, "SELECT @@server_uuid, VERSION()").Scan(&uuid, &version); err != nil || uuid == "" || uuid != os.Getenv("QS_METADATA_PROFILE_NATIVE_UUID") || version != "8.0.36" {
		t.Fatal("owned MySQL identity mismatch")
	}
	owned := "qs_rm_inventory_" + strings.TrimPrefix(name, "qs_profile_owned_")
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+owned+"`"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.ExecContext(context.Background(), "DROP DATABASE `"+owned+"`"); e != nil {
			t.Error(e)
		}
	}()
	cfg.DBName = owned
	connector, err = mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()
	if _, err = db.ExecContext(ctx, "CREATE TABLE schema_migrations (version BIGINT PRIMARY KEY,dirty BOOLEAN NOT NULL) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (110,FALSE)"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []object{{"rm_outbox", "state", "published"}, {"domain_event_outbox", "status", "published"}, {"ai_messaging_outbox", "stage", "confirmed"}} {
		extra := ""
		if table.Name == "ai_messaging_outbox" {
			extra = ",body MEDIUMBLOB NULL,wire MEDIUMBLOB NULL"
		}
		_, err = db.ExecContext(ctx, "CREATE TABLE `"+table.Name+"` (`"+table.State+"` VARCHAR(40) NULL"+extra+") ENGINE=InnoDB")
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.ExecContext(ctx, "INSERT INTO `"+table.Name+"` (`"+table.State+"`) VALUES (?),(NULL),('future'),(?),(?)", table.Closed, strings.ToUpper(table.Closed), table.Closed+" ")
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, owner := range []string{"qs.mysql", "iam.mysql"} {
		r, e := collect(ctx, db, owner, cfg.Addr)
		if e != nil || !r.Complete || !r.TransactionReleased || db.Stats().InUse != 0 || r.DropAuthorized || r.BusinessVerified || r.ExportVerified || r.WritersExcluded || r.SchemaHead == nil || *r.SchemaHead != 110 || r.SchemaDirty == nil || *r.SchemaDirty {
			t.Fatal(r, e)
		}
		for _, table := range r.Tables {
			if *table.Rows != 5 || *table.Closed != 1 || *table.Unclosed != 4 || *table.NullStates != 1 || len(table.SchemaHash) != 64 {
				t.Fatal("state byte partition failed", table)
			}
		}
	}
	for _, values := range []string{"", "(110,FALSE),(111,FALSE)", "(110,TRUE)"} {
		if _, err = db.ExecContext(ctx, "DELETE FROM schema_migrations"); err != nil {
			t.Fatal(err)
		}
		if values != "" {
			if _, err = db.ExecContext(ctx, "INSERT INTO schema_migrations VALUES "+values); err != nil {
				t.Fatal(err)
			}
		}
		r, e := collect(ctx, db, "qs.mysql", cfg.Addr)
		if e == nil || r.Complete || len(r.Tables) != 0 || !r.TransactionReleased || db.Stats().InUse != 0 {
			t.Fatal("ambiguous/dirty head was accepted", r, e)
		}
	}
	if _, err = db.ExecContext(ctx, "DELETE FROM schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO schema_migrations VALUES (110,FALSE)"); err != nil {
		t.Fatal(err)
	}
	ro, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	_, denied := ro.ExecContext(ctx, "INSERT INTO rm_outbox VALUES ('published')")
	_ = ro.Rollback()
	if denied == nil {
		t.Fatal("native MySQL did not enforce read-only transaction")
	}
	// Separate empty-stream fixture; the five original-state cases above have
	// already been verified and are not bootstrap/business responsibilities.
	if _, err = db.ExecContext(ctx, "DELETE FROM ai_messaging_outbox"); err != nil {
		t.Fatal(err)
	}
	empty, err := collect(ctx, db, "qs.mysql", cfg.Addr)
	if err != nil || !empty.Complete || empty.Tables[1].EmptyExport == nil || !empty.Tables[1].EmptyExport.EOF || empty.Tables[1].EmptyExport.StreamSHA256 != hash("") {
		t.Fatal("empty original stream export unproven", err)
	}
	ledger := empty.Tables[1].EmptyExport.OriginalLedger
	if ledger == nil || len(ledger.Rows) != 1 || ledger.Rows[0].Version != 110 || ledger.Rows[0].Dirty || ledger.OriginalDDL == "" || ledger.DDLHash != hash(ledger.OriginalDDL) {
		t.Fatal("complete original schema ledger not preserved")
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE schema_migrations ADD COLUMN owned_unknown_metadata VARBINARY(16) NULL"); err != nil {
		t.Fatal(err)
	}
	malformed, readErr := collect(ctx, db, "qs.mysql", cfg.Addr)
	if readErr == nil || malformed.Complete || malformed.Tables[1].EmptyExport != nil || !malformed.TransactionReleased {
		t.Fatal("unknown ledger fields were silently omitted", readErr)
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE schema_migrations DROP COLUMN owned_unknown_metadata"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "DROP TABLE ai_messaging_outbox"); err != nil {
		t.Fatal(err)
	}
	r, err := collect(ctx, db, "qs.mysql", cfg.Addr)
	if err == nil || r.Complete || r.Tables[len(r.Tables)-1].Rows != nil {
		t.Fatal("missing table concealed", r, err)
	}
}
