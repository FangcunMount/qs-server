//go:build integration

package retirement

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"gorm.io/gorm"
)

// This tests the existing borrowed store contract, not the new opaque external
// qualification factory or the production write pipeline. The unmapped proof is
// an explicitly synthetic storage-contract input; the complete four-copy local
// auth below authenticates original bytes, NOT qs-ai/provider business closure.
// A real AIExternalExecutionQualification is deliberately not manufactured.
// The only database is the existing protected fixture's new random SQL99 schema.
type aiMixedStorageNativeBaseline struct {
	snapshot  *AIReverseSnapshot
	operation aiReverseRow
}

func aiMixedStorageNativePrepare(t *testing.T) (*gorm.DB, *sql.DB, *gorm.DB, *aiMixedStorageNativeBaseline, *AICommandHandoffBatch, store.CommandRetirementEvidence) {
	t.Helper()
	db, pool := aiHandoffNativeSetup(t, "pending_transport")
	f := aiLocalGraph(t)
	change := app.Change{CommandID: aiFixtureCommandID, SessionID: f.projection.SessionID, Actor: f.request.Actor, Action: "cancel", ExpectedVersion: int64(f.projection.Version)}
	payload, err := json.Marshal(change)
	if err != nil {
		t.Fatal("synthetic historical change encoding failed")
	}
	insert, err := pool.ExecContext(t.Context(), "INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash,delivered,attempts,available_at) VALUES(?,?,'cancel',?, ?,TRUE,3,'2026-10-08 11:12:13.123456')", change.CommandID, f.request.RequestID, payload, sourceSHA(payload))
	if err != nil {
		t.Fatal("actual original unmapped source insert failed")
	}
	if n, err := insert.RowsAffected(); err != nil || n != 1 {
		t.Fatal("unmapped source fixture did not insert one physical row")
	}
	copies := aiReverseNativeCopies(t, pool)
	ctx, originalTx, sqlSnapshot := aiReverseNativeContext(t, db)
	t.Cleanup(func() {
		if err := originalTx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error("owned original RRRO cleanup failed")
		}
	})
	reverse, err := PrepareAIReverseSnapshot(ctx, sqlSnapshot, 99, DefaultAIReverseLimits())
	if err != nil {
		t.Fatal("actual complete full14 original snapshot failed", err)
	}
	limits := DefaultHistoricalCoordinatorLimits()
	// A mapped command consumes both its physical source references. No source
	// row is sliced out to make a mixed page look like an unmapped closed page.
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), copies.inputs(), limits)
	if err != nil || c.BindAIReverseSourceScope(ctx, reverse, copies.inputs()) != nil {
		t.Fatal("actual complete four-source local authentication failed")
	}
	if !c.authenticated.complete || c.authenticated.entries != 3 || c.authenticated.receipts[0].Records != 0 || c.authenticated.receipts[1].Records != 2 || c.authenticated.receipts[2].Records != 1 || c.authenticated.receipts[3].Records != 0 || !reverse.Summary().WholeLedgerEOF || !reverse.Summary().ActualReadOnlyRR {
		t.Fatal("fixture failed actual complete four-copy/full14 bounds")
	}
	page, err := c.NextPage(ctx)
	if err != nil || len(page.rows) != 1 || page.rows[0].legacy == nil || len(page.rows[0].keys) != 2 {
		t.Fatal("first actual page did not retain both mapped source references")
	}
	mapped, err := c.PrepareAICommandHandoffBatch(ctx, page, reverse)
	if err != nil || len(mapped.evidence) != 1 || mapped.evidence[0].CommandID != aiFixtureRequestID {
		t.Fatal("actual mapped source factory failed", err)
	}
	if page.consumed || c.coverage {
		t.Fatal("local storage fixture improperly claimed complete qualified page coverage")
	}
	unmapped := reverse.byTable[AIBridgeCommandSource][change.CommandID]
	if unmapped == nil || !unmapped.delivered || reverse.byTable[AILegacyCommandSource][change.CommandID] != nil || reverse.byTable["ai_messaging_operations"][change.CommandID] != nil || reverse.byTable["ai_messaging_outbox"][change.CommandID] != nil {
		t.Fatal("fixture did not retain an actual unmapped original ID")
	}
	reader, err := NewAISQLSourceReader(bytes.NewReader(copies.raw[1]), copies.expected[1])
	if err != nil {
		t.Fatal("actual source reader unavailable")
	}
	var original *DecodedAICommand
	for {
		value, readErr := reader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.Fatal("actual original source frame rejected", readErr)
		}
		bound, bindErr := c.authenticated.BindAICommand(value)
		if bindErr != nil {
			t.Fatal("actual original source handle rejected")
		}
		facts, factsErr := bound.Facts()
		if factsErr != nil {
			t.Fatal("actual original source facts rejected")
		}
		if facts.CommandID == change.CommandID {
			original = facts
		}
	}
	if original == nil || reader.Receipt() != c.authenticated.receipts[1] || original.RequestID != f.request.RequestID || original.SourceKind != "cancel" || original.ResourceID != f.projection.SessionID {
		t.Fatal("original physical source identity/full EOF differed")
	}
	// This intentionally exercises ONLY store source-CAS and immutable retired
	// ID placement. The fixture's local cancelled projection does not replace
	// the unavailable external53 qualified producer or mint a page capability.
	e := store.CommandRetirementEvidence{Version: 1, OperationID: c.binding.OperationID, VerifierVersion: "qs-storage-contract-fixture/v1", VerificationMethod: "source_identity_hash_and_business_closure", VerifiedAt: time.Now().UTC(), AdmissionRevision: mapped.binding.AdmissionRevision, CommandID: original.CommandID, RequestID: original.RequestID, SourceKind: original.SourceKind, OrganizationID: original.OrganizationID, SubjectID: original.SubjectID, ResourceID: original.ResourceID, Sources: []store.CommandRetirementSource{{Table: AIBridgeCommandSource, CommandID: original.CommandID, BytesKind: original.PayloadBytesDigest.Kind, BytesSHA256: original.PayloadBytesDigest.SHA256, BusinessPayloadHash: original.WriterPayloadDigest.SHA256}}, References: []store.CommandRetirementReference{{Kind: "business_record", ID: original.RequestID}}, Conclusion: "unverifiable", Reason: "history_terminal_evidence_gap", OwnershipVerified: true, ResponsibilityClosed: true, BusinessTerminal: true}
	spec := aiReverseSpecByTable("ai_messaging_operations")
	projection, _ := aiCommandHandoffProjection(spec)
	rawRows, rawColumns, _, readErr := reverse.read(ctx, "SELECT "+projection+" FROM ai_messaging_operations WHERE command_id=?", 2, aiFixtureRequestID)
	if readErr != nil || len(rawRows) != 1 || !reflect.DeepEqual(rawColumns, spec.columns) {
		t.Fatal("actual original raw operation baseline unavailable")
	}
	baseline := &aiMixedStorageNativeBaseline{snapshot: reverse, operation: rawRows[0]}
	return db, pool, originalTx, baseline, mapped, e
}

