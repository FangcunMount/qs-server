//go:build integration

package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	mysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The owned fixture helper verifies the exact local CID/image/labels/volume and
// 34306 loopback binding before opening only its new random database. No global
// instrumentation, accounts, containers, or shared schemas are changed here.
func aiReverseNativeDatabase(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	pool := aiNativeOwnedMySQL(t)
	var name string
	if pool.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&name) != nil || !strings.HasPrefix(name, "qs_ai_resolver_") {
		t.Fatal("random owned namespace binding")
	}
	version, _, e := migration.NewMigrator(pool, &migration.Config{Enabled: true, Database: name}).Run()
	if e != nil || version != 99 {
		t.Fatal("actual complete additive migration99 required")
	}
	db, e := gorm.Open(gormmysql.New(gormmysql.Config{Conn: pool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("owned GORM adapter")
	}
	return db, pool
}
func aiReverseNativeContext(t *testing.T, db *gorm.DB) (context.Context, *gorm.DB, *SQLResponsibilitySnapshot) {
	t.Helper()
	tx := db.Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatal("actual host read-only RR begin")
	}
	ctx := hostmysql.WithTx(t.Context(), tx)
	var id struct{ Server, DB string }
	if tx.Raw("SELECT @@server_uuid AS server,DATABASE() AS db").Scan(&id).Error != nil {
		t.Fatal("actual database identity")
	}
	cycle, e := PrepareSQLResponsibilitySnapshot(ctx, aiReverseHash("mysql_database_identity_v1", id.Server, id.DB), sqlevaluation.DefaultSQLResponsibilityLimits())
	if e != nil {
		if tx.Rollback().Error != nil {
			t.Error("host rollback")
		}
		t.Fatal("actual read-only snapshot unavailable", e)
	}
	return ctx, tx, cycle
}
func aiReverseNativePrepare(t *testing.T, db *gorm.DB) (*AIReverseSnapshot, context.Context, *gorm.DB) {
	t.Helper()
	ctx, tx, cycle := aiReverseNativeContext(t, db)
	s, e := PrepareAIReverseSnapshot(ctx, cycle, 99, DefaultAIReverseLimits())
	if e != nil {
		if tx.Rollback().Error != nil {
			t.Error("host rollback")
		}
		t.Fatal("native reverse scan", e)
	}
	return s, ctx, tx
}
func aiReverseNativeGraph(t *testing.T, pool *sql.DB) {
	t.Helper()
	f := aiLocalGraph(t)
	raw, e := json.Marshal(f.request)
	if e != nil {
		t.Fatal("synthetic request")
	}
	projection, e := json.Marshal(f.projection)
	if e != nil {
		t.Fatal("synthetic projection")
	}
	aiNativeExec(t, pool, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload,session_id,version,status,projection,organization_id,subject_id,testee_id) VALUES(?,?,?,?,2,'cancelled',?,?,?,?)", f.request.RequestID, sourceSHA(raw), raw, f.projection.SessionID, projection, f.request.Actor.OrgID, f.request.Actor.SubjectID, f.request.TesteeID)
	for i, id := range f.request.AssessmentIDs {
		sheet := strconv.Itoa(1000 + i)
		aiNativeExec(t, pool, "INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,created_at,updated_at,version) VALUES(?,?,?,'fixture','1',?,'adhoc','evaluated',UTC_TIMESTAMP(),UTC_TIMESTAMP(),1)", id, f.request.Actor.OrgID, f.request.TesteeID, sheet)
		aiNativeExec(t, pool, "INSERT INTO ai_bridge_request_assessments(request_id,assessment_id) VALUES(?,?)", f.request.RequestID, id)
	}
	for table, rows := range map[string][][][]byte{"ai_messaging_operations": f.ops, "ai_messaging_outbox": f.boxes, "ai_messaging_inbox": f.inbox, "ai_messaging_failures": f.failures, "ai_bridge_events": f.events} {
		for _, row := range rows {
			aiNativeInsert(t, pool, table, row)
		}
	}
	aiNativeExec(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", f.request.RequestID)
}
func aiReverseNativeCopies(t *testing.T, pool *sql.DB) authFixture {
	t.Helper()
	f := coordinatorNativeFixtureCopies(t, nil, nil)
	binding := aiNativeBinding(t, pool)
	for i, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		spec := aiReverseSpecByTable(table)
		var projected []string
		for _, col := range spec.columns {
			projected = append(projected, "CAST(`"+col+"` AS BINARY)")
		}
		rows, e := pool.QueryContext(t.Context(), "SELECT "+strings.Join(projected, ",")+" FROM `"+table+"` ORDER BY command_id")
		if e != nil {
			t.Fatal("source actual scan")
		}
		var rawRows [][][]byte
		for rows.Next() {
			raw := make([]sql.RawBytes, len(spec.columns))
			dst := make([]any, len(raw))
			for j := range raw {
				dst[j] = &raw[j]
			}
			if rows.Scan(dst...) != nil {
				t.Fatal("actual source row")
			}
			row := make([][]byte, len(raw))
			for j, cell := range raw {
				if cell != nil {
					row[j] = append([]byte{}, cell...)
				}
			}
			rawRows = append(rawRows, row)
		}
		if rows.Err() != nil || rows.Close() != nil {
			t.Fatal("source actual EOF")
		}
		boundary, columns := binding.BridgeBoundary, binding.BridgeColumns
		if i == 1 {
			boundary, columns = binding.LegacyBoundary, binding.LegacyColumns
		}
		f.raw[i+1], f.expected[i+1] = coordinatorNativeAICopy(t, table, rawRows, columns, boundary)
	}
	return f
}
func aiReverseNativeBind(t *testing.T, ctx context.Context, s *AIReverseSnapshot, f authFixture) *HistoricalCoordinator {
	t.Helper()
	c, e := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if e != nil {
		t.Fatal("full four-copy native fixture auth")
	}
	if e = c.BindAIReverseSourceScope(ctx, s, f.inputs()); e != nil {
		t.Fatal("four-copy scope auth", e)
	}
	return c
}
func aiReverseNativeEnd(t *testing.T, tx *gorm.DB) {
	t.Helper()
	if tx.Rollback().Error != nil {
		t.Fatal("host-ended borrowed transaction")
	}
}

