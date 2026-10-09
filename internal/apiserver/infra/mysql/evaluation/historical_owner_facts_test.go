package evaluation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSQLHistoricalOwnerFactsRejectsFakeBorrowedTransactions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	g, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var typednil *sql.Tx
	for _, pool := range []gorm.ConnPool{db, &historicalUnknownPool{ConnPool: tx}, typednil, &gorm.PreparedStmtTX{Tx: typednil}} {
		candidate := g.Session(&gorm.Session{NewDB: true})
		candidate.Statement.ConnPool = pool
		if _, err := PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), candidate), 42, "original-event", strings.Repeat("a", 64)); err == nil {
			t.Fatal("fake borrowed transaction accepted")
		}
	}
	candidate := g.Session(&gorm.Session{NewDB: true})
	candidate.Statement.ConnPool = tx
	mock.ExpectQuery(regexp.QuoteMeta("SELECT @@server_uuid AS server,DATABASE() AS `database`")).WillReturnRows(sqlmock.NewRows([]string{"server", "database"}).AddRow("actual-server", "actual-schema"))
	if _, err := PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), candidate), 42, "original-event", strings.Repeat("a", 64)); !errors.Is(err, ErrSQLHistoricalFactsIdentity) {
		t.Fatal("unrelated actual database accepted", err)
	}
	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoricalDatabaseIdentityMatchesIndependentInventoryFraming(t *testing.T) {
	h := sha256.New()
	for _, part := range []string{"mysql_database_identity_v1", "server-uuid", "database-name"} {
		frame := []byte{1, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint64(frame[1:], uint64(len(part)))
		_, _ = h.Write(frame)
		_, _ = h.Write([]byte(part))
	}
	if sqlHistoricalIdentity("server-uuid", "database-name") != hex.EncodeToString(h.Sum(nil)) {
		t.Fatal("inventory identity protocol drift")
	}
}

func TestSQLHistoricalCurrentMessageRejectsDuplicateAndUnknownBody(t *testing.T) {
	for _, raw := range []string{`{"org_id":7,"org_id":8}`, `{"org_id":7} {"org_id":7}`, strings.Repeat("[", 65) + strings.Repeat("]", 65)} {
		if err := sqlHistoricalUniqueJSON([]byte(raw)); err == nil {
			t.Fatal("ambiguous current payload accepted")
		}
	}
	if err := sqlHistoricalStrictJSON([]byte(`{"extra":1}`), new(sqlHistoricalEventOwner)); err == nil {
		t.Fatal("unknown current payload accepted")
	}
}

func TestSQLHistoricalCurrentResponsibilityUsesActualSDKWire(t *testing.T) {
	_, wire := testCommittedReference(t, 9001, 42, "actual-current-event")
	row := historicalSQLRow{}
	for name, value := range map[string]string{"id": "1", "producer": wire.Producer, "message_id": wire.MessageID, "destination": wire.Destination, "event_type": wire.EventType, "schema_version": wire.SchemaVersion, "scope": wire.Scope, "content_type": wire.ContentType, "occurred_at": wire.OccurredAt, "payload": string(wire.Payload), "fingerprint": string(wire.Fingerprint), "state": "published", "transport_confirmed_at": "2026-10-08 07:00:00.123"} {
		v := value
		row[name] = &v
	}
	fact := sqlHistoricalResponsibility("rm_outbox", row, 42, 7)
	if fact.Invalid || fact.Unfinished || fact.AssessmentID != 42 || fact.OrgID != 7 {
		t.Fatalf("actual SDK fixture rejected: %+v", fact)
	}
}

func TestSQLHistoricalOwnerFactsPrivateSnapshotCannotSerialize(t *testing.T) {
	v := SQLHistoricalFactsSnapshot{Owner: SQLHistoricalOwner{ConductingContextBytes: []byte("private raw anchor")}}
	if _, err := json.Marshal(v); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
		t.Fatal("private SQL JSON snapshot serialized")
	}
	if _, err := bson.Marshal(v); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
		t.Fatal("private SQL BSON snapshot serialized")
	}
}
