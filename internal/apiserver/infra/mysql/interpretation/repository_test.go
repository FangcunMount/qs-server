package interpretation

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/catalogreconcile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	drivermysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAdmissionFailurePORoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 15, 9, 0, 0, 123000000, time.UTC)
	failure, err := admission.NewFailure(admission.Input{
		ID: meta.FromUint64(11), OutcomeID: meta.FromUint64(22), OrgID: 7,
		AssessmentID: meta.FromUint64(33), TesteeID: 44, EventID: "evt-1", TraceID: "trace-1",
		Kind: admission.KindCatalogNotFound, Code: "catalog_not_found", SafeMessage: "catalog is unavailable",
		Retryable: true, GenerationID: meta.FromUint64(55), OutcomeVersion: "v3", Attempt: 4,
		Decision: "retryable", FirstFailedAt: at.Add(-time.Minute), LastFailedAt: at, OccurredAt: at.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	po := admissionFailureToPO(failure)
	roundTrip, err := admissionFailureToDomain(po)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.ID() != failure.ID() || roundTrip.Fingerprint() != failure.Fingerprint() || roundTrip.Attempt() != 4 {
		t.Fatalf("round trip = id:%s fingerprint:%s attempt:%d", roundTrip.ID(), roundTrip.Fingerprint(), roundTrip.Attempt())
	}
	if !roundTrip.LastFailedAt().Equal(at) || roundTrip.OutcomeVersion() != "v3" || !roundTrip.Retryable() {
		t.Fatalf("round trip lost durable evidence: %#v", roundTrip)
	}
}

func TestAdmissionFailureDuplicateIncrementsAttempt(t *testing.T) {
	db, mock := newMockGorm(t)
	at := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	failure, err := admission.NewFailure(admission.Input{
		ID: meta.FromUint64(11), OutcomeID: meta.FromUint64(22), EventID: "evt-1",
		Kind: admission.KindCatalogNotFound, Code: "catalog_not_found", SafeMessage: "missing", OccurredAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `interpretation_admission_failure`")).
		WillReturnError(&drivermysql.MySQLError{Number: 1062, Message: "Duplicate entry for uk_interpretation_admission_failure_fingerprint"})
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `interpretation_admission_failure` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	created, err := NewAdmissionFailureRepository(db).UpsertByFingerprint(context.Background(), failure)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("duplicate evidence was reported as created")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogAuditCheckpointPORoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 15, 10, 0, 0, 456000000, time.UTC)
	checkpoint := catalogreconcile.AuditCheckpoint{
		SchemaVersion: 1, Revision: 7, CycleID: "cycle-1", Phase: catalogreconcile.AuditPhaseCompleted,
		AfterAssessmentID: 12, SourceUpperAssessmentID: 20, CatalogUpperAssessmentID: 19,
		WorkingCounts:    catalogreconcile.DriftCounts{Missing: 1, Dangling: 2},
		WorkingOrgCounts: map[int64]catalogreconcile.DriftCounts{7: {AssociationMismatch: 3}},
		LastCompleted: &catalogreconcile.CompletedAuditSnapshot{
			CycleID: "cycle-1", CompletedAt: at, Counts: catalogreconcile.DriftCounts{Missing: 1},
			OrgCounts: map[int64]catalogreconcile.DriftCounts{7: {WrongWinner: 4}},
		},
		NextCycleAt: at.Add(24 * time.Hour), UpdatedAt: at,
	}
	po, err := catalogAuditCheckpointToPO(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := catalogAuditCheckpointFromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.Revision != checkpoint.Revision || roundTrip.WorkingCounts.Dangling != 2 || roundTrip.WorkingOrgCounts[7].AssociationMismatch != 3 {
		t.Fatalf("round trip checkpoint = %#v", roundTrip)
	}
	if roundTrip.LastCompleted == nil || roundTrip.LastCompleted.OrgCounts[7].WrongWinner != 4 || !roundTrip.NextCycleAt.Equal(checkpoint.NextCycleAt) {
		t.Fatalf("round trip completed snapshot = %#v", roundTrip.LastCompleted)
	}
}

func TestCatalogAuditCheckpointSaveRejectsStaleRevision(t *testing.T) {
	db, mock := newMockGorm(t)
	at := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `interpretation_catalog_audit_checkpoint` SET")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := NewCatalogAuditCheckpointRepository(db).SaveAuditCheckpoint(context.Background(), 1, catalogreconcile.AuditCheckpoint{
		SchemaVersion: 1, Revision: 2, CycleID: "cycle-1", Phase: catalogreconcile.AuditPhaseMissing,
		WorkingOrgCounts: map[int64]catalogreconcile.DriftCounts{}, UpdatedAt: at,
	})
	if !errors.Is(err, catalogreconcile.ErrAuditCheckpointCAS) {
		t.Fatalf("save error = %v, want checkpoint CAS", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMergeImportedAuditCheckpointPreservesNewerLiveCycle(t *testing.T) {
	currentAt := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	sourceCompletedAt := currentAt.Add(-2 * time.Hour)
	current := catalogreconcile.AuditCheckpoint{
		SchemaVersion: 1, Revision: 510, CycleID: "mysql-live", Phase: catalogreconcile.AuditPhaseMissing,
		AfterAssessmentID: 100, UpdatedAt: currentAt,
	}
	source := catalogreconcile.AuditCheckpoint{
		SchemaVersion: 1, Revision: 20577, CycleID: "mongo-old", Phase: catalogreconcile.AuditPhaseCompleted,
		UpdatedAt:     currentAt.Add(-time.Hour),
		LastCompleted: &catalogreconcile.CompletedAuditSnapshot{CycleID: "mongo-old", CompletedAt: sourceCompletedAt},
	}

	merged, changed := mergeImportedAuditCheckpoint(current, source)
	if !changed {
		t.Fatal("historical completed snapshot was not merged")
	}
	if merged.CycleID != current.CycleID || merged.Phase != current.Phase || merged.AfterAssessmentID != current.AfterAssessmentID {
		t.Fatalf("live MySQL cycle was replaced: %#v", merged)
	}
	if merged.Revision != current.Revision+1 {
		t.Fatalf("merged revision = %d, want %d", merged.Revision, current.Revision+1)
	}
	if merged.LastCompleted == nil || merged.LastCompleted.CycleID != source.LastCompleted.CycleID {
		t.Fatalf("historical completed snapshot was not preserved: %#v", merged.LastCompleted)
	}
}

func TestMergeImportedAuditCheckpointUsesNewerSourceWithLocalRevision(t *testing.T) {
	currentAt := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	current := catalogreconcile.AuditCheckpoint{Revision: 20, CycleID: "mysql-old", UpdatedAt: currentAt}
	source := catalogreconcile.AuditCheckpoint{Revision: 3, CycleID: "mongo-new", UpdatedAt: currentAt.Add(time.Minute)}

	merged, changed := mergeImportedAuditCheckpoint(current, source)
	if !changed || merged.CycleID != source.CycleID {
		t.Fatalf("newer source was not imported: changed=%t checkpoint=%#v", changed, merged)
	}
	if merged.Revision != current.Revision+1 {
		t.Fatalf("imported revision = %d, want local CAS revision %d", merged.Revision, current.Revision+1)
	}
}

func TestMergeImportedAuditCheckpointSkipsOlderEvidence(t *testing.T) {
	currentAt := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	current := catalogreconcile.AuditCheckpoint{
		Revision: 20, CycleID: "mysql-live", UpdatedAt: currentAt,
		LastCompleted: &catalogreconcile.CompletedAuditSnapshot{CycleID: "mysql-complete", CompletedAt: currentAt.Add(-time.Hour)},
	}
	source := catalogreconcile.AuditCheckpoint{
		Revision: 200, CycleID: "mongo-old", UpdatedAt: currentAt.Add(-time.Minute),
		LastCompleted: &catalogreconcile.CompletedAuditSnapshot{CycleID: "mongo-complete", CompletedAt: currentAt.Add(-2 * time.Hour)},
	}

	merged, changed := mergeImportedAuditCheckpoint(current, source)
	if changed || merged.CycleID != current.CycleID || merged.Revision != current.Revision {
		t.Fatalf("older evidence changed live checkpoint: changed=%t checkpoint=%#v", changed, merged)
	}
}

func newMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}
