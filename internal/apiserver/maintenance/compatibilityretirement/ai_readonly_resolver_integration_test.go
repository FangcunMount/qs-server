//go:build integration

package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	drivermysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type aiReadonlyNativeFixture struct {
	*aiNativeFixture
	gorm *gorm.DB
}

// Only the independently inspected owned SQL fixture is reused. This helper
// creates a random namespace, runs the complete real SQL99 migration, and
// populates the same original point graph as the existing locking tests.
func newAIReadonlyNativeFixture(t *testing.T, mapped, closed bool) *aiReadonlyNativeFixture {
	t.Helper()
	db := aiNativeOwnedMySQL(t)
	var database string
	if db.QueryRowContext(t.Context(), "SELECT DATABASE()").Scan(&database) != nil {
		t.Fatal("owned namespace read failed")
	}
	if version, _, err := migration.NewMigrator(db, &migration.Config{Enabled: true, Database: database}).Run(); err != nil || version != 99 {
		var serverError *drivermysql.MySQLError
		var number uint16
		var state string
		if errors.As(err, &serverError) {
			number = serverError.Number
			state = string(serverError.SQLState[:])
		}
		t.Fatal("complete actual SQL99 migration failed", "migration_or_head_failure", version, number, state)
	}
	var actualHead uint64
	var actualDirty bool
	if db.QueryRowContext(t.Context(), "SELECT version,dirty FROM schema_migrations").Scan(&actualHead, &actualDirty) != nil || actualHead != 99 || actualDirty {
		t.Fatal("actual complete migration head99 clean proof failed")
	}
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	if err != nil {
		t.Fatal("owned GORM adapter failed")
	}
	graph := aiLocalGraph(t)
	raw, _ := aiFixturePayload(t, "start")
	projected, err := json.Marshal(graph.projection)
	if err != nil {
		t.Fatal("synthetic projection encoding failed")
	}
	aiNativeExec(t, db, "UPDATE ai_messaging_admission SET closed=?,revision=7 WHERE singleton=1", closed)
	aiNativeExec(t, db, `INSERT INTO ai_bridge_requests(request_id,request_hash,payload,session_id,version,status,projection,organization_id,subject_id,testee_id,created_at,updated_at) VALUES(?,?,CONVERT(? USING utf8mb4),?,?,?,CONVERT(? USING utf8mb4),?,?,?,'2026-10-08 11:12:12.123456','2026-10-08 11:12:14.123456')`, graph.request.RequestID, sourceSHA(raw), raw, graph.projection.SessionID, graph.projection.Version, graph.projection.Status, projected, graph.request.Actor.OrgID, graph.request.Actor.SubjectID, graph.request.TesteeID)
	delivered := 1
	if mapped {
		delivered = 0
		aiNativeExec(t, db, "INSERT INTO ai_messaging_aggregates VALUES(?,2)", graph.request.RequestID)
	}
	aiNativeExec(t, db, `INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash,delivered,attempts,available_at) SELECT request_id,request_id,'start',payload,request_hash,?,3,'2026-10-08 11:12:13.123456' FROM ai_bridge_requests`, delivered)
	for _, id := range graph.request.AssessmentIDs {
		aiNativeExec(t, db, "INSERT INTO ai_bridge_request_assessments VALUES(?,?)", graph.request.RequestID, id)
	}
	for _, row := range graph.events {
		aiNativeInsert(t, db, "ai_bridge_events", row)
	}
	if mapped {
		aiNativeExec(t, db, `INSERT INTO ai_messaging_legacy_commands SELECT command_id,request_id,kind,CAST(payload AS BINARY),payload_hash,attempts,available_at,'2026-10-08T19:12:12.123456+08:00',?,'2026-10-08 11:12:14.123456' FROM ai_bridge_commands`, graph.hashes[graph.request.RequestID])
		for _, group := range []struct {
			table string
			rows  [][][]byte
		}{{"ai_messaging_operations", graph.ops}, {"ai_messaging_outbox", graph.boxes}, {"ai_messaging_inbox", graph.inbox}, {"ai_messaging_failures", graph.failures}} {
			for _, row := range group.rows {
				aiNativeInsert(t, db, group.table, row)
			}
		}
	}
	binding := aiNativeBinding(t, db)
	reader := &AILocalResolver{pool: db}
	rows, err := reader.read(t.Context(), aiBridgeQuery, graph.request.RequestID, graph.request.RequestID)
	if err != nil || len(rows) != 1 {
		t.Fatal("actual native source read failed")
	}
	bridge, err := aiCurrentSource(rows[0], AIBridgeCommandSource, binding.BridgeColumns, binding.BridgeBoundary)
	if err != nil {
		t.Fatal("original bridge decode failed")
	}
	var legacy *DecodedAICommand
	if mapped {
		rows, err = reader.read(t.Context(), aiLegacyQuery, graph.request.RequestID, graph.request.RequestID)
		if err != nil || len(rows) != 1 {
			t.Fatal("original legacy read failed")
		}
		legacy, err = aiCurrentSource(rows[0], AILegacyCommandSource, binding.LegacyColumns, binding.LegacyBoundary)
		if err != nil {
			t.Fatal("original legacy decode failed")
		}
	}
	return &aiReadonlyNativeFixture{aiNativeFixture: &aiNativeFixture{db: db, binding: binding, bridge: bridge, legacy: legacy, graph: graph}, gorm: gdb}
}