func TestAIReverseNativeOriginalEventPKAndRequestVersions(t *testing.T) {
	for _, which := range []string{"two_versions", "physical_id_mismatch", "request_version_unique"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiReverseNativeDatabase(t)
			aiReverseNativeGraph(t, pool)
			f := aiLocalGraph(t)
			prior := aiReversePriorEvent(t, f)
			if which == "physical_id_mismatch" {
				prior.eventRow[0] = []byte("50000000-0000-4000-8000-000000000099")
			}
			wantRows := 2
			if which == "request_version_unique" {
				// Migration72 protects request+version separately from event_id.
				// The real database must reject a different PK at current version2.
				_, e := pool.ExecContext(t.Context(), "INSERT INTO ai_bridge_events(event_id,request_id,version,payload_hash) VALUES(?,?,2,?)", prior.eventRow[0], prior.eventRow[1], prior.eventRow[3])
				var duplicate *mysql.MySQLError
				if !errors.As(e, &duplicate) || duplicate.Number != 1062 {
					t.Fatal("actual request+version unique constraint did not reject")
				}
				wantRows = 1
			} else {
				aiNativeInsert(t, pool, "ai_bridge_events", prior.eventRow)
				aiNativeInsert(t, pool, "ai_messaging_inbox", prior.inbox)
				aiNativeInsert(t, pool, "ai_messaging_outbox", prior.ackOutbox)
			}
			s, ctx, tx := aiReverseNativePrepare(t, db)
			defer aiReverseNativeEnd(t, tx)
			aiReverseNativeBind(t, ctx, s, aiReverseNativeCopies(t, pool))
			var originals []struct {
				EventID   string `gorm:"column:event_id"`
				RequestID string `gorm:"column:request_id"`
				Version   uint64 `gorm:"column:version"`
			}
			if tx.Raw("SELECT event_id,request_id,version FROM ai_bridge_events ORDER BY version").Scan(&originals).Error != nil || len(originals) != wantRows || len(s.byTable["ai_bridge_events"]) != wantRows {
				t.Fatal("actual native event PK rows merged or omitted")
			}
			seen := map[string]bool{}
			for _, row := range originals {
				n := s.byTable["ai_bridge_events"][row.EventID]
				if row.EventID == row.RequestID || row.RequestID != f.request.RequestID || seen[row.EventID] || n == nil || n.id != row.EventID || n.request != row.RequestID || n.version != row.Version || n.org != f.request.Actor.OrgID || n.subject != f.request.Actor.SubjectID || n.testee != f.request.TesteeID || n.resource != f.projection.SessionID {
					t.Fatal("native original event identity or owner changed")
				}
				seen[row.EventID] = true
			}
			if s.byTable["ai_bridge_events"][f.request.RequestID] != nil || !seen[f.projection.EventID] {
				t.Fatal("original event substituted with request or current PK lost")
			}
			if wantRows == 2 && (!seen[string(prior.eventRow[0])] || originals[0].Version != 1 || originals[1].Version != 2) {
				t.Fatal("different original versions not preserved")
			}
			var current struct {
				Version    uint64
				Projection []byte
			}
			var projected app.Event
			if tx.Raw("SELECT version,CAST(projection AS BINARY) AS projection FROM ai_bridge_requests WHERE request_id=?", f.request.RequestID).Scan(&current).Error != nil || current.Version != 2 || json.Unmarshal(current.Projection, &projected) != nil || projected.EventID != f.projection.EventID || projected.Version != 2 {
				t.Fatal("older original event replaced current projection")
			}
			keys := map[string]bool{}
			foundMismatch := false
			for offset := uint64(0); ; {
				rows, next, e := s.ObservationsPage(offset, 2)
				if e != nil || next != offset+uint64(len(rows)) {
					t.Fatal("native body-free observation pagination")
				}
				for _, row := range rows {
					if row.Store == "ai_bridge_events" {
						if keys[row.PrimaryKeySHA256] {
							t.Fatal("different original PK observations collapsed")
						}
						keys[row.PrimaryKeySHA256] = true
					}
					for _, reason := range row.Reasons {
						foundMismatch = foundMismatch || reason == "inbox_interpretation_event_conflict"
					}
				}
				if len(rows) == 0 {
					break
				}
				offset = next
			}
			if len(keys) != wantRows || !keys[aiReverseKeySHA([]string{f.projection.EventID})] || wantRows == 2 && !keys[aiReverseKeySHA([]string{string(prior.eventRow[0])})] {
				t.Fatal("native original event PK digest coverage lost")
			}
			report := s.Summary()
			if len(report.Ledgers) != 14 || !report.WholeLedgerEOF || !report.ActualReadOnlyRR || report.MigrationVersion != 99 || report.SourceAuthenticationRequired || report.GlobalReverseQualified || report.CASAuthority || report.DropReady {
				t.Fatal("native scan provenance or authority changed")
			}
			if which == "physical_id_mismatch" {
				if report.Blocking == 0 || !foundMismatch || s.byTable["ai_bridge_events"][prior.event.EventID] != nil {
					t.Fatal("wrong original event PK substituted or conflict hidden")
				}
			} else if report.Blocking != 0 || report.Unknown != 0 {
				t.Fatal("valid original event PK rows rejected", report.BlockingReasons)
			}
			var live int
			if tx.Raw("SELECT 1").Scan(&live).Error != nil || live != 1 {
				t.Fatal("adapter ended borrowed host transaction")
			}
		})
	}
}

