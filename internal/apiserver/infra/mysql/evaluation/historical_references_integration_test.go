//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	evidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	drivermysql "github.com/go-sql-driver/mysql"
	golangmigrate "github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openHistoricalReferencesDB(t *testing.T, from98 ...bool) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("QS_HISTORY_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_HISTORY_REQUIRE_DATABASE") == "1" {
			t.Fatal("isolated native fixture required")
		}
		t.Skip("isolated native historical fixture not configured")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("fixture DSN invalid")
	}
	if cfg.Net != "tcp" || !strings.HasPrefix(cfg.Addr, "127.0.0.1:") {
		t.Fatal("fixture must be local loopback")
	}
	cfg.DBName = ""
	cfg.ParseTime, cfg.MultiStatements, cfg.Loc = true, true, time.UTC
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("qs_compat_history_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Log("created previously absent owned schema", name)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("owned schema cleanup: %v", err)
		}
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
		t.Log("removed owned schema", name)
	})
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	// These historical evidence tests retain the production A schema and old
	// source objects at SQL99. B's paired empty/installed upgrade is covered by
	// its separate native tests; this fixture must never request its DROP tail.
	source, err := iofs.New(os.DirFS("../../../../pkg/migration/migrations/mysql"), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	}()
	conn, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	driver, err := migratemysql.WithConnection(t.Context(), conn, &migratemysql.Config{DatabaseName: name, MigrationsTable: "schema_migrations"})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := golangmigrate.NewWithInstance("iofs", source, name, driver)
	if err != nil {
		t.Fatal(err)
	}
	if len(from98) > 0 && from98[0] {
		if err := instance.Migrate(98); err != nil {
			t.Fatal(err)
		}
		version, dirty, err := instance.Version()
		if err != nil || version != 98 || dirty {
			t.Fatalf("actual old-head baseline: head=%d dirty=%v error=%v", version, dirty, err)
		}
		t.Log("actual complete resources upgraded to head98 clean before additive99")
	}
	for attempt := 0; attempt < 2; attempt++ {
		err := instance.Migrate(99)
		if (attempt == 0 && err != nil) || (attempt == 1 && !errors.Is(err, golangmigrate.ErrNoChange)) {
			t.Fatalf("actual A99 upgrade/restart attempt=%d error=%v", attempt, err)
		}
		version, dirty, err := instance.Version()
		if err != nil || version != 99 || dirty {
			t.Fatalf("actual A99 clean head: head=%d dirty=%v error=%v", version, dirty, err)
		}
	}
	return db
}

func TestHistoricalReferencesUpgradeFrom98Native(t *testing.T) {
	db := openHistoricalReferencesDB(t, true)
	var count int
	if err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND ((table_name='assessment' AND column_name='historical_lifecycle_evidence') OR (table_name='evaluation_outcome' AND column_name='historical_committed_evidence'))").Scan(&count).Error; err != nil || count != 2 {
		t.Fatal("additive99 columns missing", count, err)
	}
}

func insertHistoricalAssessment(t *testing.T, db *gorm.DB, id uint64) {
	t.Helper()
	at := time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)
	if err := db.Exec("INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,created_at,updated_at,submitted_at,evaluated_at,version) VALUES(?,7,21,'Q','1.0',?,'adhoc','evaluated',?,?,?,?,1)", id, 10000+id, at, at, at, at).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at,finished_at) VALUES('evaluation_run',?,1,?,'succeeded',?,?)", fmt.Sprintf("%d:1", id), id, at, at).Error; err != nil {
		t.Fatal(err)
	}
}

func nativeHistoricalEntry(id, eventType, binding string, run *evidence.HistoricalRunReferenceV1) evidence.HistoricalReferenceEntryV1 {
	digest := evidence.SourceDigest("mysql-original-source-row-v1", []byte(id))
	return evidence.HistoricalReferenceEntryV1{EventID: id, EventType: eventType, Source: evidence.HistoricalSourceReferenceV1{Database: "mysql", Object: "domain_event_outbox", PrimaryKeyKind: "mysql_uint64", PrimaryKeySHA256: evidence.SourceDigest("mysql-pk-v1", []byte(id)).SHA256, Digest: digest}, Proof: &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: id, Digest: digest, BusinessBindingSHA256: binding, Origin: "retirement", Verification: evidence.Verification{Method: "native-historical-verifier", Version: "v1", OperationID: "123-1", VerifiedAt: time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}, Run: run}
}

// Audit fixtures load already persisted conclusions. These helpers deliberately
// provide no source verification, CAS/retry, admission or migration capability.
func historicalFixtureBinding(db *gorm.DB, table string, id uint64, eventType string, run *evidence.HistoricalRunReferenceV1) (string, error) {
	server, database, err := historicalDatabase(db)
	if err != nil {
		return "", err
	}
	rows, err := historicalAuditRows(db, table, "id=?", 1, id)
	if err != nil {
		return "", err
	}
	if len(rows) != 1 {
		return "", gorm.ErrRecordNotFound
	}
	var originalRun historicalSQLRow
	if run != nil {
		values, err := historicalAuditRows(db, "runtime_checkpoint", "scope='evaluation_run' AND resource_id=? AND attempt_no=?", 1, run.RunID, run.Attempt)
		if err != nil {
			return "", err
		}
		if len(values) != 1 {
			return "", gorm.ErrRecordNotFound
		}
		originalRun = values[0]
	}
	return historicalStableBinding(server, database, table, eventType, rows[0], originalRun)
}
func appendHistoricalFixture(db *gorm.DB, table, column string, id uint64, entries ...evidence.HistoricalReferenceEntryV1) error {
	var raw *string
	if err := db.Raw("SELECT `"+column+"` FROM `"+table+"` WHERE id=?", id).Scan(&raw).Error; err != nil {
		return err
	}
	set, err := historicalDecode(raw)
	if err != nil {
		return err
	}
	if set == nil {
		set = &evidence.HistoricalReferenceSetV1{Version: 1}
	}
	set.Entries = append(set.Entries, entries...)
	encoded, err := json.Marshal(set)
	if err != nil {
		return err
	}
	return db.Exec("UPDATE `"+table+"` SET `"+column+"`=? WHERE id=?", string(encoded), id).Error
}