func aiReadonlyNativeCopies(t *testing.T, f *aiReadonlyNativeFixture) authFixture {
	t.Helper()
	reader := &AILocalResolver{pool: f.db}
	var copies authFixture
	copies.raw[0], copies.expected[0] = fixtureSQLCopy(t, nil, nil)
	copies.raw[3], copies.expected[3] = fixtureMongoCopy(t, nil)
	for i, target := range []struct {
		table, q string
		columns  SQLColumns
		boundary SourceBoundary
	}{{AIBridgeCommandSource, aiBridgeQuery, f.binding.BridgeColumns, f.binding.BridgeBoundary}, {AILegacyCommandSource, aiLegacyQuery, f.binding.LegacyColumns, f.binding.LegacyBoundary}} {
		rows, err := reader.read(t.Context(), target.q, f.graph.request.RequestID, f.graph.request.RequestID)
		if err != nil {
			t.Fatal("full actual source read failed")
		}
		copies.raw[i+1], copies.expected[i+1] = coordinatorNativeAICopy(t, target.table, rows, target.columns, target.boundary)
	}
	return copies
}

func aiReadonlyNativeEpoch(t *testing.T, f *aiReadonlyNativeFixture, copies authFixture) (context.Context, *gorm.DB, *SQLResponsibilitySnapshot, *HistoricalCoordinator, *AIReadOnlyResolver) {
	t.Helper()
	tx := f.gorm.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if tx.Error != nil {
		t.Fatal("actual RRRO transaction begin failed")
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil && err != sql.ErrTxDone {
			t.Error("owned RRRO rollback failed")
		}
	})
	ctx := hostmysql.WithTx(t.Context(), tx)
	current, err := PrepareSQLResponsibilitySnapshot(ctx, f.binding.DatabaseIdentityHash, sqlevaluation.DefaultSQLResponsibilityLimits())
	if err != nil {
		t.Fatal("full eight-ledger actual snapshot failed", err)
	}
	c, err := PrepareHistoricalCoordinator(ctx, HistoricalCoordinatorBinding{f.binding.SourceSHA, f.binding.OperationID}, copies.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal("actual four-copy authentication failed", err)
	}
	ro, err := c.PrepareAIReadOnlyResolver(ctx, current, 99, copies.inputs()[1:3])
	if err != nil {
		t.Fatal("readonly original resolver preparation failed", err)
	}
	return ctx, tx, current, c, ro
}