func aiMixedStorageNativeReadback(t *testing.T, db *gorm.DB, pool *sql.DB, before *aiMixedStorageNativeBaseline, mapped *AICommandHandoffBatch, retired store.CommandRetirementEvidence, committed bool) {
	t.Helper()
	current, ctx, third := aiReverseNativePrepare(t, db)
	defer aiReverseNativeEnd(t, third)
	if current.pool == before.snapshot.pool || current.pool == mapped.oldPool || !current.Summary().WholeLedgerEOF || !current.Summary().ActualReadOnlyRR || current.report.DatabaseIdentitySHA256 != before.snapshot.report.DatabaseIdentitySHA256 || current.anchorMetadataSHA != before.snapshot.anchorMetadataSHA || !reflect.DeepEqual(current.anchors, before.snapshot.anchors) {
		t.Fatal("independent third actual RRRO identity/owner baseline differed")
	}
	for i, ledger := range before.snapshot.report.Ledgers {
		if ledger.Store == "ai_messaging_operations" {
			continue
		}
		now := current.report.Ledgers[i]
		now.Pages = ledger.Pages
		if now != ledger {
			t.Fatal("borrowed mixed write changed a non-operation ledger", ledger.Store)
		}
	}
	spec := aiReverseSpecByTable("ai_messaging_operations")
	projection, _ := aiCommandHandoffProjection(spec)
	probe := &AIReverseSnapshot{pool: current.pool, started: current.started, limits: current.limits}
	for _, id := range []string{aiFixtureRequestID, retired.CommandID} {
		rows, columns, _, err := probe.read(ctx, "SELECT "+projection+" FROM ai_messaging_operations WHERE command_id=?", 2, id)
		if err != nil || !reflect.DeepEqual(columns, spec.columns) {
			t.Fatal("actual third raw operation read failed")
		}
		if id == retired.CommandID {
			if !committed {
				if len(rows) != 0 {
					t.Fatal("unmapped retirement escaped host rollback")
				}
				continue
			}
			if len(rows) != 1 || aiCommandRetirementOperationExpected(columns, rows[0], retired) != nil {
				t.Fatal("committed original ID/null/body-free retirement differs")
			}
			var live uint64
			if current.pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_outbox WHERE message_id=?", id).Scan(&live) != nil || live != 0 {
				t.Fatal("unmapped historical ID acquired a current MQ message")
			}
			continue
		}
		if len(rows) != 1 {
			t.Fatal("original mapped operation was removed or duplicated")
		}
		for _, column := range columns {
			if committed && column == "retirement_evidence" {
				continue
			}
			left := before.operation[column]
			right := rows[0][column]
			if (left == nil) != (right == nil) || !bytes.Equal(left, right) {
				t.Fatal("original pending operation/budget/receipt bytes changed", column)
			}
		}
		if committed && aiCommandHandoffOperationUnchanged(columns, before.operation, rows[0], mapped.evidence[0]) != nil {
			t.Fatal("actual mapped evidence falsely retired the live command")
		}
	}
	if current.ValidateBorrowedSnapshot(ctx) != nil || pool.PingContext(t.Context()) != nil {
		t.Fatal("library closed a borrowed transaction or pool")
	}
}