// The native responsibility case must really change its one original ACK.
// A successful UPDATE affecting zero rows would not exercise staged transport.
func aiReverseNativeStageReceiptACK(t *testing.T, pool *sql.DB) {
	t.Helper()
	result, e := pool.ExecContext(t.Context(), "UPDATE ai_messaging_outbox SET stage='staged',confirmed_at=NULL WHERE message_id=?", "80000000-0000-4000-8000-000000000001")
	if e != nil {
		t.Fatal("native ACK stage preparation rejected")
	}
	changed, e := result.RowsAffected()
	if e != nil || changed != 1 {
		t.Fatal("native ACK stage preparation did not affect exactly one row")
	}
}

func TestAIReverseNativeWholeFourteenAndUnrelatedTraffic(t *testing.T) {
	db, pool := aiReverseNativeDatabase(t)
	aiReverseNativeGraph(t, pool)
	aiReverseNativeStageReceiptACK(t, pool)
	s, ctx, tx := aiReverseNativePrepare(t, db)
	defer aiReverseNativeEnd(t, tx)
	f := aiReverseNativeCopies(t, pool)
	aiReverseNativeBind(t, ctx, s, f)
	r := s.Summary()
	if len(r.Ledgers) != 14 || !r.WholeLedgerEOF || !r.ActualReadOnlyRR || r.SourceAuthenticationRequired || r.Related != 0 || r.Blocking != 0 || r.OutsideActive != 1 || r.GlobalReverseQualified || r.CASAuthority || r.DropReady || !r.ExternalOriginRequired || !r.ExternalQSAIClosureRequired {
		t.Fatal("full scan/outside diagnostic contract", r.BlockingReasons)
	}
	var live int
	if tx.Raw("SELECT 1").Scan(&live).Error != nil || live != 1 {
		t.Fatal("adapter ended host transaction")
	}
	var count int
	if pool.QueryRow("SELECT COUNT(*) FROM ai_messaging_operations").Scan(&count) != nil || count != 1 {
		t.Fatal("read-only scan altered ledger")
	}
}
func TestAIReverseNativeSourceScopeTargetPendingAndRawNULL(t *testing.T) {
	db, pool := aiReverseNativeDatabase(t)
	aiReverseNativeGraph(t, pool)
	row := aiFixtureRow(t, AIBridgeCommandSource, "start")
	row[5] = []byte("1")
	args := make([]any, len(row))
	for i, cell := range row {
		if cell != nil {
			args[i] = cell
		}
	}
	aiNativeExec(t, pool, "INSERT INTO ai_bridge_commands VALUES(?,?,?,?,?,?,?,?)", args...)
	aiReverseNativeStageReceiptACK(t, pool)
	s, ctx, tx := aiReverseNativePrepare(t, db)
	defer aiReverseNativeEnd(t, tx)
	f := aiReverseNativeCopies(t, pool)
	aiReverseNativeBind(t, ctx, s, f)
	if s.report.Related == 0 || s.report.Blocking == 0 || s.report.OutsideActive != 0 || s.report.SourceAuthenticationRequired {
		t.Fatal("authenticated target pending responsibility omitted")
	}
	for _, l := range s.report.Ledgers {
		if l.Store == AIBridgeCommandSource && l.Rows != 1 {
			t.Fatal("actual native source row omitted")
		}
	}
}
func TestAIReverseNativeGlobalOrphanCrossOrgAndUnknownKind(t *testing.T) {
	for _, which := range []string{"operation_missing", "row_org", "unknown_kind", "inbox_missing"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiReverseNativeDatabase(t)
			aiReverseNativeGraph(t, pool)
			switch which {
			case "operation_missing":
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_operations")
			case "row_org":
				aiNativeExec(t, pool, "UPDATE ai_messaging_outbox SET organization_id=999 WHERE message_id=?", aiFixtureRequestID)
			case "unknown_kind":
				aiNativeExec(t, pool, "UPDATE ai_messaging_inbox SET kind=999")
			case "inbox_missing":
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_inbox")
			}
			s, ctx, tx := aiReverseNativePrepare(t, db)
			defer aiReverseNativeEnd(t, tx)
			aiReverseNativeBind(t, ctx, s, aiReverseNativeCopies(t, pool))
			if s.report.Blocking == 0 {
				t.Fatal("unrelated scope hid orphan/conflict/unknown")
			}
		})
	}
}
func TestAIReverseNativeFreshActualEndedEpochAndLowerBoundDrift(t *testing.T) {
	for _, which := range []string{"identical", "lower_pk_changed", "deleted", "schema", "NULL_clock"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiReverseNativeDatabase(t)
			aiReverseNativeGraph(t, pool)
			old, ctx, tx := aiReverseNativePrepare(t, db)
			f := aiReverseNativeCopies(t, pool)
			firstCoordinator := aiReverseNativeBind(t, ctx, old, f)
			if _, e := old.RecheckFresh(ctx, old.snapshot, 99, firstCoordinator, f.inputs()); e == nil {
				t.Fatal("same actual snapshot treated as fresh")
			}
			aiReverseNativeEnd(t, tx)
			switch which {
			case "lower_pk_changed":
				aiNativeExec(t, pool, "UPDATE ai_messaging_observations SET recorded_count=recorded_count+1 WHERE kind='duplicate_event'")
			case "deleted":
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_failures")
			case "schema":
				aiNativeExec(t, pool, "ALTER TABLE ai_messaging_observations ADD COLUMN future_unknown BIGINT NULL")
			case "NULL_clock":
				aiNativeExec(t, pool, "UPDATE ai_bridge_requests SET updated_at='2026-10-08 11:12:14.123456'")
			}
			freshCtx, freshTx, cycle := aiReverseNativeContext(t, db)
			defer aiReverseNativeEnd(t, freshTx)
			secondCoordinator, e := PrepareHistoricalCoordinator(freshCtx, coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if e != nil {
				t.Fatal("independent complete four-copy coordinator")
			}
			if _, e = old.RecheckFresh(freshCtx, cycle, 99, firstCoordinator, f.inputs()); e == nil {
				t.Fatal("first coordinator reused as current epoch authentication")
			}
			if _, e = old.RecheckFresh(freshCtx, cycle, 98, secondCoordinator, f.inputs()); e == nil {
				t.Fatal("caller migration substituted for actual clean99")
			}
			proof, e := old.RecheckFresh(freshCtx, cycle, 99, secondCoordinator, f.inputs())
			if which == "identical" {
				if e != nil || proof == nil || proof.self != proof || proof.fresh.pool == old.pool || proof.fresh.scope.owner != secondCoordinator {
					t.Fatal("actual ended/new paired epoch", e)
				}
				if !reflect.DeepEqual(proof.old.report.SourceCopies, proof.fresh.report.SourceCopies) {
					t.Fatal("source copy baselines changed")
				}
			} else if e == nil || proof != nil {
				t.Fatal("inside-upper/schema/NULL drift accepted")
			}
		})
	}
}
func TestAIReverseNativeBudgetAndTruncatedBorrowedCopy(t *testing.T) {
	db, pool := aiReverseNativeDatabase(t)
	aiReverseNativeGraph(t, pool)
	ctx, tx, cycle := aiReverseNativeContext(t, db)
	defer aiReverseNativeEnd(t, tx)
	limits := DefaultAIReverseLimits()
	limits.MaxRows = 1
	if s, e := PrepareAIReverseSnapshot(ctx, cycle, 99, limits); !errors.Is(e, ErrAIReverseBounds) || s != nil {
		t.Fatal("budget truncation exposed complete snapshot")
	}
	s, e := PrepareAIReverseSnapshot(ctx, cycle, 99, DefaultAIReverseLimits())
	if e != nil {
		t.Fatal(e)
	}
	for _, which := range []string{"nonempty_frame_truncated", "empty_source_illegal_trailer"} {
		t.Run(which, func(t *testing.T) {
			f := aiReverseNativeCopies(t, pool)
			if which == "nonempty_frame_truncated" {
				// This is a legal synthetic source-copy frame, not a Mongo
				// origin/closure proof. Only the fourth stream EOF is tested.
				body := wireFixture(t, "answersheet.submitted")
				row := marshalBSON(t, fixtureMongoRow(t, body, int64(1)))
				f.raw[3], f.expected[3] = fixtureMongoCopy(t, [][]byte{row})
				if len(f.raw[3]) <= 8 || f.expected[3].Records != 1 || f.expected[3].Boundary.Empty {
					t.Fatal("valid nonempty fourth-source frame required before truncation")
				}
			} else if len(f.raw[3]) != 0 || f.expected[3].Records != 0 || !f.expected[3].Boundary.Empty {
				t.Fatal("valid empty fourth-source fixture required")
			}
			// Authenticate all four complete, unmodified fixture copies first;
			// the second stream pass must reject the unchanged expectation's
			// deliberately damaged bytes without publishing any source scope.
			c, e := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
			if e != nil {
				t.Fatal("complete original four-source fixture", e)
			}
			inputs := f.inputs()
			if which == "nonempty_frame_truncated" {
				inputs[3].Input = bytes.NewReader(f.raw[3][:len(f.raw[3])-1])
			} else {
				inputs[3].Input = bytes.NewReader(append(append([]byte(nil), f.raw[3]...), byte(0)))
			}
			if e = c.BindAIReverseSourceScope(ctx, s, inputs); e == nil || s.scope != nil || !s.report.SourceAuthenticationRequired {
				t.Fatal("incomplete fourth EOF published source scope")
			}
		})
	}
}

