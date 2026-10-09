//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"gorm.io/gorm"
)

// This test reuses the protected fixture's genuinely owned random namespaces.
// The helper's synthetic files are deliberately discarded: both SQL events and
// both Mongo events are inserted into the actual four original source objects,
// and originNativeCopies captures their actual columns, raw BSON and UUIDs.
// There is exactly one Requested/Outcome/Submitted/Generated original ID.
func historicalSpoolNativeFixture(t *testing.T) (*gorm.DB, *mongo.Client, *mongo.Database, MongoOwnerConfig, mongo.Session) {
	t.Helper()
	sqlDB, client, db, config, _, mongoEvents := wholeJointNativeFixture(t, false)
	mongoCASNativeHistoryIndexes(t, db)
	sheet := mongoBatchNativeAssessmentSheet()
	at := sheet.FilledAt
	requested := eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, QuestionnaireCode: "Q", QuestionnaireVer: "1.0", AnswerSheetID: "10042", ModelKind: "scale", ModelCode: "M", ModelVersion: "1.0", RequestedAt: at}
	committed := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "whole-joint-original-outcome", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
	sqlEvents := []event.DomainEvent{mongoBatchNativeEvent(t, requested, "evaluation.requested", "whole-joint-original-requested"), committed}
	for i, evt := range sqlEvents {
		body, err := domainwire.EncodeEvent(evt)
		if err != nil {
			t.Fatal("actual original SQL event encoder rejected", err)
		}
		row := fixtureSQLRow(t, body, strconv.Itoa(i+1))
		row[5] = []byte("7")
		originNativeInsertSQL(t, sqlDB, "domain_event_outbox", row)
	}
	if err := db.CreateCollection(t.Context(), "domain_event_outbox"); err != nil {
		t.Fatal("owned original collection creation rejected", err)
	}
	for i, evt := range mongoEvents {
		body, err := domainwire.EncodeEvent(evt)
		if err != nil {
			t.Fatal("actual original Mongo event encoder rejected", err)
		}
		row := setMongoField(fixtureMongoRow(t, body, int64(i+1)), "org_id", int64(7))
		if _, err = db.Collection("domain_event_outbox").InsertOne(t.Context(), row); err != nil {
			t.Fatal("owned original Mongo row insertion rejected", err)
		}
	}
	session, err := client.StartSession()
	if err != nil {
		t.Fatal("actual borrowed Mongo session unavailable", err)
	}
	t.Cleanup(func() { session.EndSession(context.Background()) })
	return sqlDB, client, db, config, session
}

type historicalSpoolNativeEpoch struct {
	coordinator *HistoricalCoordinator
	index       *WholeSourceJointIndex
	catalog     *SQLCrossStoreResponsibilityCatalog
	origin      *SourceOriginEpoch
	ai          *AIReverseSnapshot
	anchors     []*WholeSourceJointReplayAnchor
}

