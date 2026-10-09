//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"gorm.io/gorm"
)

// Every lifecycle call in this file belongs to the native TEST HOST. The
// components borrow actual transactions/sessions and never create or end them.
// The fixture covers one original Outcome event and three genuine empty old
// sources, not six-type retirement, external qs-ai closure, or write authority.
func persistenceJointNativeEpoch(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, config MongoOwnerConfig, session mongo.Session, fn func(context.Context, *SQLResponsibilitySnapshot, *MongoResponsibilitySnapshot, *gorm.DB) error) (err error) {
	t.Helper()
	if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); err != nil {
		return err
	}
	tx := sqlDB.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if tx.Error != nil {
		return errors.Join(tx.Error, session.AbortTransaction(t.Context()))
	}
	// Defer also covers testing.Goexit from the existing native fixture readers.
	defer func() { err = errors.Join(err, tx.Rollback().Error, session.AbortTransaction(t.Context())) }()
	mongoCtx := mongo.NewSessionContext(t.Context(), session)
	ctx := mongo.NewSessionContext(hostmysql.WithTx(mongoCtx, tx), session)
	var uuid, database string
	err = tx.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&uuid, &database)
	if err == nil {
		var current *SQLResponsibilitySnapshot
		current, err = PrepareSQLResponsibilitySnapshot(ctx, mongoOwnerHashParts("mysql_database_identity_v1", uuid, database), sqlevaluation.DefaultSQLResponsibilityLimits())
		if err == nil {
			var global *MongoResponsibilitySnapshot
			global, err = PrepareMongoResponsibilitySnapshot(ctx, db, config, mongoCycleTestLimits())
			if err == nil {
				err = fn(ctx, current, global, tx)
			}
		}
	}
	// Both actual read epochs end before returning, including negative cases.
	return err
}

type persistenceJointNativePage struct {
	coordinator *HistoricalCoordinator
	joint       *WholeSourceJointPage
	source      *VerifiedSourceEvent
	origin      *SourceOriginEpoch
	ai          *AIReverseSnapshot
}

func persistenceJointNativePreparePage(t *testing.T, ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, f authFixture) (*persistenceJointNativePage, error) {
	t.Helper()
	c, err := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		return nil, err
	}
	binding, err := c.BindOriginCopies(ctx, f.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		return nil, err
	}
	origin, err := PrepareSourceOriginSnapshotEpoch(ctx, binding, current, global, originTestReaders(f))
	if err != nil {
		return nil, err
	}
	index, err := c.PrepareWholeSourceJointIndex(ctx, wholeJointCopies(f), DefaultWholeSourceJointLimits())
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
	if err = c.BindAIReverseSourceScope(ctx, ai, f.inputs()); err != nil {
		return nil, err
	}
	aiReport := ai.Summary()
	if !aiReport.WholeLedgerEOF || len(aiReport.Ledgers) != 14 || aiReport.Unknown != 0 || aiReport.Blocking != 0 || aiReport.GlobalReverseQualified || aiReport.CASAuthority || aiReport.DropReady {
		return nil, ErrHistoricalCASPersistence
	}
	page, err := c.NextPage(ctx)
	if err != nil {
		return nil, err
	}
	sources, err := page.Events()
	if err != nil || len(sources) != 1 {
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
	local, err := joint.Candidate(ctx, sources[0])
	if err != nil {
		return nil, err
	}
	// This DTO is observed, never edited or used as a substitute capability.
	if !local.LocalQualified || len(local.BlockingReasons) != 0 || local.OwnerObject != "evaluation_outcome" || local.OwnerID != "9001" || local.ActualOriginalRun == nil || local.ActualOriginalRun.RunID != "42:1" || local.ActualOriginalRun.Attempt != 1 {
		return nil, ErrHistoricalCASPersistence
	}
	if err = c.QualifyWholeSourceJointPage(ctx, page, joint); err != nil {
		return nil, err
	}
	if next, eof := c.NextPage(ctx); next != nil || eof != io.EOF {
		return nil, ErrHistoricalCASPersistence
	}
	receipt := c.Receipt()
	if !receipt.SourceCoverageComplete || receipt.ConsumedRecords != [4]uint64{1, 0, 0, 0} || receipt.CASComplete || receipt.BusinessClosureVerified || receipt.DropReady {
		return nil, ErrHistoricalCASPersistence
	}
	return &persistenceJointNativePage{coordinator: c, joint: joint, source: sources[0], origin: origin, ai: ai}, nil
}