func aiReadonlyNativeQualify(t *testing.T, ctx context.Context, c *HistoricalCoordinator, ro *AIReadOnlyResolver) []HistoricalCandidate {
	t.Helper()
	page, err := c.NextPage(ctx)
	if err != nil {
		t.Fatal("actual source page failed")
	}
	if err = c.QualifyAIReadOnlyPage(ctx, page, ro); err != nil {
		t.Fatal("actual readonly point page failed", err)
	}
	if _, err = c.NextPage(ctx); err != io.EOF {
		t.Fatal("four-source second EOF failed", err)
	}
	rows, err := c.CandidateRange(0, 128)
	if err != nil {
		t.Fatal("candidate range failed")
	}
	return rows
}

func TestAIReadOnlyNativeActualRRROAndBorrowedLifecycle(t *testing.T) {
	for _, mapped := range []bool{false, true} {
		for _, closed := range []bool{false, true} {
			name := "unmapped_open"
			if mapped {
				name = "mapped_open"
			}
			if closed {
				name = strings.TrimSuffix(name, "open") + "closed"
			}
			t.Run(name, func(t *testing.T) {
				f := newAIReadonlyNativeFixture(t, mapped, closed)
				copies := aiReadonlyNativeCopies(t, f)
				ctx, tx, current, c, ro := aiReadonlyNativeEpoch(t, f, copies)
				rows := aiReadonlyNativeQualify(t, ctx, c, ro)
				want := 1
				if mapped {
					want = 2
				}
				if len(rows) != want {
					t.Fatal("physical AI source candidate omitted")
				}
				for _, row := range rows {
					if !row.LocalQualified || !evidenceHash(row.BusinessBaselineSHA256) || len(row.RequiredAdapters) == 0 || len(row.HistoricalGaps) < 4 {
						t.Fatal("readonly local facts missing or external authority overstated")
					}
				}
				if s := ro.Summary(); s.AdmissionClosed != closed || s.AdmissionRevision != 7 || !s.ActualReadOnlyRR || s.CASAuthority || s.DropReady || s.ExternalClosure != "unknown" || s.GlobalReverseCoverage != "unknown" || !evidenceHash(s.AdmissionRowSHA256) {
					t.Fatal("readonly epoch summary overstated")
				}
				if receipt := c.Receipt(); !receipt.SourceCoverageComplete || receipt.BusinessClosureVerified || receipt.DropReady || receipt.LocallyQualifiedCount != uint64(want) {
					t.Fatal("readonly candidate receipt incorrect")
				}
				if current.ValidateBorrowedSnapshot(ctx) != nil {
					t.Fatal("resolver changed transaction ownership")
				}
				borrowed := copies.inputs()[1:3]
				tracked := [2]*authObservedReader{{Reader: bytes.NewReader(copies.raw[1])}, {Reader: bytes.NewReader(copies.raw[2])}}
				for i := range borrowed {
					borrowed[i].Input = tracked[i]
				}
				if _, err := c.PrepareAIReadOnlyResolver(ctx, current, 99, borrowed); err != nil {
					t.Fatal("complete borrowed reread failed", err)
				}
				for _, r := range tracked {
					if r.closes != 0 || r.Len() != 0 || r.reads == 0 {
						t.Fatal("adapter took reader lifecycle or missed real EOF")
					}
				}
				if tx.Rollback().Error != nil {
					t.Fatal("resolver closed or committed borrowed transaction")
				}
				if ro.validate(ctx) != ErrAILocalTransaction {
					t.Fatal("ended borrowed transaction still admitted")
				}
			})
		}
	}
}