// Only real source authentication, global snapshots, business batches, private
// joint qualification and consume can create these anchors. No private self,
// receipt, terminal flag or candidate DTO is populated by the test host.
func historicalSpoolNativePrepareEpoch(t *testing.T, ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, copies authFixture) (*historicalSpoolNativeEpoch, error) {
	t.Helper()
	limits := DefaultHistoricalCoordinatorLimits()
	// The actual public contract rejects 1 (coordinator.go valid requires >=2).
	// Two bounded pages still share the same Assessment/Outcome/Sheet graph:
	// SQL page 1 changes only evidence; Mongo page 2 must preserve those deltas.
	limits.MaxPageRecords = 2
	c, err := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), copies.inputs(), limits)
	if err != nil {
		return nil, err
	}
	binding, err := c.BindOriginCopies(ctx, copies.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		return nil, err
	}
	origin, err := PrepareSourceOriginSnapshotEpoch(ctx, binding, current, global, originTestReaders(copies))
	if err != nil {
		return nil, err
	}
	index, err := c.PrepareWholeSourceJointIndex(ctx, wholeJointCopies(copies), DefaultWholeSourceJointLimits())
	if err != nil {
		return nil, err
	}
	catalog, err := PrepareSQLCrossStoreResponsibilityCatalog(ctx, current, sqlevaluation.DefaultSQLCrossStoreLimits())
	if err != nil {
		return nil, err
	}
	ai, err := PrepareAIReverseSnapshot(ctx, current, 99, DefaultAIReverseLimits())
	if err != nil {
		return nil, err
	}
	if err = c.BindAIReverseSourceScope(ctx, ai, copies.inputs()); err != nil {
		return nil, err
	}
	observed := ai.Summary()
	if !observed.WholeLedgerEOF || len(observed.Ledgers) != 14 || observed.Unknown != 0 || observed.Blocking != 0 || observed.GlobalReverseQualified || observed.CASAuthority || observed.DropReady {
		return nil, ErrHistoricalCASPersistence
	}
	epoch := &historicalSpoolNativeEpoch{coordinator: c, index: index, catalog: catalog, origin: origin, ai: ai}
	for {
		page, err := c.NextPage(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		sources, err := page.Events()
		if err != nil || len(sources) != 2 {
			return nil, ErrHistoricalCASPersistence
		}
		request, err := MongoHistoricalSQLBatchSelectors(sources)
		if err != nil {
			return nil, err
		}
		initial, err := PrepareSQLBusinessOwnerBatch(ctx, current, request, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return nil, err
		}
		joint, err := c.PrepareWholeSourceJointPage(ctx, page, index, catalog, initial, global)
		if err != nil {
			return nil, err
		}
		for _, source := range sources {
			candidate, err := joint.Candidate(ctx, source)
			if err != nil || !candidate.LocalQualified || len(candidate.BlockingReasons) != 0 {
				return nil, ErrHistoricalCASPersistence
			}
			facts, err := source.Facts()
			if err != nil || qualifiedCASSourceMatches(facts, candidate) != nil {
				return nil, ErrHistoricalCASPersistence
			}
			if _, _, err = qualifiedCASOwnerIDs(facts, candidate); err != nil {
				return nil, err
			}
		}
		if err = c.QualifyWholeSourceJointPage(ctx, page, joint); err != nil {
			return nil, err
		}
		anchor, err := c.SealWholeSourceJointReplayAnchor(ctx, joint)
		if err != nil {
			return nil, err
		}
		epoch.anchors = append(epoch.anchors, anchor)
	}
	receipt := c.Receipt()
	if len(epoch.anchors) != 2 || !receipt.SourceCoverageComplete || receipt.ConsumedRecords != [4]uint64{2, 0, 0, 2} || receipt.CandidateCount != 4 || receipt.CASComplete || receipt.BusinessClosureVerified || receipt.DropReady {
		return nil, ErrHistoricalCASPersistence
	}
	return epoch, nil
}

func historicalSpoolNativePrepared(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, config MongoOwnerConfig, session mongo.Session) (*HistoricalCASSpool, authFixture) {
	t.Helper()
	open := func(pattern string) *os.File {
		file, err := os.CreateTemp(t.TempDir(), pattern)
		if err != nil {
			t.Fatal("private O_EXCL spool unavailable", err)
		}
		t.Cleanup(func() {
			if file.Close() != nil {
				t.Error("borrowed private spool close failed")
			}
		})
		return file
	}
	spool, err := NewHistoricalCASSpool(t.Context(), open("mongo-"), open("sql-"), 32<<20, 16<<20)
	if err != nil {
		t.Fatal("actual private spool constructor rejected", err)
	}
	var copies authFixture
	var first *AIReverseRecheckAnchor
	err = persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
		copies = originNativeCopies(t, ctx, tx, global)
		epoch, err := historicalSpoolNativePrepareEpoch(t, ctx, current, global, copies)
		if err != nil {
			return err
		}
		first, err = epoch.ai.FreezeFreshAnchor(ctx, epoch.origin)
		return err
	})
	if err != nil {
		t.Fatal("genuine first source/global/joint epoch rejected", err)
	}
	// The first SQL RRRO and Mongo snapshot have actually ended. No old owner
	// graph survives here; only the producer's genuine graphless anchor does.
	err = persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
		epoch, err := historicalSpoolNativePrepareEpoch(t, ctx, current, global, copies)
		if err != nil {
			return err
		}
		ai, err := first.RecheckFresh(ctx, current, 99, epoch.coordinator, copies.inputs())
		if err != nil {
			return err
		}
		origin, err := first.RecheckOrigin(ctx, ai, current, global, epoch.coordinator, originTestReaders(copies))
		if err != nil {
			return err
		}
		for position, anchor := range epoch.anchors {
			sequence, offset, err := anchor.Selectors()
			if err != nil {
				return err
			}
			joint, err := epoch.coordinator.ReplayWholeSourceJointPage(ctx, anchor, sequence, offset, epoch.index, epoch.catalog, global)
			if err != nil {
				return err
			}
			_, _, sealed, err := epoch.coordinator.PrepareQualifiedHistoricalCAS(ctx, joint, origin, ai)
			if err != nil {
				return err
			}
			ticket, err := spool.Append(ctx, joint, sealed)
			if err != nil {
				return err
			}
			actualPosition, entries := ticket.Diagnostic()
			if actualPosition != position || entries != 2 {
				return ErrHistoricalCASSpool
			}
		}
		return spool.Seal(ctx, epoch.coordinator)
	})
	if err != nil {
		t.Fatal("genuine second epoch factory/replay/Append/Seal rejected", err)
	}
	// This return is after the second actual RO pair ended. It creates neither
	// a new expiry nor a transaction and cannot claim any production authority.
	return spool, copies
}