func persistenceJointNativeAttachment(t *testing.T, ctx context.Context, p *persistenceJointNativePage) (sqlevaluation.SQLHistoricalBatchAttachment, error) {
	t.Helper()
	facts, err := p.source.Facts()
	if err != nil {
		return sqlevaluation.SQLHistoricalBatchAttachment{}, err
	}
	local, err := p.joint.Candidate(ctx, p.source)
	if err != nil {
		return sqlevaluation.SQLHistoricalBatchAttachment{}, err
	}
	// The original Run is authenticated by the actual source + exact actual Run
	// graph, never supplied by a latest/default selector or forged source DTO.
	if facts.OutcomeCommitted == nil || facts.OutcomeCommitted.EvaluationRunID != "42:1" || local.ActualOriginalRun == nil {
		return sqlevaluation.SQLHistoricalBatchAttachment{}, ErrHistoricalCASPersistence
	}
	run := *local.ActualOriginalRun
	binding, err := sqlevaluation.SQLHistoricalBatchBinding(ctx, p.joint.sql.facts, 42, 9001, facts.EventType, &run)
	if err != nil {
		return sqlevaluation.SQLHistoricalBatchAttachment{}, err
	}
	entry := evidence.HistoricalReferenceEntryV1{
		EventID: facts.EventID, EventType: facts.EventType, Source: facts.Source, Run: &run,
		Proof: &evidence.EventEvidenceV1{Version: 1, Class: evidence.Unverifiable, EventID: facts.EventID, Digest: facts.Source.Digest, BusinessBindingSHA256: binding, Origin: "retirement",
			Verification: evidence.Verification{Method: "native-joint-local-persistence-only", Version: "v1", OperationID: coordinatorBinding().OperationID,
				Reason:     "native local owner and current-ledger observations; independent approval, external qs-ai closure, writer fence and host commit response remain unproved",
				VerifiedAt: time.Now().UTC().Truncate(time.Millisecond), BusinessTerminal: local.LocalQualified, OwnershipVerified: local.LocalQualified, ResponsibilityClosed: local.LocalQualified}},
	}
	// Entry is strictly validated write INPUT. It cannot create joint/origin/AI
	// authority: those were independently produced above from actual native I/O.
	if err = entry.Validate(); err != nil {
		return sqlevaluation.SQLHistoricalBatchAttachment{}, err
	}
	return sqlevaluation.SQLHistoricalBatchAttachment{AssessmentID: 42, OutcomeID: 9001, Entry: entry, ContentDigest: facts.ContentDigest}, nil
}

func persistenceJointNativeSeal(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, config MongoOwnerConfig, session mongo.Session) (*HistoricalCASPersistencePage, *sqlevaluation.SQLHistoricalBatchCASPlan, *VerifiedSourceEvent, error) {
	t.Helper()
	var f authFixture
	var anchor *AIReverseRecheckAnchor
	err := persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
		f = originNativeCopies(t, ctx, tx, global)
		page, e := persistenceJointNativePreparePage(t, ctx, current, global, f)
		if e != nil {
			return e
		}
		anchor, e = page.ai.FreezeFreshAnchor(ctx, page.origin)
		return e
	})
	if err != nil {
		return nil, nil, nil, err
	}
	// Only the graphless real capability survives the ended first native scopes.
	var sealed *HistoricalCASPersistencePage
	var plan *sqlevaluation.SQLHistoricalBatchCASPlan
	var source *VerifiedSourceEvent
	err = persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
		page, e := persistenceJointNativePreparePage(t, ctx, current, global, f)
		if e != nil {
			return e
		}
		freshAI, e := anchor.RecheckFresh(ctx, current, 99, page.coordinator, f.inputs())
		if e != nil {
			return e
		}
		freshOrigin, e := anchor.RecheckOrigin(ctx, freshAI, current, global, page.coordinator, originTestReaders(f))
		if e != nil {
			return e
		}
		attachment, e := persistenceJointNativeAttachment(t, ctx, page)
		if e != nil {
			return e
		}
		plan, e = sqlevaluation.PrepareSQLHistoricalBatchCAS(ctx, page.joint.sql.facts, []sqlevaluation.SQLHistoricalBatchAttachment{attachment})
		if e != nil {
			return e
		}
		sealed, e = page.coordinator.SealHistoricalCASPersistencePage(ctx, page.joint, freshOrigin, freshAI, plan, nil)
		if e == nil {
			source = page.source
		}
		return e
	})
	return sealed, plan, source, err
}

func persistenceJointNativeReadback(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, config MongoOwnerConfig, session mongo.Session, source *VerifiedSourceEvent, applied *HistoricalCASAppliedPage) (*HistoricalCASPersistenceObservation, error) {
	t.Helper()
	var observed *HistoricalCASPersistenceObservation
	err := persistenceJointNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, current *SQLResponsibilitySnapshot, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
		request, e := MongoHistoricalSQLBatchSelectors([]*VerifiedSourceEvent{source})
		if e != nil {
			return e
		}
		sqlFresh, e := PrepareSQLBusinessOwnerBatch(ctx, current, request, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		mongoFresh, e := PrepareMongoHistoricalOwnerBatch(ctx, global, sqlFresh.facts, []*VerifiedSourceEvent{source}, DefaultMongoHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		observed, e = applied.VerifyPersisted(ctx, sqlFresh, mongoFresh)
		return e
	})
	return observed, err
}

