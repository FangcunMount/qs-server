//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// Only the existing independently inspected loopback fixture may be used. It
// owns a new random database and its lifecycle. No shared schema/account or
// external qs-ai state is changed or invented by this candidate native test.
func aiHandoffNativeSetup(t *testing.T, which string) (*gorm.DB, *sql.DB) {
	t.Helper()
	db, pool := aiReverseNativeDatabase(t)
	aiReverseNativeGraph(t, pool)
	aiReverseNativeMappedSource(t, pool)
	aiNativeExec(t, pool, "UPDATE ai_messaging_admission SET closed=TRUE,revision=7 WHERE singleton=1")
	switch which {
	case "pending_business_with_original_receipt":
		// Actual accepted START binds the original session before the first
		// projection. Keep its exact receipt/inbox/STORED ACK and all budgets.
		aiNativeExec(t, pool, "DELETE FROM ai_bridge_events")
		aiNativeExec(t, pool, "DELETE FROM ai_messaging_inbox WHERE kind=?", int(pb.MessagingKind_INTERPRETATION_STATE))
		aiNativeExec(t, pool, "DELETE FROM ai_messaging_outbox WHERE message_id=?", "80000000-0000-4000-8000-000000000002")
		aiNativeExec(t, pool, "UPDATE ai_bridge_requests SET projection=NULL,version=0,status='pending'")
	case "pending_transport":
		// This is the existing real reverse fixture transition. It retains
		// current awaiting_receipt and NULL original command receipt as such.
		aiNativeExec(t, pool, "UPDATE ai_messaging_operations SET decision='',code='',receipt_id=NULL,receipt=NULL,decided_at=NULL")
		aiNativeExec(t, pool, "UPDATE ai_messaging_outbox SET stage='awaiting_receipt',confirmed_at=NULL WHERE message_id=?", aiFixtureRequestID)
		aiNativeExec(t, pool, "DELETE FROM ai_messaging_failures")
		aiNativeExec(t, pool, "DELETE FROM ai_messaging_inbox WHERE kind=?", int(pb.MessagingKind_COMMAND_RECEIPT))
		aiNativeExec(t, pool, "DELETE FROM ai_messaging_outbox WHERE message_id=?", "80000000-0000-4000-8000-000000000001")
	case "unmapped":
		aiNativeExec(t, pool, "DELETE FROM ai_messaging_legacy_commands")
	case "held":
		// Keep the valid STORED ACK body but hold its publisher. The actual
		// related held row must remain a blocker, never be made confirmed.
		aiNativeExec(t, pool, "UPDATE ai_messaging_outbox SET stage='held',confirmed_at=NULL,error_code='fixture_held' WHERE message_id=?", "80000000-0000-4000-8000-000000000001")
	case "unknown":
		// Use a real stored wire, with no trustworthy incoming owner binding.
		// A quarantine row cannot be hidden by organization or command JOINs.
		aiNativeExec(t, pool, "INSERT INTO ai_messaging_quarantine(wire_sha256,wire,code,attempts,first_seen_at,last_seen_at) SELECT wire_sha256,wire,'fixture_unknown_owner',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM ai_messaging_outbox WHERE message_id=?", aiFixtureRequestID)
	case "settled":
	default:
		t.Fatal("unsupported owned fixture case")
	}
	return db, pool
}

func aiHandoffNativePrepare(t *testing.T, db *gorm.DB, pool *sql.DB) (context.Context, *gorm.DB, *HistoricalCoordinator, *HistoricalSourcePage, *AIReverseSnapshot) {
	t.Helper()
	copies := aiReverseNativeCopies(t, pool)
	ctx, tx, sqlSnapshot := aiReverseNativeContext(t, db)
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error("owned original RRRO cleanup failed")
		}
	})
	reverse, err := PrepareAIReverseSnapshot(ctx, sqlSnapshot, 99, DefaultAIReverseLimits())
	if err != nil {
		t.Fatal("actual full14 RRRO failed", err)
	}
	c := aiReverseNativeBind(t, ctx, reverse, copies)
	// Reuse the actual two-source adapter preparation too; it observes original
	// admission/header/metadata without requiring a terminal projection.
	readonly, err := c.PrepareAIReadOnlyResolver(ctx, sqlSnapshot, 99, copies.inputs()[1:3])
	if err != nil || readonly.validate(ctx) != nil {
		t.Fatal("actual readonly source binding failed")
	}
	page, err := c.NextPage(ctx)
	if err != nil {
		t.Fatal("actual authenticated source page unavailable", err)
	}
	if !c.authenticated.complete || !reverse.Summary().WholeLedgerEOF || !reverse.Summary().ActualReadOnlyRR || reverse.scope == nil || reverse.scope.verifiedEntries != c.authenticated.entries {
		t.Fatal("actual original whole EOF producer missing")
	}
	return ctx, tx, c, page, reverse
}

