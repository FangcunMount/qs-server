//go:build integration

package aibridge

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func observationCount(t *testing.T, db *sql.DB, kind string) uint64 {
	t.Helper()
	var count uint64
	mustMQ(t, db.QueryRow("SELECT recorded_count FROM ai_messaging_observations WHERE kind=?", kind).Scan(&count))
	return count
}

func TestMQPayloadObservationCommitsOnlyTechnicalFactAndPreservesReadTransaction(t *testing.T) {
	f := newMQFixture(t)
	ctx := context.Background()
	kind := "payload_serve_reference_mismatch"
	baseline := observationCount(t, f.db, kind)
	tx, err := f.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	mustMQ(t, err)
	reader := &MessagingReader{DB: f.db}
	mustMQ(t, reader.ObserveMessagePayloadFailure(ctx, kind))
	var one int
	mustMQ(t, tx.QueryRow("SELECT 1").Scan(&one))
	mustMQ(t, tx.Rollback())
	if observationCount(t, f.db, kind) != baseline+1 {
		t.Fatal("technical fact lost with read rollback")
	}
	var identities int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_outbox WHERE aggregate_key=?", f.request.RequestID).Scan(&identities))
	if identities != 0 {
		t.Fatal("payload error created business message")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if reader.ObserveMessagePayloadFailure(cancelled, kind) == nil || observationCount(t, f.db, kind) != baseline+1 {
		t.Fatal("cancelled observation committed")
	}
	if reader.ObserveMessagePayloadFailure(ctx, "untrusted-body") == nil || reader.ObserveMessagePayloadFailure(ctx, "duplicate_event") == nil {
		t.Fatal("untrusted category or duplicate bypass accepted")
	}
	mustMQ(t, f.db.Ping())
}

func TestMQTechnicalRecordingCoverageAndSchemaFailureAreExplicit(t *testing.T) {
	f := newMQFixture(t)
	values, err := MessagingSnapshot(t.Context(), f.db)
	mustMQ(t, err)
	if values["duplicate_observations_available"] != 1 || values["payload_error_observations_available"] != 1 || values["observations_history_complete"] != 0 || values["observations_recording_since_epoch_seconds"] <= 0 {
		t.Fatal("recorded facts misrepresented as complete history", values)
	}
	for _, kind := range observationKinds {
		if _, ok := values["recorded_"+kind]; !ok {
			t.Fatal("seeded kind unavailable", kind)
		}
	}
	tx, err := f.db.BeginTx(t.Context(), nil)
	mustMQ(t, err)
	defer tx.Rollback()
	// Only this uncommitted test change is visible to startup validation.
	_, err = tx.Exec("DELETE FROM ai_messaging_observations WHERE kind='duplicate_event'")
	mustMQ(t, err)
	if err = RequireMessagingObservations(t.Context(), tx); err == nil || strings.Contains(err.Error(), "SELECT") {
		t.Fatal("partial coverage allowed startup or leaked storage", err)
	}
	missing, err := collectTechnicalObservations(t.Context(), tx)
	mustMQ(t, err)
	if len(missing) != 2 || missing["duplicate_observations_available"] != 0 || missing["payload_error_observations_available"] != 0 {
		t.Fatal("partial ledger fabricated zero counts", missing)
	}
	mustMQ(t, tx.Rollback())
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return RequireMessagingObservations(t.Context(), tx) }))
	mustMQ(t, f.db.Ping())
}
