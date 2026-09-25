package eventoutbox

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	qsMysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	sdkMysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormMysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// The SDK Appender must borrow QS's actual UoW transaction and +08:00 pool.
// This is a bridge proof, not a substitute for a full assessment workflow.
func TestStandardAppenderOriginalQSUoWMySQL(t *testing.T) {
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set QS_SERVER_TEST_MYSQL_DSN to a disposable local m4_fenced database")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Net != "tcp" || !strings.HasPrefix(parsed.Addr, "127.0.0.1:") || parsed.DBName != "m4_fenced" {
		t.Fatal("standard bridge proof requires a localhost disposable m4_fenced database")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(3)
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var zone string
	if err := db.QueryRowContext(ctx, `SELECT @@session.time_zone`).Scan(&zone); err != nil || zone != "+08:00" {
		t.Fatalf("session zone=%q want +08:00 err=%v", zone, err)
	}
	for _, name := range []string{"rm_outbox", "m4_standard_bridge_fact"} {
		if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+name); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS m4_standard_bridge_fact`)
		_, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS rm_outbox`)
	})
	if _, err := db.ExecContext(ctx, sdkMysql.Schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE m4_standard_bridge_fact (id VARCHAR(64) PRIMARY KEY, value VARCHAR(128) NOT NULL) ENGINE=InnoDB`); err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(gormMysql.New(gormMysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	uow := qsMysql.NewUnitOfWork(gormDB)
	location := time.FixedZone("UTC+8", 8*3600)
	due := time.Now().In(location).Add(10 * time.Minute).Truncate(time.Microsecond)
	occurredAt := time.Now().In(location).Format(time.RFC3339Nano)
	makeMessage := func(id string, payload []byte) message.Message {
		t.Helper()
		m, err := message.New(message.Input{
			Producer: "qs-server", ID: id, Destination: "qs.evaluation.lifecycle", EventType: "evaluation.requested",
			SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: occurredAt,
			Payload: payload,
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	stage := func(factID string, m message.Message, finish error) error {
		return uow.WithinTransaction(ctx, func(txCtx context.Context) error {
			tx, err := qsMysql.RequireTx(txCtx)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO m4_standard_bridge_fact(id,value) VALUES (?,?)`, factID, "committed").Error; err != nil {
				return err
			}
			appender, err := sdkMysql.BindGORM(tx)
			if err != nil {
				return err
			}
			if err := appender.Append(txCtx, m, due); err != nil {
				return err
			}
			return finish
		})
	}
	firstPayload := []byte(`{"id":"m4-standard-one","type":"evaluation.requested"}`)
	first := makeMessage("m4-standard-one", firstPayload)
	if err := stage("fact-one", first, nil); err != nil {
		t.Fatalf("commit fact and message: %v", err)
	}
	var factCount, messageCount int
	var dueText string
	var storedPayload []byte
	if err := db.QueryRowContext(ctx, `SELECT DATE_FORMAT(next_attempt_at,'%Y-%m-%d %H:%i:%s.%f'),payload FROM rm_outbox WHERE message_id='m4-standard-one'`).Scan(&dueText, &storedPayload); err != nil {
		t.Fatal(err)
	}
	if dueText != due.UTC().Format(fencedTimeLayout) || !bytes.Equal(storedPayload, firstPayload) {
		t.Fatalf("standard due/payload changed: due=%q payload_equal=%t", dueText, bytes.Equal(storedPayload, firstPayload))
	}
	store, err := sdkMysql.New(db)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := store.ClaimDue(ctx, 1, time.Second)
	if err != nil || len(claims) != 0 {
		t.Fatalf("delayed message claimed early: claims=%d err=%v", len(claims), err)
	}
	wantRollback := errors.New("abort original QS transaction")
	if err := stage("fact-rolled-back", makeMessage("m4-standard-rolled-back", []byte(`{"id":"rolled-back"}`)), wantRollback); !errors.Is(err, wantRollback) {
		t.Fatalf("rollback result: %v", err)
	}
	conflict := makeMessage("m4-standard-one", []byte(`{"id":"m4-standard-one","changed":true}`))
	if err := stage("fact-conflict", conflict, nil); !errors.Is(err, outbox.ErrConflict) {
		t.Fatalf("identity conflict: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM m4_standard_bridge_fact`).Scan(&factCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rm_outbox`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if factCount != 1 || messageCount != 1 {
		t.Fatalf("atomicity: facts=%d messages=%d", factCount, messageCount)
	}
	if err := db.QueryRowContext(ctx, `SELECT payload FROM rm_outbox WHERE message_id='m4-standard-one'`).Scan(&storedPayload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(storedPayload, firstPayload) {
		t.Fatal("conflicting retry changed immutable stored payload")
	}
}