func TestAIReadOnlyNativeSnapshotIsolationAndFreshEpochDrift(t *testing.T) {
	for _, change := range []string{"attempts", "admission", "body", "retired"} {
		t.Run(change, func(t *testing.T) {
			f := newAIReadonlyNativeFixture(t, true, false)
			copies := aiReadonlyNativeCopies(t, f)
			ctx, tx, _, c, ro := aiReadonlyNativeEpoch(t, f, copies)
			before, err := ro.resolve(ctx, f.bridge, f.legacy)
			if err != nil {
				t.Fatal("initial actual graph resolution failed", err)
			}
			switch change {
			case "attempts":
				aiNativeExec(t, f.db, "UPDATE ai_bridge_commands SET attempts=attempts+1")
			case "admission":
				aiNativeExec(t, f.db, "UPDATE ai_messaging_admission SET closed=TRUE,revision=revision+1")
			case "body":
				aiNativeExec(t, f.db, "UPDATE ai_messaging_outbox SET body=CONCAT(body,' ') LIMIT 1")
			case "retired":
				// This satisfies the real SQL97 CHECK only as a negative synthetic
				// fixture. Its marker is not a reviewed retirement conclusion.
				aiNativeExec(t, f.db, `UPDATE ai_messaging_operations SET retired=TRUE,
					body_sha256=NULL,aggregate_sequence=NULL,created_at=NULL,
					decision='',code='',receipt_id=NULL,receipt=NULL,decided_at=NULL,
					retirement_evidence=CONVERT(? USING utf8mb4),
					retired_at='2026-10-08 11:12:15.123456'`,
					`{"version":1,"ownership_verified":true,"responsibility_closed":true,"business_terminal":true,"fixture":"synthetic_native_not_production_proof"}`)
			}
			after, err := ro.resolve(ctx, f.bridge, f.legacy)
			if err != nil || before.Summary().BaselineSHA256 != after.Summary().BaselineSHA256 {
				t.Fatal("same actual RO snapshot lost isolation")
			}
			first := aiReadonlyNativeQualify(t, ctx, c, ro)
			if tx.Rollback().Error != nil {
				t.Fatal("first epoch lifecycle failed")
			}
			ctx2, _, _, c2, ro2 := aiReadonlyNativeEpoch(t, f, copies)
			if change == "admission" {
				second := aiReadonlyNativeQualify(t, ctx2, c2, ro2)
				if len(first) != len(second) || first[0].BusinessBaselineSHA256 == second[0].BusinessBaselineSHA256 || ro.Summary() == ro2.Summary() {
					t.Fatal("fresh epoch hid actual admission drift")
				}
			} else {
				p, err := c2.NextPage(ctx2)
				if err != nil {
					t.Fatal("fresh original page failed")
				}
				if c2.QualifyAIReadOnlyPage(ctx2, p, ro2) == nil || p.consumed || len(c2.candidates) != 0 {
					t.Fatal("fresh epoch hid changed point graph")
				}
			}
		})
	}
}

