//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
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

func storedHistoricalSet(t *testing.T, db *gorm.DB, table, column string, id uint64) *evidence.HistoricalReferenceSetV1 {
	t.Helper()
	var raw *string
	if err := db.Raw("SELECT `"+column+"` FROM `"+table+"` WHERE id=?", id).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	set, err := historicalDecode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestHistoricalReferencesAppendRollbackAndPreservationNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	run := evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
	var baseline *AssessmentHistoricalBaseline
	var entry evidence.HistoricalReferenceEntryV1
	rollback := errors.New("rollback after append")
	if err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		var err error
		baseline, err = PrepareAssessmentHistoricalReferences(ctx, 42, run)
		if err != nil {
			return err
		}
		binding, err := baseline.BindingSHA256("evaluation.requested", &run)
		if err != nil {
			return err
		}
		entry = nativeHistoricalEntry("old-request-1", "evaluation.requested", binding, &run)
		if err := AppendAssessmentHistoricalReferences(ctx, baseline, entry); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42) != nil {
		t.Fatal("borrowed transaction rollback leaked evidence")
	}
	for _, id := range []string{"old-request-1", "old-request-2", "old-request-1"} {
		next := entry.Clone()
		next.EventID = id
		next.Proof.EventID = id
		next.Source.Digest = evidence.SourceDigest("mysql-original-source-row-v1", []byte(id))
		next.Proof.Digest = next.Source.Digest
		next.Source.PrimaryKeySHA256 = evidence.SourceDigest("mysql-pk-v1", []byte(id)).SHA256
		if err := db.Transaction(func(tx *gorm.DB) error {
			return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, next)
		}); err != nil {
			t.Fatal(err)
		}
	}
	set := storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42)
	if len(set.Entries) != 2 {
		t.Fatal("multiple original IDs collapsed", len(set.Entries))
	}
	changed := entry.Clone()
	changed.Proof.Verification.OperationID = "123-2"
	if err := db.Transaction(func(tx *gorm.DB) error {
		return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, changed)
	}); !errors.Is(err, evidence.ErrHistoricalReferenceConflict) {
		t.Fatal("different proof accepted", err)
	}
	repo := NewAssessmentRepository(db)
	a, err := repo.FindByID(t.Context(), baseline.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if got := storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42); len(got.Entries) != 2 {
		t.Fatal("normal Save lost maintenance evidence")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, entry)
	}); !errors.Is(err, evidence.ErrHistoricalReferenceConflict) {
		t.Fatal("idempotency hid changed business baseline", err)
	}
}

func TestHistoricalReferencesOutcomeAndUnknownFactsNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "current-standard-id")
	if err := NewOutcomeRepository(db).Save(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	var baseline *OutcomeHistoricalBaseline
	var first evidence.HistoricalReferenceEntryV1
	run := evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
	if err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		var err error
		baseline, err = PrepareOutcomeHistoricalReferences(ctx, 9001, run)
		if err != nil {
			return err
		}
		binding, err := baseline.BindingSHA256("evaluation.outcome.committed", &run)
		if err != nil {
			return err
		}
		first = nativeHistoricalEntry("old-committed-1", "evaluation.outcome.committed", binding, &run)
		return AppendOutcomeHistoricalReferences(ctx, baseline, first)
	}); err != nil {
		t.Fatal(err)
	}
	second := first.Clone()
	second.EventID, second.Proof.EventID = "old-committed-2", "old-committed-2"
	if err := db.Transaction(func(tx *gorm.DB) error {
		return AppendOutcomeHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, second)
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewOutcomeRepository(db).FindByID(t.Context(), record.ID())
	if err != nil || loaded.CommittedEventID() != "current-standard-id" || loaded.CommittedEventEvidence().Class != evidence.StandardReferenceClass {
		t.Fatal("history replaced standard reference", err)
	}
	if len(storedHistoricalSet(t, db, "evaluation_outcome", "historical_committed_evidence", 9001).Entries) != 2 {
		t.Fatal("outcome IDs collapsed")
	}
	if err := db.Exec("ALTER TABLE evaluation_outcome ADD verifier_canary VARBINARY(8) NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return AppendOutcomeHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, first)
	}); !errors.Is(err, evidence.ErrHistoricalReferenceConflict) {
		t.Fatal("unknown schema fact change accepted", err)
	}
}

