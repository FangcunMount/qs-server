//go:build integration && reliable_messaging_m4 && reliable_messaging_m5

package runtimeclosure

import (
	"database/sql"
	"net/http"
	"os"
	"testing"
	"time"

	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	qs04ReadyMarker  = "/tmp/m6-qs04-broker-kill-ready"
	qs04ResumeMarker = "/tmp/m6-qs04-broker-restarted"
)

// The companion script owns a disposable Broker. It kills NSQ only after the
// standard Outbox is published and before any Worker consumer connects.
func TestM6QS04RecoveredOriginalHasOneWorkerEffect(t *testing.T) {
	if os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" || os.Getenv("RM_QS04_COORDINATED") != "1" {
		t.Fatal("disposable coordinated NSQ and MySQL/Mongo/Redis are required")
	}
	for _, marker := range []string{qs04ReadyMarker, qs04ResumeMarker} {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("stale QS-04 coordinator marker %s: %v", marker, err)
		}
		t.Cleanup(func() { _ = os.Remove(marker) })
	}
	prepareQS04Channel(t)
	var delivery *standardClosureDelivery
	var originalEventID string
	var originalOrgID int64
	var recoveredQS04DB *gorm.DB
	var originalSubmittedAt time.Time
	scenario := runtimeClosureScenario{
		beforeEvaluation: func(t *testing.T, assessmentID uint64, db *gorm.DB, _ *eventsubsystem.Subsystem) {
			recoveredQS04DB = db
			sqlDB, err := db.DB()
			require.NoError(t, err)
			var orgID int64
			require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT org_id FROM assessment WHERE id=? AND status='submitted'`, assessmentID).Scan(&orgID))
			originalOrgID = orgID
			require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT event_id FROM qs_rm_evaluation_request_ref WHERE assessment_id=?`, assessmentID).Scan(&originalEventID))
			require.Eventually(t, func() bool {
				var state string
				var attempts int
				return sqlDB.QueryRowContext(t.Context(), `SELECT state,attempt_count FROM rm_outbox WHERE message_id=?`, originalEventID).
					Scan(&state, &attempts) == nil && state == "published" && attempts == 1
			}, 20*time.Second, 50*time.Millisecond)
			var runCount int64
			require.NoError(t, db.Table("runtime_checkpoint").Where("scope='evaluation_run' AND assessment_id=?", assessmentID).Count(&runCount).Error)
			require.Zero(t, runCount)
			require.NoError(t, os.WriteFile(qs04ReadyMarker, []byte(originalEventID), 0600))
			waitQS04BrokerRestart(t)
			// Only the disposable Assessment is aged beyond the review cutoff.
			require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT submitted_at FROM assessment WHERE id=?`, assessmentID).Scan(&originalSubmittedAt))
			require.NoError(t, db.Exec(`UPDATE assessment SET submitted_at=DATE_SUB(NOW(), INTERVAL 20 MINUTE) WHERE id=? AND status='submitted'`, assessmentID).Error)
			cutoff := time.Now().Add(-10 * time.Minute)
			inspection, err := mysqlstandard.InspectOriginalRequest(t.Context(), sqlDB, orgID, assessmentID, cutoff)
			require.NoError(t, err)
			require.Equal(t, "candidate_never_claimed", inspection.State)
			require.Equal(t, originalEventID, inspection.EventID)
			var version uint64
			require.NoError(t, sqlDB.QueryRowContext(t.Context(), `SELECT version FROM rm_outbox WHERE message_id=? AND state='published'`, originalEventID).Scan(&version))
			ledger, err := mysqlstandard.NewGapRecoveryLedger(sqlDB)
			require.NoError(t, err)
			decision, err := ledger.Authorize(t.Context(), mysqlstandard.GapRecoveryRequest{
				OrgID: orgID, ActorID: 99, RequestID: "m6-qs04-full-worker-effect",
				AssessmentID: assessmentID, EventID: originalEventID, ExpectedVersion: version,
				Reason: "disposable broker lost confirmed unclaimed message", SubmittedBefore: cutoff,
			})
			require.NoError(t, err)
			require.True(t, decision.Authorized)
			require.NotNil(t, delivery)
			require.NoError(t, delivery.Connect())
		},
		afterReport: func(t *testing.T, _ runtimeClosureDelivery, _ grpctransport.Deps, _, assessmentID uint64) {
			// The common closure already checks the real Worker, model result,
			// single Run/Outcome, report, duplicate delivery and report reads.
			// Here the original Outbox identity and second Relay attempt join it
			// to the earlier controlled Broker loss.
			db := recoveredQS04DB
			var eventID, state string
			var attempts int
			var version uint64
			require.NoError(t, db.Raw(`SELECT message_id,state,attempt_count,version FROM rm_outbox WHERE message_id=?`, originalEventID).
				Row().Scan(&eventID, &state, &attempts, &version))
			require.Equal(t, originalEventID, eventID)
			require.Equal(t, "published", state)
			require.Equal(t, 2, attempts)
			assertRowCount(t, db, "runtime_checkpoint", "scope='evaluation_run' AND assessment_id=?", 1, assessmentID)
			assertRowCount(t, db, "evaluation_outcome", "assessment_id=?", 1, assessmentID)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			ledger, err := mysqlstandard.NewGapRecoveryLedger(sqlDB)
			require.NoError(t, err)
			denied, err := ledger.Authorize(t.Context(), mysqlstandard.GapRecoveryRequest{
				OrgID: originalOrgID, ActorID: 99, RequestID: "m6-qs04-after-worker-claim-denied",
				AssessmentID: assessmentID, EventID: originalEventID, ExpectedVersion: version,
				Reason: "verify already claimed original cannot be resent", SubmittedBefore: time.Now().Add(-10 * time.Minute),
			})
			require.NoError(t, err)
			require.False(t, denied.Authorized)
			require.Equal(t, "ever_claimed", denied.Code)
			var unchangedState string
			var unchangedAttempts int
			var unchangedVersion uint64
			require.NoError(t, db.Raw(`SELECT state,attempt_count,version FROM rm_outbox WHERE message_id=?`, originalEventID).
				Row().Scan(&unchangedState, &unchangedAttempts, &unchangedVersion))
			require.Equal(t, "published", unchangedState)
			require.Equal(t, attempts, unchangedAttempts)
			require.Equal(t, version, unchangedVersion)
			reviewSummary, err := ledger.ReadSummary(t.Context(), originalOrgID)
			require.NoError(t, err)
			require.Equal(t, mysqlstandard.GapRecoverySummary{Authorized: 1, Denied: 1, WaitingRelay: 0}, reviewSummary)
			// Restore this disposable fixture before the common closure checks
			// that all business dates belong to the current runtime window.
			require.NoError(t, db.Exec(`UPDATE assessment SET submitted_at=? WHERE id=?`, originalSubmittedAt, assessmentID).Error)
			t.Logf("QS-04 original event=%s relay_attempts=%d Run=1 Outcome=1 later_recovery=%s", eventID, attempts, denied.Code)
		},
	}
	runCurrentRuntimeClosure(t, func(t *testing.T, opts eventsubsystem.Options, sqlDB *sql.DB) (*eventsubsystem.Subsystem, runtimeClosureDelivery, error) {
		subsystem, d, err := newM5StandardEventSubsystemControlled(t, opts, sqlDB, true, false, true)
		if err == nil {
			delivery = d.(*standardClosureDelivery)
		}
		return subsystem, d, err
	}, scenario)
}

func prepareQS04Channel(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	for _, path := range []string{
		"/topic/create?topic=qs.evaluation.lifecycle",
		"/channel/create?topic=qs.evaluation.lifecycle&channel=rm-m6-qs04-business-recovery",
	} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://nsqd:4151"+path, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

func waitQS04BrokerRestart(t *testing.T) {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		if _, err := os.Stat(qs04ResumeMarker); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		select {
		case <-deadline:
			t.Fatal("host did not confirm the disposable broker restart")
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