func TestAIReadOnlyNativePreparationAndPageBindingRefusals(t *testing.T) {
	f := newAIReadonlyNativeFixture(t, true, false)
	copies := aiReadonlyNativeCopies(t, f)
	ctx, tx, current, c, ro := aiReadonlyNativeEpoch(t, f, copies)
	for _, test := range []struct {
		name     string
		snapshot *SQLResponsibilitySnapshot
		head     uint64
		copies   []SourceCopyInput
	}{{"nil", nil, 99, copies.inputs()[1:3]}, {"empty_capability", &SQLResponsibilitySnapshot{}, 99, copies.inputs()[1:3]}, {"wrong_head", current, 98, copies.inputs()[1:3]}, {"missing_copy", current, 99, copies.inputs()[1:2]}, {"changed_source", current, 99, func() []SourceCopyInput {
		in := copies.inputs()[1:3]
		in[0].Expected.DataHash = strings.Repeat("f", 64)
		return in
	}()}, {"truncated", current, 99, func() []SourceCopyInput {
		in := copies.inputs()[1:3]
		in[0].Input = bytes.NewReader(copies.raw[1][:len(copies.raw[1])-2])
		return in
	}()}} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := c.PrepareAIReadOnlyResolver(ctx, test.snapshot, test.head, test.copies); err == nil {
				t.Fatal("invalid original readonly binding admitted")
			}
		})
	}
	if ro.validate(t.Context()) != ErrAILocalTransaction {
		t.Fatal("ordinary pool context admitted")
	}
	other := f.gorm.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if other.Error != nil {
		t.Fatal("other owned transaction failed")
	}
	defer func() {
		if err := other.Rollback().Error; err != nil {
			t.Error("other owned rollback failed")
		}
	}()
	if ro.validate(hostmysql.WithTx(t.Context(), other)) != ErrAILocalTransaction {
		t.Fatal("different real transaction admitted")
	}
	p, err := c.NextPage(ctx)
	if err != nil {
		t.Fatal("actual pending source page failed")
	}
	if tx.Rollback().Error != nil {
		t.Fatal("owned epoch close failed")
	}
	if c.QualifyAIReadOnlyPage(ctx, p, ro) != ErrAILocalTransaction || p.consumed || len(c.candidates) != 0 {
		t.Fatal("ended snapshot consumed successful page")
	}
}

func TestAIReadOnlyNativeBlocksUnknownHeldCrossOrgAndSourceChanges(t *testing.T) {
	for _, change := range []string{"held", "unknown", "cross_org", "changed_source"} {
		t.Run(change, func(t *testing.T) {
			f := newAIReadonlyNativeFixture(t, true, false)
			copies := aiReadonlyNativeCopies(t, f)
			switch change {
			case "held":
				aiNativeExec(t, f.db, "UPDATE ai_messaging_inbox SET outcome='held' LIMIT 1")
			case "unknown":
				aiNativeExec(t, f.db, "INSERT INTO ai_messaging_quarantine VALUES(?,'private-corrupt-wire','private-code',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", strings.Repeat("f", 64))
			case "cross_org":
				aiNativeExec(t, f.db, "UPDATE ai_messaging_operations SET organization_id=1")
			case "changed_source":
				aiNativeExec(t, f.db, "UPDATE ai_bridge_commands SET attempts=attempts+1")
			}
			ctx, _, _, c, ro := aiReadonlyNativeEpoch(t, f, copies)
			p, err := c.NextPage(ctx)
			if err != nil {
				t.Fatal("actual original page failed")
			}
			if c.QualifyAIReadOnlyPage(ctx, p, ro) == nil || p.consumed || len(c.candidates) != 0 {
				t.Fatal("unfinished/conflicting graph consumed a successful page")
			}
		})
	}
}

func TestAIReadOnlyNativeEmptySourcesStillBindAdmission(t *testing.T) {
	f := newAIReadonlyNativeFixture(t, false, false)
	aiNativeExec(t, f.db, "DELETE FROM ai_bridge_commands")
	f.binding = aiNativeBinding(t, f.db)
	copies := aiReadonlyNativeCopies(t, f)
	ctx, tx, _, c, ro := aiReadonlyNativeEpoch(t, f, copies)
	if _, err := c.NextPage(ctx); err != io.EOF {
		t.Fatal("empty actual four-source EOF failed")
	}
	if ro.validate(ctx) != nil || ro.Summary().ExternalClosure != "unknown" || ro.Summary().GlobalReverseCoverage != "unknown" {
		t.Fatal("empty source widened closure")
	}
	if tx.Rollback().Error != nil {
		t.Fatal("first empty epoch close failed")
	}
	aiNativeExec(t, f.db, "UPDATE ai_messaging_admission SET revision=revision+1")
	_, _, _, _, fresh := aiReadonlyNativeEpoch(t, f, copies)
	if ro.Summary() == fresh.Summary() {
		t.Fatal("empty source hid actual admission drift")
	}
}