func aiHandoffNativeRW(t *testing.T, db *gorm.DB) (context.Context, *gorm.DB) {
	t.Helper()
	tx := db.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatal("actual new host RW begin failed")
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error("owned host RW cleanup failed")
		}
	})
	return hostmysql.WithTx(t.Context(), tx), tx
}

// Fixed all14 physical columns preserve every actual nullable source, clock,
// budget, body, wire, receipt and ACK byte. Only the dedicated target metadata
// column is excluded and checked independently below. No bodies leave memory.
func aiHandoffNativeProtocolBaseline(t *testing.T, pool *sql.DB) string {
	t.Helper()
	r := &AILocalResolver{pool: pool}
	baseline := &aiLocalSnapshot{}
	for _, spec := range aiReverseSpecs {
		projection, order := aiCommandHandoffProjection(spec)
		rows, err := r.read(t.Context(), "SELECT "+projection+" FROM `"+spec.table+"` ORDER BY "+order+" LIMIT 1025")
		if err != nil {
			t.Fatal("independent current protocol read failed", err)
		}
		if spec.table == "ai_messaging_operations" {
			for _, row := range rows {
				if len(row) != 17 {
					t.Fatal("exact original operation layout changed")
				}
				row[15] = nil
			}
		}
		if err := baseline.add(spec.table, rows); err != nil {
			t.Fatal("native fixture baseline bound exceeded")
		}
	}
	return baseline.hash()
}

func aiHandoffNativeNoEvidence(t *testing.T, pool *sql.DB) {
	t.Helper()
	var count uint64
	if pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM ai_messaging_operations WHERE retirement_evidence IS NOT NULL OR retired=TRUE").Scan(&count) != nil || count != 0 {
		t.Fatal("refused handoff wrote/adopted retirement evidence")
	}
}

func TestAICommandHandoffNativePublicBatchCommitPreservesCurrentProtocol(t *testing.T) {
	for _, which := range []string{"pending_business_with_original_receipt", "pending_transport"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiHandoffNativeSetup(t, which)
			before := aiHandoffNativeProtocolBaseline(t, pool)
			ctx, roTx, c, page, reverse := aiHandoffNativePrepare(t, db, pool)
			batch, err := c.PrepareAICommandHandoffBatch(ctx, page, reverse)
			if err != nil || batch == nil {
				t.Fatal("actual original handoff prepare failed", err)
			}
			if page.consumed {
				t.Fatal("prepare consumed source page")
			}
			if which == "pending_business_with_original_receipt" {
				request := reverse.byTable["ai_bridge_requests"][aiFixtureRequestID]
				if request == nil || !request.observation.Unfinished || !request.projectionAbsent || reverse.byTable["ai_messaging_operations"][aiFixtureRequestID].receipt == "" {
					t.Fatal("fixture omitted actual pending business/original receipt")
				}
			} else if !reverse.byTable["ai_messaging_operations"][aiFixtureRequestID].observation.Unfinished || !reverse.byTable["ai_messaging_outbox"][aiFixtureRequestID].observation.Unfinished {
				t.Fatal("fixture omitted actual current pending transport")
			}
			aiReverseNativeEnd(t, roTx)
			writeCtx, rw := aiHandoffNativeRW(t, db)
			result, err := batch.Record(writeCtx, rw)
			if err != nil || result.Records != 1 || !result.HostCommitRequired || result.ProviderClosureVerified || result.RetirementAuthority || result.DropReady {
				t.Fatal("borrowed real handoff write failed or overstated authority", err)
			}
			var stillOpen int
			if rw.Statement.ConnPool.QueryRowContext(writeCtx, "SELECT 1").Scan(&stillOpen) != nil || stillOpen != 1 {
				t.Fatal("caller transaction was closed or committed by library")
			}
			if rw.Commit().Error != nil {
				t.Fatal("actual host handoff commit failed")
			}
			if after := aiHandoffNativeProtocolBaseline(t, pool); after != before {
				t.Fatal("current pending/budget/receipt/outbox or original source bytes changed")
			}
			var raw []byte
			var retired bool
			if pool.QueryRowContext(t.Context(), "SELECT CAST(retirement_evidence AS BINARY),retired FROM ai_messaging_operations WHERE command_id=?", aiFixtureRequestID).Scan(&raw, &retired) != nil {
				t.Fatal("independent committed evidence readback failed")
			}
			var actual store.CommandRetirementEvidence
			if json.Unmarshal(raw, &actual) != nil || retired || len(batch.evidence) != 1 || !reflect.DeepEqual(actual, batch.evidence[0]) || actual.CommandID != aiFixtureRequestID || actual.Conclusion != "transferred_verified" || actual.BusinessTerminal || actual.ResponsibilityClosed {
				t.Fatal("committed original identity/protocol handoff evidence differs or claims provider completion")
			}
			// An ended write scope cannot be automatically retried/adopted.
			if _, err := batch.Record(writeCtx, rw); err == nil {
				t.Fatal("consumed batch was silently reused")
			}
		})
	}
}