func TestHistoricalReferencesConcurrencyCapacityAndConstraintsNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	var baseline *AssessmentHistoricalBaseline
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		baseline, err = PrepareAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), 42)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	binding, err := baseline.BindingSHA256("evaluation.failed", nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errorsFound := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := nativeHistoricalEntry(fmt.Sprintf("concurrent-%d", i), "evaluation.failed", binding, nil)
			errorsFound <- db.Transaction(func(tx *gorm.DB) error {
				return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, e)
			})
		}(i)
	}
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	set := storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42)
	if len(set.Entries) != 2 {
		t.Fatal("concurrent append lost reference")
	}
	entries := make([]evidence.HistoricalReferenceEntryV1, evidence.HistoricalReferenceMaxEntries-2)
	for i := range entries {
		entries[i] = nativeHistoricalEntry(fmt.Sprintf("capacity-%d", i), "evaluation.failed", binding, nil)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, entries...)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), baseline, nativeHistoricalEntry("overflow", "evaluation.failed", binding, nil))
	}); !errors.Is(err, evidence.ErrHistoricalReferenceLimit) {
		t.Fatal("capacity overflow accepted", err)
	}
	valid := storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42)
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"version":1,"entries":[]}`, strings.Replace(string(raw), `"evaluation.failed"`, `"evaluation.outcome.committed"`, 1), strings.Replace(string(raw), `"retired_verified"`, `"standard_reference"`, 1), strings.Replace(string(raw), `"responsibility_closed":true`, `"responsibility_closed":false`, 1)} {
		if err := db.Exec("UPDATE assessment SET historical_lifecycle_evidence=? WHERE id=42", bad).Error; err == nil {
			t.Fatal("native CHECK accepted invalid history")
		}
	}
	oversized := valid.Clone()
	for i := range oversized.Entries {
		oversized.Entries[i].Proof.Verification.Reason = strings.Repeat("r", 4096)
	}
	tooLarge, err := json.Marshal(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE assessment SET historical_lifecycle_evidence=? WHERE id=42", string(tooLarge)).Error; err == nil {
		t.Fatal("native byte CHECK accepted oversized history")
	}
	down, err := os.ReadFile(filepath.Join("../../../../pkg/migration/migrations/mysql", "000099_business_historical_references.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(down)).Error; err == nil {
		t.Fatal("destructive automatic down allowed")
	}
	if len(storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42).Entries) != 128 {
		t.Fatal("failed update/down changed history")
	}
}

func TestHistoricalReferencesExactFactsAndOldSnapshotNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	prepare := func() *AssessmentHistoricalBaseline {
		var b *AssessmentHistoricalBaseline
		if err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			b, err = PrepareAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), 42)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, change := range []struct {
		column      string
		replacement any
	}{{"org_id", int64(8)}, {"questionnaire_code", "q"}, {"origin_id", ""}} {
		b := prepare()
		hash, err := b.BindingSHA256("evaluation.requested", nil)
		if err != nil {
			t.Fatal(err)
		}
		e := nativeHistoricalEntry("changed-"+change.column, "evaluation.requested", hash, nil)
		if err := db.Exec("UPDATE assessment SET `"+change.column+"`=? WHERE id=42", change.replacement).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), b, e)
		}); !errors.Is(err, evidence.ErrHistoricalReferenceConflict) {
			t.Fatal("original owner/bytes/NULL drift accepted", change.column, err)
		}
		var original any
		if b.baseline.row[change.column] != nil {
			original = *b.baseline.row[change.column]
		}
		if err := db.Exec("UPDATE assessment SET `"+change.column+"`=? WHERE id=42", original).Error; err != nil {
			t.Fatal(err)
		}
	}
	b := prepare()
	hash, err := b.BindingSHA256("evaluation.requested", nil)
	if err != nil {
		t.Fatal(err)
	}
	e := nativeHistoricalEntry("old-snapshot", "evaluation.requested", hash, nil)
	old := db.Begin()
	if old.Error != nil {
		t.Fatal(old.Error)
	}
	defer func() { _ = old.Rollback().Error }()
	var oldView *string
	if err := old.Raw("SELECT historical_lifecycle_evidence FROM assessment WHERE id=42").Scan(&oldView).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), b, e)
	}); err != nil {
		t.Fatal(err)
	}
	if err := AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), old), b, e); err != nil {
		t.Fatal("FOR UPDATE failed old-snapshot idempotency", err)
	}
	if err := old.Commit().Error; err != nil {
		t.Fatal(err)
	}
	other := openHistoricalReferencesDB(t)
	if err := other.Transaction(func(tx *gorm.DB) error {
		return AppendAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), b, e)
	}); !errors.Is(err, evidence.ErrHistoricalReferenceConflict) {
		t.Fatal("cross-database baseline accepted", err)
	}
	run := evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
	for _, mutation := range []string{"UPDATE runtime_checkpoint SET lease_expires_at=DATE_ADD(NOW(3),INTERVAL 1 HOUR) WHERE assessment_id=42", "UPDATE runtime_checkpoint SET lease_expires_at=NULL,finished_at=NULL WHERE assessment_id=42"} {
		if err := db.Exec(mutation).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			_, err := PrepareAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), 42, run)
			return err
		}); !errors.Is(err, evidence.ErrHistoricalReferenceConflict) {
			t.Fatal("unfinished/leased original run accepted", err)
		}
	}
	if err := db.Exec("UPDATE runtime_checkpoint SET scope='EVALUATION_RUN',lease_expires_at=NULL,finished_at=NOW(3) WHERE assessment_id=42").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := PrepareAssessmentHistoricalReferences(hostmysql.WithTx(t.Context(), tx), 42, run)
		return err
	}); !errors.Is(err, evidence.ErrHistoricalReferenceConflict) {
		t.Fatal("casefold checkpoint scope accepted as original run", err)
	}
}