func persistenceJointNativeLimitedReport(t *testing.T, observed *HistoricalCASPersistenceObservation) {
	t.Helper()
	if observed == nil {
		t.Fatal("missing actual independently observed persistence")
	}
	r := observed.Report()
	if r.ObservedReferences != 1 || !r.OriginalReadEpochEnded || !r.ApplyEpochEnded || !r.IndependentRawReadbackMatched || !r.WholeFourSourceAuthenticationObserved || r.HostCommitResponseVerified || r.WholeRetirementPersistenceComplete || r.AICommandPersistenceComplete || r.BusinessClosureVerified || r.CASAuthorized || r.DropReady || len(r.Required) == 0 || !evidence.ValidSHA256(r.SQLExpectedRowsSHA256) || !evidence.ValidSHA256(r.MongoExpectedRowsSHA256) {
		t.Fatal("limited actual persistence observation was absent or upgraded into authority")
	}
}

func TestHistoricalCASPersistenceNativeRealJointOriginAIFreshAndThirdEpoch(t *testing.T) {
	for _, mode := range []string{"commit", "rollback", "active_write", "baseline_drift"} {
		t.Run(mode, func(t *testing.T) {
			sqlDB, client, db, config := originNativeDBs(t, true)
			mongoBatchNativeIndexes(t, db)
			at := mongoLocalSheet().FilledAt
			evt := event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: event.BaseEvent{ID: "native-persistence-original-outcome", EventTypeValue: "evaluation.outcome.committed", AggregateTypeValue: "Evaluation", AggregateIDValue: "42", OccurredAtValue: at}, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "9001", EvaluationRunID: "42:1", CommittedAt: at}}
			body, err := domainwire.EncodeEvent(evt)
			if err != nil {
				t.Fatal(err)
			}
			row := fixtureSQLRow(t, body, "1")
			row[5] = []byte("7")
			originNativeInsertSQL(t, sqlDB, "domain_event_outbox", row)
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(t.Context())
			sealed, plan, source, err := persistenceJointNativeSeal(t, sqlDB, db, config, session)
			if err != nil {
				t.Fatal("actual four-source/joint/graphless fresh seal rejected", err)
			}
			var applied *HistoricalCASAppliedPage
			rollback := errors.New("native test host intentionally rolls back")
			err = sqlDB.Transaction(func(tx *gorm.DB) error {
				ctx := hostmysql.WithTx(t.Context(), tx)
				statement, e := plan.Apply(ctx)
				if e != nil {
					return e
				}
				applied, e = sealed.BindApplied(ctx, statement, nil)
				if e != nil {
					return e
				}
				if mode == "active_write" {
					other, e := client.StartSession()
					if e != nil {
						return e
					}
					observed, readErr := persistenceJointNativeReadback(t, sqlDB, db, config, other, source, applied)
					other.EndSession(t.Context())
					if readErr == nil || observed != nil {
						return errors.New("active actual write epoch was certified as persisted")
					}
					var alive int
					if e = tx.Raw("SELECT 1").Row().Scan(&alive); e != nil || alive != 1 {
						return errors.New("readback consumed host-owned active writer")
					}
				}
				if mode == "rollback" {
					return rollback
				}
				return nil
			}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
			if mode == "rollback" {
				if !errors.Is(err, rollback) {
					t.Fatal("host rollback failed", err)
				}
				observed, readErr := persistenceJointNativeReadback(t, sqlDB, db, config, session, source, applied)
				if readErr == nil || observed != nil {
					t.Fatal("rollback became an independently persisted page")
				}
			} else {
				if err != nil {
					t.Fatal("host-owned actual write/commit failed", err)
				}
				observed, readErr := persistenceJointNativeReadback(t, sqlDB, db, config, session, source, applied)
				if readErr != nil {
					t.Fatal("genuine third epoch exact readback rejected", readErr)
				}
				persistenceJointNativeLimitedReport(t, observed)
			}
			var preserved int
			condition := "historical_committed_evidence IS NULL"
			if mode != "rollback" {
				condition = "JSON_LENGTH(JSON_EXTRACT(historical_committed_evidence,'$.entries'))=1"
			}
			if err = sqlDB.Raw("SELECT COUNT(*) FROM evaluation_outcome WHERE id=9001 AND committed_event_id IS NULL AND committed_event_evidence IS NULL AND " + condition).Row().Scan(&preserved); err != nil || preserved != 1 {
				t.Fatal("standard single-slot/physical NULL or host rollback was changed", err)
			}
			if mode == "baseline_drift" {
				changed := sqlDB.Exec("UPDATE assessment SET version=version+1 WHERE id=42")
				if changed.Error != nil || changed.RowsAffected != 1 {
					t.Fatal("actual non-evidence baseline drift fixture was ineffective", changed.Error)
				}
				observed, readErr := persistenceJointNativeReadback(t, sqlDB, db, config, session, source, applied)
				if readErr == nil || observed != nil {
					t.Fatal("fresh raw non-evidence drift was hidden by successful Apply")
				}
			}
			t.Log("genuine_four_source_and_joint_origin_ai_capabilities=true original_epochs_ended=true limited_sql_persistence_only=true production_cas_authority=false drop_ready=false host_commit_response_verified=false")
		})
	}
}