func TestAICommandHandoffNativeOriginalLedgerAndBusinessDriftRejectWithoutWrite(t *testing.T) {
	for _, which := range []string{"fourteen_ledger", "assessment_owner"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiHandoffNativeSetup(t, "settled")
			ctx, roTx, c, page, reverse := aiHandoffNativePrepare(t, db, pool)
			batch, err := c.PrepareAICommandHandoffBatch(ctx, page, reverse)
			if err != nil {
				t.Fatal("actual baseline handoff preparation failed", err)
			}
			aiReverseNativeEnd(t, roTx)
			switch which {
			case "fourteen_ledger":
				aiNativeExec(t, pool, "UPDATE ai_messaging_observations SET recorded_count=recorded_count+1 WHERE kind='duplicate_event'")
			case "assessment_owner":
				aiNativeExec(t, pool, "UPDATE assessment SET org_id=org_id+1")
			}
			before := aiHandoffNativeProtocolBaseline(t, pool)
			writeCtx, rw := aiHandoffNativeRW(t, db)
			result, err := batch.Record(writeCtx, rw)
			if !errors.Is(err, ErrAILocalChanged) || result.Records != 0 {
				t.Fatal("actual fresh ledger/owner drift was accepted", err)
			}
			if rw.Rollback().Error != nil {
				t.Fatal("actual refusal rollback failed")
			}
			aiHandoffNativeNoEvidence(t, pool)
			if after := aiHandoffNativeProtocolBaseline(t, pool); after != before {
				t.Fatal("refusal changed current protocol rows")
			}
		})
	}
}

func TestAICommandHandoffNativeUnmappedHeldAndUnknownNeverWrite(t *testing.T) {
	for _, which := range []string{"unmapped", "held", "unknown"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiHandoffNativeSetup(t, which)
			before := aiHandoffNativeProtocolBaseline(t, pool)
			ctx, roTx, c, page, reverse := aiHandoffNativePrepare(t, db, pool)
			batch, err := c.PrepareAICommandHandoffBatch(ctx, page, reverse)
			if err == nil || batch != nil {
				t.Fatal("actual unmapped/held/unknown state minted a write batch")
			}
			if which == "held" && !errors.Is(err, ErrAILocalResponsibility) {
				t.Fatal("actual held responsibility was not retained", err)
			}
			if which != "held" && !errors.Is(err, ErrAILocalUnknown) {
				t.Fatal("external/unowned execution was not unknown", err)
			}
			aiReverseNativeEnd(t, roTx)
			aiHandoffNativeNoEvidence(t, pool)
			if after := aiHandoffNativeProtocolBaseline(t, pool); after != before {
				t.Fatal("prepare refusal changed current protocol rows")
			}
		})
	}
}
