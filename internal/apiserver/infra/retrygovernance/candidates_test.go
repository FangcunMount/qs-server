package retrygovernance

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCandidateCursorRoundTripAndBounds(t *testing.T) {
	cursor := encodeCandidateCursor(125)
	offset, err := decodeCandidateCursor(cursor)
	if err != nil || offset != 125 {
		t.Fatalf("decode cursor = %d, %v; want 125, nil", offset, err)
	}
	for _, invalid := range []string{"%%%", encodeCandidateCursor(maxCandidateOffset + 1)} {
		if _, err := decodeCandidateCursor(invalid); err == nil {
			t.Fatalf("decodeCandidateCursor(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestMySQLOutboxCandidatesExplainAutomaticAndManualSummary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 19, 3, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("retry_disposition IN ('automatic','manual_required')")).
		WithArgs(int64(7), 10).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "attempt_count", "disposition", "next_attempt_at", "last_error_kind", "updated_at"}).
			AddRow("automatic-event", 2, "automatic", now, "temporary", now).
			AddRow("manual-event", 30, "manual_required", nil, "temporary", now))
	reader := &Reader{mysql: gormDB}
	var items []app.RetryCandidate
	if err := reader.appendMySQLOutboxCandidates(t.Context(), 7, 10, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Disposition != "automatic" || items[1].Disposition != "manual_required" {
		t.Fatalf("candidates = %#v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