func TestAIReverseNativePendingSessionIdentityBeforeFirstProjection(t *testing.T) {
	for _, which := range []string{"bound", "wrong_session", "missing_original_start"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiReverseNativeDatabase(t)
			aiReverseNativeGraph(t, pool)
			aiNativeExec(t, pool, "DELETE FROM ai_bridge_events")
			aiNativeExec(t, pool, "DELETE FROM ai_messaging_inbox WHERE kind=?", int(pb.MessagingKind_INTERPRETATION_STATE))
			aiNativeExec(t, pool, "DELETE FROM ai_messaging_outbox WHERE message_id=?", "80000000-0000-4000-8000-000000000002")
			aiNativeExec(t, pool, "UPDATE ai_bridge_requests SET projection=NULL,version=0,status='pending'")
			switch which {
			case "wrong_session":
				aiNativeExec(t, pool, "UPDATE ai_bridge_requests SET session_id=?", "90000000-0000-4000-8000-000000000001")
			case "missing_original_start":
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_operations")
			}
			s, ctx, tx := aiReverseNativePrepare(t, db)
			defer aiReverseNativeEnd(t, tx)
			aiReverseNativeBind(t, ctx, s, aiReverseNativeCopies(t, pool))
			r := s.Summary()
			n := s.byTable["ai_bridge_requests"][aiFixtureRequestID]
			if which == "bound" {
				if r.Blocking != 0 || !n.projectionAbsent || n.observation.Invalid || !n.observation.Unfinished || r.OutsideActive != 1 {
					t.Fatal("actual accepted START-only session transition falsely damaged or terminal", r.BlockingReasons)
				}
			} else if r.Blocking == 0 || !n.observation.Invalid {
				t.Fatal("unproven pending session bypassed exact original START graph")
			}
			if r.CASAuthority || r.DropReady || !r.ExternalQSAIClosureRequired {
				t.Fatal("pending session classification granted authority")
			}
		})
	}
}
func aiReverseNativeMappedSource(t *testing.T, pool *sql.DB) {
	t.Helper()
	old := aiFixtureRow(t, AIBridgeCommandSource, "start")
	args := make([]any, len(old))
	for i, cell := range old {
		if cell != nil {
			args[i] = cell
		}
	}
	aiNativeExec(t, pool, "INSERT INTO ai_bridge_commands VALUES(?,?,?,?,?,?,?,?)", args...)
	// The retained handoff copies MySQL's actual CAST(payload AS BINARY), not
	// the fixture's client serialization. Preserve that exact original copy.
	var actual []byte
	if pool.QueryRow("SELECT CAST(payload AS BINARY) FROM ai_bridge_commands WHERE command_id=?", aiFixtureRequestID).Scan(&actual) != nil {
		t.Fatal("actual native source payload read")
	}
	var hash string
	if pool.QueryRow("SELECT body_sha256 FROM ai_messaging_operations WHERE command_id=?", aiFixtureRequestID).Scan(&hash) != nil {
		t.Fatal("actual native live body binding")
	}
	aiNativeExec(t, pool, "INSERT INTO ai_messaging_legacy_commands VALUES(?,?,?,?,?,?,?,?,?,?)", aiFixtureRequestID, aiFixtureRequestID, "start", actual, string(old[4]), 3, string(old[7]), "", hash, "2026-10-08 11:12:14.123456")
}
func TestAIReverseNativeExactMappedUndeliveredAndInheritedResponsibility(t *testing.T) {
	for _, which := range []string{"settled_mapping", "budget_reset", "original_time_conflict", "raw_payload_conflict", "unmapped", "current_pending"} {
		t.Run(which, func(t *testing.T) {
			db, pool := aiReverseNativeDatabase(t)
			aiReverseNativeGraph(t, pool)
			aiReverseNativeMappedSource(t, pool)
			switch which {
			case "budget_reset":
				aiNativeExec(t, pool, "UPDATE ai_messaging_outbox SET attempts=2 WHERE message_id=?", aiFixtureRequestID)
			case "original_time_conflict":
				aiNativeExec(t, pool, "UPDATE ai_messaging_legacy_commands SET source_original_time=?", "2026-10-08T11:12:12.123456+08:00")
			case "raw_payload_conflict":
				aiNativeExec(t, pool, "UPDATE ai_messaging_legacy_commands SET source_payload=CONCAT(source_payload,CHAR(10))")
			case "unmapped":
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_legacy_commands")
			case "current_pending":
				aiNativeExec(t, pool, "UPDATE ai_messaging_operations SET decision='',code='',receipt_id=NULL,receipt=NULL,decided_at=NULL")
				aiNativeExec(t, pool, "UPDATE ai_messaging_outbox SET stage='awaiting_receipt',confirmed_at=NULL WHERE message_id=?", aiFixtureRequestID)
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_failures")
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_inbox WHERE kind=?", int(pb.MessagingKind_COMMAND_RECEIPT))
				aiNativeExec(t, pool, "DELETE FROM ai_messaging_outbox WHERE message_id=?", "80000000-0000-4000-8000-000000000001")
			}
			s, ctx, tx := aiReverseNativePrepare(t, db)
			defer aiReverseNativeEnd(t, tx)
			copies := aiReverseNativeCopies(t, pool)
			if which == "raw_payload_conflict" {
				if c, e := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), copies.inputs(), DefaultHistoricalCoordinatorLimits()); e == nil || c != nil {
					t.Fatal("full four-copy auth accepted different raw bytes for mapped same command")
				}
				old, legacy := s.byTable[AIBridgeCommandSource][aiFixtureRequestID], s.byTable[AILegacyCommandSource][aiFixtureRequestID]
				if !old.observation.Invalid || !legacy.observation.Invalid || !old.observation.Unfinished || s.scope != nil || !s.report.SourceAuthenticationRequired {
					t.Fatal("raw copy conflict lost its local/source-auth blocker")
				}
				return
			}
			aiReverseNativeBind(t, ctx, s, copies)
			r := s.Summary()
			old := s.byTable[AIBridgeCommandSource][aiFixtureRequestID]
			if which == "settled_mapping" {
				if r.Blocking != 0 || old.delivered || old.observation.Unfinished || old.observation.Invalid {
					t.Fatal("actual exact handoff retained obsolete pending or invented gRPC delivery", r.BlockingReasons)
				}
			} else if which == "current_pending" {
				op := s.byTable["ai_messaging_operations"][aiFixtureRequestID]
				box := s.byTable["ai_messaging_outbox"][aiFixtureRequestID]
				if old.observation.Unfinished || op.observation.Invalid || !op.observation.Unfinished || !box.observation.Unfinished || r.Blocking == 0 {
					t.Fatal("handoff lost actual current pending transport responsibility", r.BlockingReasons)
				}
			} else if r.Blocking == 0 || !old.observation.Unfinished {
				t.Fatal("bad/unmapped source cleared original responsibility", r.BlockingReasons)
			}
			var delivered int
			if pool.QueryRow("SELECT delivered FROM ai_bridge_commands WHERE command_id=?", aiFixtureRequestID).Scan(&delivered) != nil || delivered != 0 {
				t.Fatal("read-only reverse scan modified original delivered state")
			}
		})
	}
}
func TestAIReverseNativeEvaluationOriginalReceiptKindAndRun(t *testing.T) {
	for _, kind := range []pb.MessagingKind{pb.MessagingKind_EVALUATION_START, pb.MessagingKind_EVALUATION_CANCEL} {
		for _, shape := range []string{"exact", "wrong_run", "workflow", "rejected"} {
			t.Run(kind.String()+"/"+shape, func(t *testing.T) {
				db, pool := aiReverseNativeDatabase(t)
				f := aiReverseEvaluationGraph(t, kind, shape)
				aiNativeInsert(t, pool, "ai_messaging_operations", f.op)
				for _, row := range f.boxes {
					aiNativeInsert(t, pool, "ai_messaging_outbox", row)
				}
				for _, row := range f.inbox {
					aiNativeInsert(t, pool, "ai_messaging_inbox", row)
				}
				aiNativeExec(t, pool, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", f.aggregate)
				s, ctx, tx := aiReverseNativePrepare(t, db)
				defer aiReverseNativeEnd(t, tx)
				aiReverseNativeBind(t, ctx, s, aiReverseNativeCopies(t, pool))
				r := s.Summary()
				if shape == "exact" || shape == "rejected" {
					if r.Blocking != 0 || r.Unknown != 0 {
						t.Fatal("legitimate unrelated evaluation receipt/rejection blocked", r.BlockingReasons)
					}
				} else if r.Blocking == 0 {
					t.Fatal("full source scope hid wrong original receipt kind/run")
				}
				if r.GlobalReverseQualified || r.CASAuthority || r.DropReady || !r.StoredWireAuthenticationRequired {
					t.Fatal("local receipt facts fabricated global execution authority")
				}
			})
		}
	}
}

func TestAIReverseNativeCompactFreezeRequiresActualPairedOriginWithoutEndingHost(t *testing.T) {
	db, pool := aiReverseNativeDatabase(t)
	aiReverseNativeGraph(t, pool)
	s, ctx, tx := aiReverseNativePrepare(t, db)
	defer aiReverseNativeEnd(t, tx)
	f := aiReverseNativeCopies(t, pool)
	aiReverseNativeBind(t, ctx, s, f)
	for _, origin := range []*SourceOriginEpoch{nil, {}} {
		if anchor, e := s.FreezeFreshAnchor(ctx, origin); e == nil || anchor != nil {
			t.Fatal("missing/zero paired origin minted compact capability")
		}
	}
	var live int
	if tx.Raw("SELECT 1").Scan(&live).Error != nil || live != 1 {
		t.Fatal("failed compact freeze ended borrowed host Tx")
	}
}
