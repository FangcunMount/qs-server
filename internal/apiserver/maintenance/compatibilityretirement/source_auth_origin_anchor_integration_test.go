//go:build integration

package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"gorm.io/gorm"
)

func TestSourceOriginAnchorNativeActualFreezeAndIndependentRecheck(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			sqlDB, client, db, cfg := originNativeDBs(t, empty)
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(t.Context())
			var fixture authFixture
			var anchor *FreshRecheckAnchor
			var original *SourceOriginEpoch
			var oldScope context.Context
			err = originNativeEpoch(t, sqlDB, db, cfg, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
				fixture = originNativeCopies(t, ctx, tx, global)
				copies, err := VerifySourceCopies(ctx, fixture.inputs())
				if err != nil {
					return err
				}
				binding, err := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
				if err != nil {
					return err
				}
				original, err = PrepareSourceOriginEpoch(ctx, binding, cycle, global, originTestReaders(fixture))
				if err != nil {
					return err
				}
				anchor, err = original.FreezeFreshRecheckAnchor(ctx)
				if err != nil {
					return err
				}
				oldScope = ctx
				if anchor.binding != original.binding || anchor.originalEpochHash != original.hash || !anchor.intact() || anchor.boundaries != original.boundaries || anchor.receipts != original.receipts {
					t.Fatal("anchor dropped original authenticated facts")
				}
				if bytes.Equal(anchor.transaction.session, original.transaction.session) && &anchor.transaction.session[0] == &original.transaction.session[0] {
					t.Fatal("mutable Lsid bytes were borrowed")
				}
				if proof, err := anchor.Recheck(ctx, cycle, global, originTestReaders(fixture)); err != ErrSourceOriginFresh || proof != nil {
					t.Fatal("same actual snapshot accepted", err)
				}
				// Likewise, a real new Mongo snapshot cannot compensate for
				// retaining the original borrowed SQL transaction.
				newSession, err := client.StartSession()
				if err != nil {
					return err
				}
				defer newSession.EndSession(t.Context())
				if err := newSession.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); err != nil {
					return err
				}
				nextCtx := mongo.NewSessionContext(ctx, newSession)
				newGlobal, err := PrepareMongoResponsibilitySnapshot(nextCtx, db, cfg, mongoCycleTestLimits())
				if err == nil {
					if bytes.Equal(newGlobal.txn.session, anchor.transaction.session) && newGlobal.txn.number == anchor.transaction.number {
						t.Fatal("test did not create an actual new Mongo snapshot")
					}
					if proof, recheckErr := anchor.Recheck(nextCtx, cycle, newGlobal, originTestReaders(fixture)); recheckErr != ErrSourceOriginFresh || proof != nil {
						t.Fatal("original actual SQL transaction accepted with new Mongo", recheckErr)
					}
				}
				abortErr := newSession.AbortTransaction(t.Context())
				if err != nil {
					return err
				}
				if abortErr != nil {
					return abortErr
				}
				// A different real SQL RRRO transaction cannot compensate for
				// retaining the original real Mongo session/transaction.
				return sqlDB.Transaction(func(newTx *gorm.DB) error {
					nextCtx := mongo.NewSessionContext(hostmysql.WithTx(ctx, newTx), session)
					nextCycle, err := sqlevaluation.PrepareSQLHistoricalResponsibilityCycle(nextCtx, cycle.Report().DatabaseIdentitySHA256, sqlevaluation.DefaultSQLResponsibilityLimits())
					if err != nil {
						return err
					}
					if newTx.Statement.ConnPool == anchor.sqlConnection || nextCycle.Report().CycleID == anchor.sqlCycleID {
						t.Fatal("test did not create an actual new SQL transaction")
					}
					if proof, err := anchor.Recheck(nextCtx, nextCycle, global, originTestReaders(fixture)); err != ErrSourceOriginFresh || proof != nil {
						t.Fatal("original actual Mongo transaction accepted with new SQL", err)
					}
					return nil
				}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
			})
			if err != nil {
				t.Fatal(err)
			}
			if a, err := original.FreezeFreshRecheckAnchor(oldScope); err == nil || a != nil {
				t.Fatal("ended host scope minted a new anchor")
			}
			original = nil
			err = originNativeEpoch(t, sqlDB, db, cfg, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
				proof, err := anchor.Recheck(ctx, cycle, global, originTestReaders(fixture))
				if err != nil {
					return err
				}
				r := proof.Report()
				if !r.ActualOriginMatched || !r.IndependentEpochRechecked || !r.SourceFilesMatched || r.FirstEpochSHA256 != anchor.originalEpochHash || r.BindingSHA256 != anchor.binding.hash || r.CASAuthorized || r.DropReady || r.BusinessClosureVerified || !r.IndependentApprovalRequired || !r.HostProcessBudgetRequired || !r.WriterFenceRequired || !r.FirstAuthMetadataContinuityUnproven {
					t.Fatal("anchor-based proof changed original facts or authority")
				}
				if anchor.transaction.number == global.txn.number || !bytes.Equal(anchor.transaction.session, global.txn.session) {
					t.Fatal("native test did not use actual new TxnNumber on same Lsid")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if sqlDB.Exec("SELECT 1").Error != nil || client.Ping(t.Context(), nil) != nil {
				t.Fatal("anchor took host pool/session lifecycle")
			}
		})
	}
}

