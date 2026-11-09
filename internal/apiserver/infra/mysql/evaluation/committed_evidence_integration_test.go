//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/testutil/legacymysqloutbox"
	"github.com/FangcunMount/reliable-messaging/message"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	drivermysql "github.com/go-sql-driver/mysql"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openCommittedEvidenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("QS_COMPAT_OUTCOME_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_COMPAT_REQUIRE_DATABASE") == "1" {
			t.Fatal("QS_COMPAT_OUTCOME_MYSQL_DSN is required")
		}
		t.Skip("isolated Outcome evidence database is not configured")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.MultiStatements = true
	cfg.Loc = time.UTC
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("qs_compat_outcome_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`"); err != nil {
		if closeErr := admin.Close(); closeErr != nil {
			t.Errorf("close fixture administrator after setup failure: %v", closeErr)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`")
		if err != nil {
			t.Errorf("drop owned fixture: %v", err)
		}
		if closeErr := admin.Close(); closeErr != nil {
			t.Errorf("close owned fixture administrator: %v", closeErr)
		}
	})
	cfg.DBName = name
	db, err := gorm.Open(mysqlDriver.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Errorf("close owned fixture business pool: %v", closeErr)
		}
	})
	if err := db.Exec("CREATE TABLE assessment(id BIGINT UNSIGNED PRIMARY KEY,org_id BIGINT,testee_id BIGINT UNSIGNED,status VARCHAR(32),deleted_at DATETIME(3) NULL)").Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000042_add_evaluation_outcome", "000043_add_evaluation_outcome_report_context", "000053_drop_evaluation_outcome_payload_format", "000058_drop_evaluation_outcome_algorithm_family", "000084_standard_reliable_outbox", "000096_evaluation_committed_event_evidence"} {
		body, err := os.ReadFile(filepath.Join("../../../../pkg/migration/migrations/mysql", name+".up.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(body)).Error; err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
	return db
}

func stageCommittedTestEvent(ctx context.Context, stager *mysqlstandard.Stager, row committedStandardRow) error {
	outer, _, err := legacy.Decode(row.Payload)
	if err != nil {
		return err
	}
	inner, err := domainwire.DecodeEnvelope(outer.Payload)
	if err != nil {
		return err
	}
	evt := event.Event[json.RawMessage]{BaseEvent: event.BaseEvent{ID: inner.ID, EventTypeValue: inner.EventType, AggregateTypeValue: inner.AggregateType, AggregateIDValue: inner.AggregateID, OccurredAtValue: inner.OccurredAt}, Data: inner.Data}
	return stager.Stage(ctx, evt)
}

func TestCommittedEvidenceAtomicNativeMySQL(t *testing.T) {
	db := openCommittedEvidenceDB(t)
	record, row := testCommittedReference(t, 9001, 42, "native-committed")
	cfg, err := eventcatalog.Load("../../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(cfg), "api-server")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewOutcomeRepository(db)
	sentinel := errors.New("failure after business and standard outbox writes")
	write := func(fail bool) error {
		return db.Transaction(func(tx *gorm.DB) error {
			ctx := hostmysql.WithTx(t.Context(), tx)
			if err := repo.Save(ctx, record); err != nil {
				return err
			}
			if err := stageCommittedTestEvent(ctx, stager, row); err != nil {
				return err
			}
			if fail {
				return sentinel
			}
			return nil
		})
	}
	if err := write(true); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	for _, table := range []string{"evaluation_outcome", "rm_outbox"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("rollback %s count=%d error=%v", table, count, err)
		}
	}
	if err := write(false); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.FindByID(t.Context(), record.ID())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CommittedEventID() != record.CommittedEventID() || loaded.BusinessBindingSHA256() != record.BusinessBindingSHA256() {
		t.Fatal("native JSON/time mapping changed original evidence")
	}
	reader := &consistencyReadModel{db: db}
	proof, err := reader.listCommittedOutboxEvidence(t.Context(), []uint64{42})
	if err != nil {
		t.Fatal(err)
	}
	if proof[42] == nil || proof[42].InvalidReason != "" || proof[42].RowCount != 1 {
		t.Fatalf("forward=%#v", proof[42])
	}
	upper, err := reader.OutboxUpperBound(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	orphan, orphanRow := testCommittedReference(t, 9002, 43, "native-orphan")
	_ = orphan
	if err := db.Transaction(func(tx *gorm.DB) error {
		return stageCommittedTestEvent(hostmysql.WithTx(t.Context(), tx), stager, orphanRow)
	}); err != nil {
		t.Fatal(err)
	}
	fixed, err := reader.ReadOutboxBatch(t.Context(), 0, upper, 1)
	if err != nil || len(fixed.Conflicts) != 0 || fixed.NextCursor != upper {
		t.Fatalf("fixed=%#v err=%v", fixed, err)
	}
	nextUpper, err := reader.OutboxUpperBound(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	batch, err := reader.ReadOutboxBatch(t.Context(), 0, nextUpper, 10)
	if err != nil || len(batch.Conflicts) != 1 || batch.Conflicts[0].MessageID != "native-orphan" {
		t.Fatalf("reverse=%#v err=%v", batch, err)
	}
	// Install the compatibility view over an exact copy and hide the original
	// name. The original fixture table stays intact throughout this proof.
	originalBytes := readEvidenceRows(t, db, "rm_outbox")
	for _, statement := range []string{
		"CREATE TABLE isolated_outbox_archive LIKE rm_outbox",
		"INSERT INTO isolated_outbox_archive SELECT * FROM rm_outbox",
		"CREATE SQL SECURITY INVOKER VIEW qs_rm_outbox_history AS SELECT * FROM isolated_outbox_archive",
		"RENAME TABLE rm_outbox TO isolated_original_rm_outbox",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(originalBytes, readEvidenceRows(t, db, "qs_rm_outbox_history")) {
		t.Fatal("archive changed the original Outbox rows")
	}
	archived, err := reader.listCommittedOutboxEvidence(t.Context(), []uint64{42})
	if err != nil || !reflect.DeepEqual(proof, archived) {
		t.Fatalf("archived forward=%#v err=%v", archived, err)
	}
	archivedUpper, err := reader.OutboxUpperBound(t.Context())
	if err != nil || archivedUpper != nextUpper {
		t.Fatalf("archived upper=%d want=%d err=%v", archivedUpper, nextUpper, err)
	}
	archivedBatch, err := reader.ReadOutboxBatch(t.Context(), 0, archivedUpper, 10)
	if err != nil || !reflect.DeepEqual(batch, archivedBatch) {
		t.Fatalf("archived reverse=%#v err=%v", archivedBatch, err)
	}
	// A broken view must report an error even when the old table is available.
	for _, statement := range []string{
		"RENAME TABLE isolated_original_rm_outbox TO rm_outbox",
		"RENAME TABLE isolated_outbox_archive TO isolated_archive_unavailable",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := reader.OutboxUpperBound(t.Context()); err == nil {
		t.Fatal("broken history view silently selected the old table")
	}
	if err := db.Exec("DROP VIEW qs_rm_outbox_history").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE qs_rm_outbox_history (id BIGINT UNSIGNED PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := reader.OutboxUpperBound(t.Context()); err == nil {
		t.Fatal("a physical table shadowed the expected compatibility view")
	}
	if err := db.Exec("RENAME TABLE qs_rm_outbox_history TO isolated_history_shadow").Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(originalBytes, readEvidenceRows(t, db, "rm_outbox")) {
		t.Fatal("retained original table changed during the read-only proof")
	}
	// Even a forged SDK outer type/fingerprint cannot hide a committed orphan.
	altered := orphanRow.input()
	altered.EventType = eventcatalog.EvaluationRequested
	forged, err := message.New(altered)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := forged.Fingerprint()
	if err := db.Exec("UPDATE rm_outbox SET event_type=?,fingerprint=UNHEX(?) WHERE message_id=?", altered.EventType, hex.EncodeToString(fingerprint[:]), orphanRow.MessageID).Error; err != nil {
		t.Fatal(err)
	}
	disguised, err := reader.ReadOutboxBatch(t.Context(), 0, nextUpper, 10)
	if err != nil || len(disguised.Conflicts) != 1 || disguised.Conflicts[0].MessageID != orphanRow.MessageID {
		t.Fatalf("disguised orphan=%#v err=%v", disguised, err)
	}
	if err := db.Exec("UPDATE rm_outbox SET payload=CONCAT(payload,' ') WHERE message_id=?", row.MessageID).Error; err != nil {
		t.Fatal(err)
	}
	proof, err = reader.listCommittedOutboxEvidence(t.Context(), []uint64{42})
	if err != nil || proof[42].InvalidReason == "" {
		t.Fatalf("tamper=%#v err=%v", proof[42], err)
	}
	// No retired table was created anywhere in this fixture.
}

func readEvidenceRows(t *testing.T, db *gorm.DB, table string) [][][]byte {
	t.Helper()
	rows, err := db.Table(table).Order("id ASC").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][][]byte
	for rows.Next() {
		values := make([][]byte, len(columns))
		fields := make([]any, len(columns))
		for i := range fields {
			fields[i] = &values[i]
		}
		if err := rows.Scan(fields...); err != nil {
			t.Fatal(err)
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