func TestAICommandPersistenceNativeStoreMixedBorrowedCommitAndRollback(t *testing.T) {
	for _, finish := range []string{"commit", "rollback"} {
		t.Run(finish, func(t *testing.T) {
			db, pool, originalTx, before, mapped, retired := aiMixedStorageNativePrepare(t)
			aiReverseNativeEnd(t, originalTx)
			ctx, write := aiHandoffNativeRW(t, db)
			result, err := mapped.Record(ctx, write)
			if err != nil || result.Records != 1 || !result.HostCommitRequired || result.ProviderClosureVerified || result.RetirementAuthority || result.DropReady {
				t.Fatal("actual mapped borrowed recording failed or claimed completion", err)
			}
			if err := store.RecordOperationRetirement(ctx, write, retired); err != nil {
				t.Fatal("actual unmapped source-CAS placement failed", err)
			}
			var active int
			if write.Statement.ConnPool.QueryRowContext(ctx, "SELECT 1").Scan(&active) != nil || active != 1 {
				t.Fatal("library ended the borrowed write epoch")
			}
			var attempts, delivered uint64
			var stage string
			if write.Raw("SELECT attempts,stage FROM ai_messaging_outbox WHERE message_id=?", aiFixtureRequestID).Row().Scan(&attempts, &stage) != nil || attempts != 3 || stage != "awaiting_receipt" || write.Raw("SELECT delivered FROM ai_bridge_commands WHERE command_id=?", aiFixtureRequestID).Row().Scan(&delivered) != nil || delivered != 0 {
				t.Fatal("current pending inherited budget or old delivery flag changed")
			}
			if finish == "commit" {
				if write.Commit().Error != nil {
					t.Fatal("actual host mixed commit failed")
				}
			} else if write.Rollback().Error != nil {
				t.Fatal("actual host mixed rollback failed")
			}
			aiMixedStorageNativeReadback(t, db, pool, before, mapped, retired, finish == "commit")
		})
	}
}

func TestAICommandPersistenceNativeStoreMixedSourceConflictRollsBackEarlierWrite(t *testing.T) {
	db, pool, originalTx, before, mapped, retired := aiMixedStorageNativePrepare(t)
	aiReverseNativeEnd(t, originalTx)
	ctx, write := aiHandoffNativeRW(t, db)
	if _, err := mapped.Record(ctx, write); err != nil {
		t.Fatal("actual mapped first statement failed", err)
	}
	changed := app.Change{CommandID: retired.CommandID, SessionID: retired.ResourceID, Actor: app.Actor{OrgID: retired.OrganizationID, SubjectID: retired.SubjectID}, Action: "cancel", ExpectedVersion: 3}
	raw, err := json.Marshal(changed)
	if err != nil {
		t.Fatal("source conflict fixture encoding failed")
	}
	mutation := write.Exec("UPDATE ai_bridge_commands SET payload=? WHERE command_id=?", raw, retired.CommandID)
	if mutation.Error != nil || mutation.RowsAffected != 1 {
		t.Fatal("actual source drift did not affect the original row")
	}
	if err := store.RecordOperationRetirement(ctx, write, retired); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed physical source bytes were adopted", err)
	}
	if write.Rollback().Error != nil {
		t.Fatal("host rollback of partial mixed statements failed")
	}
	aiMixedStorageNativeReadback(t, db, pool, before, mapped, retired, false)
}