func historicalSpoolNativeApply(t *testing.T, ctx context.Context, sqlDB *gorm.DB, db *mongo.Database, session mongo.Session, spool *HistoricalCASSpool, rollback bool) (applied *HistoricalCASSpoolApplied, result error) {
	t.Helper()
	if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); err != nil {
		return nil, err
	}
	tx := sqlDB.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return nil, errors.Join(tx.Error, session.AbortTransaction(ctx))
	}
	sqlAttempted, mongoAttempted := false, false
	defer func() {
		if !sqlAttempted {
			result = errors.Join(result, tx.Rollback().Error)
		}
		if !mongoAttempted {
			result = errors.Join(result, session.AbortTransaction(ctx))
		}
	}()
	paired := mongo.NewSessionContext(hostmysql.WithTx(ctx, tx), session)
	tickets, err := spool.Tickets()
	if err != nil || len(tickets) != 2 {
		return nil, ErrHistoricalCASSpool
	}
	for _, ticket := range tickets {
		if err = spool.ApplyTicket(paired, ticket); err != nil {
			return nil, err
		}
	}
	applied, err = spool.FinishApply(paired)
	if err != nil {
		return nil, err
	}
	// Components must leave the actual host-owned writer and pool alive.
	var live int
	if err = tx.Raw("SELECT 1").Row().Scan(&live); err != nil || live != 1 {
		return nil, ErrHistoricalCASSpool
	}
	if rollback {
		return applied, nil
	}
	// These are actual TEST HOST responses, not a distributed atomic commit
	// and not an opaque production approval. An unknown response is not retried.
	sqlAttempted = true
	if err = tx.Commit().Error; err != nil {
		return nil, err
	}
	mongoAttempted = true
	if err = session.CommitTransaction(ctx); err != nil {
		return nil, err
	}
	if err = db.Client().Ping(ctx, nil); err != nil {
		return nil, err
	}
	return applied, nil
}

func historicalSpoolNativeReadback(t *testing.T, ctx context.Context, sqlDB *gorm.DB, db *mongo.Database, config MongoOwnerConfig, session mongo.Session, spool *HistoricalCASSpool, applied *HistoricalCASSpoolApplied, copies authFixture) (result error) {
	t.Helper()
	// Use the inherited ORIGINAL deadline, not the test helper's fresh parent
	// time. This does not certify that production 1.24M references fit that TTL.
	return persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(epochCtx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
		deadlineCtx, cancel := context.WithDeadline(epochCtx, deadlineFromSpoolNative(t, ctx))
		defer cancel()
		// Retain the exact borrowed SessionContext required by the transaction
		// adapter; WithDeadline alone preserves values but drops its interface.
		bounded := mongo.NewSessionContext(deadlineCtx, mongo.SessionFromContext(epochCtx))
		c, err := PrepareHistoricalCoordinator(bounded, coordinatorBinding(), copies.inputs(), DefaultHistoricalCoordinatorLimits())
		if err != nil {
			return err
		}
		index, err := c.PrepareWholeSourceJointIndex(bounded, wholeJointCopies(copies), DefaultWholeSourceJointLimits())
		if err != nil {
			return err
		}
		tickets, err := spool.Tickets()
		if err != nil || len(tickets) != 2 {
			return ErrHistoricalCASSpool
		}
		for _, ticket := range tickets {
			observed, err := spool.VerifyTicket(bounded, applied, ticket, current, global, index)
			if err != nil {
				return err
			}
			if observed.References != 2 || !observed.IndependentRawReadbackMatched || observed.HostCommitVerified || observed.WholeRetirementComplete || observed.AICommandsPersisted || observed.BusinessClosureVerified || observed.CASAuthorized || observed.DropReady {
				return ErrHistoricalCASSpool
			}
		}
		return spool.FinishReadback(bounded, applied)
	})
}

