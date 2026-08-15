package mongoconsistency

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCheckpointPORoundTrip(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 15, 12, 0, 0, 123000000, time.UTC)
	checkpoint := appaudit.Checkpoint{
		SchemaVersion: appaudit.CheckpointSchemaVersion, Revision: 7, CycleID: "cycle-1",
		Phase: appaudit.PhaseGeneratedTerminal, Cursor: 22, UpperBound: 90,
		Working: appaudit.Statistics{
			Scanned:  33,
			Findings: map[string]int64{appaudit.DriftGeneratedMissingArtifact: 2},
			Samples:  map[string][]string{appaudit.DriftGeneratedMissingArtifact: {"11", "12"}},
		},
		LastCompleted: &appaudit.CompletedCycle{
			CycleID: "cycle-0", CompletedAt: at.Add(-24 * time.Hour),
			Statistics: appaudit.Statistics{Scanned: 10, Findings: map[string]int64{}, Samples: map[string][]string{}},
		},
		NextCycleAt: at.Add(24 * time.Hour), UpdatedAt: at,
	}
	po, err := toPO(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := fromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.Revision != 7 || roundTrip.Cursor != 22 || roundTrip.UpperBound != 90 || roundTrip.Working.Findings[appaudit.DriftGeneratedMissingArtifact] != 2 {
		t.Fatalf("round trip = %#v", roundTrip)
	}
	if roundTrip.LastCompleted == nil || roundTrip.LastCompleted.CycleID != "cycle-0" || !roundTrip.NextCycleAt.Equal(checkpoint.NextCycleAt) {
		t.Fatalf("completed cycle = %#v", roundTrip.LastCompleted)
	}
}

func TestCheckpointPODecodePreservesScannedCountWhenLegacyMapsAreNull(t *testing.T) {
	checkpoint, err := fromPO(checkpointPO{
		SchemaVersion:  appaudit.CheckpointSchemaVersion,
		Revision:       1,
		CycleID:        "legacy-cycle",
		Phase:          string(appaudit.PhaseAnswerSheetOutbox),
		StatisticsJSON: `{"scanned":17,"findings":null,"samples":null}`,
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Working.Scanned != 17 || checkpoint.Working.Findings == nil || checkpoint.Working.Samples == nil {
		t.Fatalf("decoded working statistics = %#v", checkpoint.Working)
	}
}

func TestCheckpointSaveRejectsStaleRevision(t *testing.T) {
	t.Parallel()
	db, mock := newMongoConsistencyMockGorm(t)
	at := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE \x60mongo_consistency_audit_checkpoint\x60 SET")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	err := NewCheckpointRepository(db).Save(context.Background(), 1, appaudit.Checkpoint{
		SchemaVersion: appaudit.CheckpointSchemaVersion, Revision: 2, CycleID: "cycle-1",
		Phase: appaudit.PhaseAnswerSheetOutbox, Working: appaudit.NewStatistics(), UpdatedAt: at,
	})
	if !errors.Is(err, appaudit.ErrCheckpointCAS) {
		t.Fatalf("save error = %v, want checkpoint CAS", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newMongoConsistencyMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