func TestSourceOriginAnchorNativeFullSourceAndMetadataDriftRefused(t *testing.T) {
	for _, kind := range []string{"sql_null", "sql_delete", "sql_upper", "mongo_raw_order", "mongo_delete", "mongo_uuid", "file_truncated", "expiry", "anchor_tamper"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB, client, db, cfg := originNativeDBs(t, false)
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(t.Context())
			var fixture authFixture
			var anchor *FreshRecheckAnchor
			err = originNativeEpoch(t, sqlDB, db, cfg, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
				fixture = originNativeCopies(t, ctx, tx, global)
				copies, err := VerifySourceCopies(ctx, fixture.inputs())
				if err != nil {
					return err
				}
				binding, err := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
				if err != nil {
					return err
				}
				epoch, err := PrepareSourceOriginEpoch(ctx, binding, cycle, global, originTestReaders(fixture))
				if err != nil {
					return err
				}
				anchor, err = epoch.FreezeFreshRecheckAnchor(ctx)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "sql_null":
				err = sqlDB.Exec("UPDATE domain_event_outbox SET last_error='' WHERE id=1").Error
			case "sql_delete":
				err = sqlDB.Exec("DELETE FROM domain_event_outbox WHERE id=1").Error
			case "sql_upper":
				row := fixtureSQLRow(t, bytes.ReplaceAll(wireFixture(t, "evaluation.failed"), []byte("original-event-1"), []byte("origin-extra")), "5")
				originNativeInsertSQL(t, sqlDB, "domain_event_outbox", row)
			case "mongo_raw_order":
				var old bson.D
				if err = db.Collection("domain_event_outbox").FindOne(t.Context(), bson.D{{Key: "_id", Value: int64(1)}}).Decode(&old); err == nil {
					old[1], old[len(old)-1] = old[len(old)-1], old[1]
					_, err = db.Collection("domain_event_outbox").ReplaceOne(t.Context(), bson.D{{Key: "_id", Value: int64(1)}}, old)
				}
			case "mongo_delete":
				_, err = db.Collection("domain_event_outbox").DeleteOne(t.Context(), bson.D{{Key: "_id", Value: int64(1)}})
			case "mongo_uuid":
				err = db.Collection("domain_event_outbox").Drop(t.Context())
				if err == nil {
					err = db.CreateCollection(t.Context(), "domain_event_outbox")
				}
			case "file_truncated":
				fixture.raw[0] = fixture.raw[0][:len(fixture.raw[0])-1]
			case "expiry":
				anchor.binding.started = time.Now().Add(-2 * anchor.binding.limits.MaxDuration)
			case "anchor_tamper":
				anchor.sqlHead = "editable-report"
			}
			if err != nil {
				t.Fatal("owned fixture mutation failed", err)
			}
			err = originNativeEpoch(t, sqlDB, db, cfg, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
				proof, err := anchor.Recheck(ctx, cycle, global, originTestReaders(fixture))
				if proof != nil {
					t.Fatal("changed anchor minted proof")
				}
				return err
			})
			if err == nil {
				t.Fatal("changed physical source/metadata/lifetime accepted")
			}
		})
	}
}