func deadlineFromSpoolNative(t *testing.T, ctx context.Context) time.Time {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("original inherited spool deadline missing")
	}
	return deadline
}

func historicalSpoolNativeCounts(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, committed bool) {
	t.Helper()
	expected := 0
	if committed {
		expected = 1
	}
	var lifecycle, outcome int
	if err := sqlDB.Raw("SELECT COALESCE(JSON_LENGTH(JSON_EXTRACT(historical_lifecycle_evidence,'$.entries')),0) FROM assessment WHERE id=42").Row().Scan(&lifecycle); err != nil {
		t.Fatal("actual lifecycle readback unavailable", err)
	}
	if err := sqlDB.Raw("SELECT COALESCE(JSON_LENGTH(JSON_EXTRACT(historical_committed_evidence,'$.entries')),0) FROM evaluation_outcome WHERE id=9001 AND committed_event_id IS NULL AND committed_event_evidence IS NULL").Row().Scan(&outcome); err != nil {
		t.Fatal("actual standard-slot-preserving Outcome readback unavailable", err)
	}
	if lifecycle != expected || outcome != expected || mongoCASNativeCount(t, db, "answersheets", "legacy_submission_evidence") != expected || mongoCASNativeCount(t, db, "report_generations", "historical_generated_evidence") != expected {
		t.Fatal("actual committed/rolled-back dedicated slots do not match the original four references")
	}
	var original int
	if err := sqlDB.Raw("SELECT COUNT(*) FROM domain_event_outbox").Row().Scan(&original); err != nil || original != 2 {
		t.Fatal("spool altered or removed the original SQL source")
	}
	originalMongo, err := db.Collection("domain_event_outbox").CountDocuments(t.Context(), bson.D{})
	if err != nil || originalMongo != 2 {
		t.Fatal("spool altered or removed the original Mongo source")
	}
}

func TestHistoricalCASSpoolNativeTwoPagesCommitAndFinalRawReadback(t *testing.T) {
	sqlDB, _, db, config, session := historicalSpoolNativeFixture(t)
	spool, copies := historicalSpoolNativePrepared(t, sqlDB, db, config, session)
	bounded, cancel, err := spool.InheritedContext(t.Context())
	if err != nil {
		t.Fatal("original inherited TTL rejected", err)
	}
	defer cancel()
	applied, err := historicalSpoolNativeApply(t, bounded, sqlDB, db, session, spool, false)
	if err != nil || applied == nil {
		t.Fatal("actual host dual Commit/Apply failed", err)
	}
	if err = historicalSpoolNativeReadback(t, bounded, sqlDB, db, config, session, spool, applied, copies); err != nil {
		t.Fatal("actual third-epoch final raw readback rejected", err)
	}
	historicalSpoolNativeCounts(t, sqlDB, db, true)
	t.Log("actual_native_four_original_types=true actual_pages=2 actual_host_dual_commit_responses=true independent_final_raw_readback=true atomic_cross_database_commit=false production_authority=false drop_ready=false")
}

func TestHistoricalCASSpoolNativeRollbackOrRawDriftNeverCompletes(t *testing.T) {
	for _, mode := range []string{"rollback", "ordinary_raw_drift"} {
		t.Run(mode, func(t *testing.T) {
			sqlDB, _, db, config, session := historicalSpoolNativeFixture(t)
			spool, copies := historicalSpoolNativePrepared(t, sqlDB, db, config, session)
			bounded, cancel, err := spool.InheritedContext(t.Context())
			if err != nil {
				t.Fatal("original inherited TTL rejected", err)
			}
			defer cancel()
			applied, err := historicalSpoolNativeApply(t, bounded, sqlDB, db, session, spool, mode == "rollback")
			if err != nil || applied == nil {
				t.Fatal("actual host dual lifecycle failed", err)
			}
			if mode == "ordinary_raw_drift" {
				changed := sqlDB.Exec("UPDATE assessment SET version=version+1 WHERE id=42")
				if changed.Error != nil || changed.RowsAffected != 1 {
					t.Fatal("actual non-evidence drift fixture ineffective", changed.Error)
				}
			}
			if err = historicalSpoolNativeReadback(t, bounded, sqlDB, db, config, session, spool, applied, copies); err == nil {
				t.Fatal("rollback or changed ordinary business bytes acquired completed readback")
			}
			historicalSpoolNativeCounts(t, sqlDB, db, mode != "rollback")
		})
	}
}
