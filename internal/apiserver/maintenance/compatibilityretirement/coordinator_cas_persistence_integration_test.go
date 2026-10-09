//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"gorm.io/gorm"
)

func TestHistoricalCASPersistenceNativeActualMongoEpochStateAndSameLsidNewTxn(t *testing.T) {
	client, db, config := mongoCycleNativeDB(t)
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.EndSession(context.Background()) })
	// The native test host alone starts/aborts these actual snapshot scopes.
	active := false
	t.Cleanup(func() {
		if active {
			if e := session.AbortTransaction(context.Background()); e != nil {
				t.Error("native test host abort cleanup", e)
			}
		}
	})
	start := func() (*MongoResponsibilitySnapshot, mongo.SessionContext) {
		t.Helper()
		if e := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); e != nil {
			t.Fatal(e)
		}
		active = true
		ctx := mongo.NewSessionContext(t.Context(), session)
		global, e := PrepareMongoResponsibilitySnapshot(ctx, db, config, mongoCycleTestLimits())
		if e != nil {
			t.Fatal("actual UUID/schema/full-collection snapshot", e)
		}
		if e = global.ValidateBorrowedSnapshot(ctx); e != nil {
			t.Fatal(e)
		}
		return global, ctx
	}
	first, firstCtx := start()
	if err = casPersistenceMongoEpochEnded(session, first.txn); !errors.Is(err, ErrHistoricalCASPersistence) {
		t.Fatal("active original actual snapshot was treated as ended", err)
	}
	if err = first.ValidateBorrowedSnapshot(firstCtx); err != nil {
		t.Fatal("epoch check closed or changed borrowed session", err)
	}
	if err = session.AbortTransaction(t.Context()); err != nil {
		t.Fatal(err)
	}
	active = false
	if err = casPersistenceMongoEpochEnded(session, first.txn); err != nil {
		t.Fatal("actual ended old snapshot not recognized", err)
	}
	second, secondCtx := start()
	if string(second.txn.session) != string(first.txn.session) || second.txn.number <= first.txn.number {
		t.Fatal("native fixture did not use the same actual Lsid with a newer actual TxnNumber")
	}
	if err = casPersistenceMongoEpochEnded(session, first.txn); err != nil {
		t.Fatal("same actual Session/new snapshot was incorrectly rejected", err)
	}
	if err = casPersistenceMongoEpochEnded(session, second.txn); !errors.Is(err, ErrHistoricalCASPersistence) {
		t.Fatal("current actual snapshot was incorrectly declared ended", err)
	}
	if err = second.ValidateBorrowedSnapshot(secondCtx); err != nil {
		t.Fatal("component ended current borrowed epoch", err)
	}
	if err = session.AbortTransaction(t.Context()); err != nil {
		t.Fatal(err)
	}
	active = false
	if err = casPersistenceMongoEpochEnded(session, second.txn); err != nil {
		t.Fatal(err)
	}
	// Ended state is observed; no successful Commit response was supplied and
	// no HistoricalCASAppliedPage/persisted closure capability has been minted.
	if r := (*HistoricalCASPersistenceObservation)(nil).Report(); r.HostCommitResponseVerified || r.WholeRetirementPersistenceComplete || r.CASAuthorized || r.DropReady {
		t.Fatal("native transaction state became authority")
	}
}

func TestHistoricalCASPersistenceNativeRealCompleteCopyAndBusinessPageRemainUnqualified(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	mongoBatchNativeIndexes(t, db)
	at := mongoBatchNativeAssessmentSheet().FilledAt
	requested := eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, QuestionnaireCode: "Q", QuestionnaireVer: "1.0", AnswerSheetID: "10042", ModelKind: "scale", ModelCode: "M", ModelVersion: "1.0", RequestedAt: at}
	fixture := coordinatorNativeFixtureCopies(t, []event.DomainEvent{mongoBatchNativeEvent(t, requested, "evaluation.requested", "native-persistence-unqualified-request")}, nil)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), fixture.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.NextPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	handles, err := page.Events()
	if err != nil || len(handles) != 1 {
		t.Fatal("actual complete source frame was not selected", err)
	}
	var uuid, database string
	if err = sqlDB.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&uuid, &database); err != nil {
		t.Fatal(err)
	}
	expected := mongoOwnerHashParts("mysql_database_identity_v1", uuid, database)
	var actualPool gorm.ConnPool
	err = mongoCycleNativeTx(t, client, func(mongoCtx mongo.SessionContext) error {
		return sqlDB.Transaction(func(tx *gorm.DB) error {
			ctx := mongo.NewSessionContext(hostmysql.WithTx(mongoCtx, tx), mongo.SessionFromContext(mongoCtx))
			current, e := PrepareSQLResponsibilitySnapshot(ctx, expected, sqlevaluation.DefaultSQLResponsibilityLimits())
			if e != nil {
				return e
			}
			selectors, e := MongoHistoricalSQLBatchSelectors(handles)
			if e != nil {
				return e
			}
			sqlBatch, e := PrepareSQLBusinessOwnerBatch(ctx, current, selectors, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			global, e := PrepareMongoResponsibilitySnapshot(ctx, db, config, mongoCycleTestLimits())
			if e != nil {
				return e
			}
			mongoBatch, e := PrepareMongoHistoricalOwnerBatch(ctx, global, sqlBatch.facts, handles, DefaultMongoHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			if e = c.QualifyPage(ctx, page, sqlBatch, mongoBatch); e != nil {
				return e
			}
			if next, e := c.NextPage(ctx); next != nil || e != io.EOF {
				t.Fatal("real second whole-source EOF not reached", e)
			}
			if _, e = c.CandidateRange(0, 128); e != nil {
				return e
			}
			if plan, e := c.PrepareSQLCAS(ctx, sqlBatch); plan != nil || !errors.Is(e, ErrCoordinatorSQLCASUnqualified) {
				t.Fatal("real copy coverage/SQL facts bypassed source approval/whole-writer gate", e)
			}
			if plan, e := c.PrepareMongoCAS(ctx, mongoBatch); plan != nil || !errors.Is(e, ErrCoordinatorMongoCASUnqualified) {
				t.Fatal("real copy coverage/Mongo facts bypassed remaining production authority", e)
			}
			// No fake joint/origin/AI handle is constructed to reach a positive
			// Seal. Authentic copy EOF and actual local facts are insufficient.
			if seal, e := c.SealHistoricalCASPersistencePage(ctx, nil, nil, nil, nil, nil); seal != nil || !errors.Is(e, ErrHistoricalCASPersistence) {
				t.Fatal("missing actual joint/origin/AI/plan instances became a persistence page", e)
			}
			actualPool = tx.Statement.ConnPool
			if e = casPersistenceSQLPoolEnded(ctx, actualPool); !errors.Is(e, ErrHistoricalCASPersistence) {
				t.Fatal("active actual RRRO was treated as ended", e)
			}
			return sqlBatch.ValidateBorrowedSnapshot(ctx)
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = casPersistenceSQLPoolEnded(t.Context(), actualPool); err != nil {
		t.Fatal("host-ended actual SQL epoch not recognized", err)
	}
	if sqlReady, mongoReady := c.SQLCASReadiness(), c.MongoCASReadiness(); sqlReady.CASAuthorized || sqlReady.DropReady || mongoReady.CASAuthorized || mongoReady.DropReady {
		t.Fatal("readonly native completion upgraded production capability")
	}
}
