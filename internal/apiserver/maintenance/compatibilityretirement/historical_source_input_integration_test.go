//go:build integration

package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func historicalSourceInputNativeEpoch(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, cfg MongoOwnerConfig, s mongo.Session, fn func(context.Context, *SQLResponsibilitySnapshot, *MongoSnapshotInputEpoch, *gorm.DB) error) error {
	t.Helper()
	return sqlDB.Transaction(func(tx *gorm.DB) error {
		ctx := mongo.NewSessionContext(hostmysql.WithTx(t.Context(), tx), s)
		var uuid, name string
		if err := tx.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&uuid, &name); err != nil {
			return err
		}
		sqlInput, err := PrepareSQLResponsibilitySnapshot(ctx, mongoOwnerHashParts("mysql_database_identity_v1", uuid, name), sqlevaluation.DefaultSQLResponsibilityLimits())
		if err != nil {
			return err
		}
		mongoInput, err := PrepareMongoSnapshotInputEpoch(ctx, db, cfg, MongoSnapshotInputLimits{Scan: mongoCycleTestLimits(), MaxDuration: 2 * time.Minute}, snapshotInputNativeFile(t))
		if err != nil {
			return err
		}
		return fn(ctx, sqlInput, mongoInput, tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func TestHistoricalSourceInputNativeTwoFullDiskEpochsAndSourceDrift(t *testing.T) {
	sqlDB, client, db, cfg := originNativeDBs(t, false)
	firstSession := snapshotInputNativeSession(t, client)
	var recipe *HistoricalSourceInputRecipe
	var planning *WholeSourceJointIndex
	var first, second *HistoricalSourceInputEpoch
	firstFile := snapshotInputNativeFile(t)
	err := historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, firstSession, func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, tx *gorm.DB) error {
		fixture := originNativeCopies(t, ctx, tx, &MongoResponsibilitySnapshot{db: db, metadata: mongoInput.metadata})
		copies, e := VerifySourceCopies(ctx, fixture.inputs())
		if e != nil {
			return e
		}
		binding, e := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
		if e != nil {
			return e
		}
		binding.limits.PageRows = 2
		recipe, e = FreezeHistoricalSourceInputRecipe(ctx, binding)
		if e != nil {
			return e
		}
		// An input recipe carries no usable old Origin capability. It does not
		// inherit the old expiration, create a replacement cap, or authorize CAS.
		recipe.binding.started = recipe.binding.started.Add(-2 * time.Hour)
		planning, e = PrepareHistoricalSourceInputIndex(ctx, coordinatorBinding(), recipe, wholeJointCopies(fixture), DefaultWholeSourceJointLimits())
		if e != nil || planning.owner != nil || planning.auth != copies || planning.encodedSHA != recipe.binding.fileHashes {
			return errors.New("actual input planning index failed or imported coordinator")
		}
		first, e = PrepareHistoricalSourceInputEpoch(ctx, recipe, sqlInput, mongoInput, firstFile, time.Minute)
		if e != nil {
			return e
		}
		if first.Summary().CASAuthorized || first.Summary().DropReady || !first.Summary().CompleteInput || first.Summary().Sources[0].Records != 4 || first.Summary().Sources[1].Records != 8 || first.Summary().Sources[3].Records != 2 {
			return errors.New("four actual source inputs or authority mismatch")
		}
		if _, e = CompareIndependentHistoricalSourceInputs(ctx, first, first); e == nil {
			return errors.New("same input accepted as two epochs")
		}
		// The snapshot remains fixed while the harness independently writes a new
		// owned row. Live count differs from the SAME snapshot, not only a DTO.
		mongoCycleNativeSheet(t, db, 29999, "", "")
		pinned, e := db.Collection("answersheets").CountDocuments(ctx, bson.D{})
		if e != nil || pinned != 0 {
			return errors.New("native snapshot lost fixed time")
		}
		live, e := db.Collection("answersheets").CountDocuments(t.Context(), bson.D{})
		if e != nil || live != 1 {
			return errors.New("external view did not differ")
		}
		_, e = db.Collection("answersheets").DeleteMany(t.Context(), bson.D{})
		if e != nil {
			return e
		}
		return first.StopCapture(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.verifyFrozen(t.Context()) != nil {
		t.Fatal("actual ended SQL scope lost pure disk input")
	}
	secondSession := snapshotInputNativeSession(t, client)
	err = historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, secondSession, func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, _ *gorm.DB) error {
		var e error
		second, e = PrepareHistoricalSourceInputEpoch(ctx, recipe, sqlInput, mongoInput, snapshotInputNativeFile(t), time.Minute)
		if e != nil {
			return e
		}
		matched, e := CompareIndependentHistoricalSourceInputs(ctx, first, second)
		if e != nil {
			return e
		}
		events, e := planning.InputEvents(ctx, matched, []string{"origin-sql-1", "origin-mongo-1"})
		if e != nil || len(events) != 2 {
			return errors.New("actual matched live scope could not read original events")
		}
		for _, ids := range [][]string{{"origin-sql-1", "origin-sql-1"}, {"outside-approved-input"}} {
			if _, e = planning.InputEvents(ctx, matched, ids); e == nil {
				return errors.New("duplicate or out-of-source planning selection accepted")
			}
		}
		if e = second.StopCapture(ctx); e != nil {
			return e
		}
		if _, e = planning.InputEvents(ctx, matched, []string{"origin-sql-1"}); e == nil {
			return errors.New("stopped native scope issued source handles")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	firstSession.EndSession(t.Context())
	secondSession.EndSession(t.Context())
	pair, err := CompareIndependentHistoricalSourceInputs(t.Context(), first, second)
	if err != nil || !pair.Summary().TwoIndependentInputsMatched || pair.Summary().BusinessClosureVerified || pair.Summary().CASAuthorized || pair.Summary().DropReady {
		t.Fatal("actual independent pure inputs failed or forged authority", err)
	}
	if planning.ReleaseInputAuthentication(t.Context(), pair) != nil || planning.auth != nil || planning.inputIndexIntact(t.Context(), pair, false) != nil {
		t.Fatal("actual ended pair failed to release planning authentication")
	}
	if _, err = planning.InputEvents(t.Context(), pair, []string{"origin-sql-1"}); err == nil {
		t.Fatal("ended released planning index issued source handles")
	}
	// Same inode and size are insufficient. Every original frozen frame is read
	// and checked against its write-time SHA before pair comparison succeeds.
	raw := make([]byte, 1)
	if _, err = firstFile.ReadAt(raw, first.pages[0].Offset); err != nil {
		t.Fatal(err)
	}
	original := raw[0]
	raw[0] ^= 1
	if _, err = firstFile.WriteAt(raw, first.pages[0].Offset); err != nil {
		t.Fatal(err)
	}
	if _, err = CompareIndependentHistoricalSourceInputs(t.Context(), first, second); err == nil {
		t.Fatal("same-inode changed bytes accepted")
	}
	raw[0] = original
	if _, err = firstFile.WriteAt(raw, first.pages[0].Offset); err != nil {
		t.Fatal(err)
	}
	// A new source ID beyond the original upper is rejected rather than hidden
	// by a range or silently inserted into the frozen coverage.
	row := fixtureSQLRow(t, bytes.ReplaceAll(wireFixture(t, "evaluation.failed"), []byte("original-event-1"), []byte("unapproved-new-source")), "5")
	originNativeInsertSQL(t, sqlDB, "domain_event_outbox", row)
	thirdSession := snapshotInputNativeSession(t, client)
	err = historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, thirdSession, func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, _ *gorm.DB) error {
		epoch, e := PrepareHistoricalSourceInputEpoch(ctx, recipe, sqlInput, mongoInput, snapshotInputNativeFile(t), time.Minute)
		if e == nil || epoch != nil {
			return errors.New("new source beyond approved upper accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if pair.ReleaseCaptureIndex(t.Context()) != nil || !recipe.captureStopped || recipe.binding.copies.rows != nil || recipe.binding.copies.eventIDs != nil || recipe.binding.copies.pairs != nil || planning.auth != nil || len(planning.entries) != 6 || planning.inputIndexIntact(t.Context(), pair, false) != nil {
		t.Fatal("planning retained the old auth maps or lost its exact offset index")
	}
}

func TestHistoricalSourceInputNativeBudgetAndOriginalMetadataFailure(t *testing.T) {
	sqlDB, client, db, cfg := originNativeDBs(t, true)
	s := snapshotInputNativeSession(t, client)
	err := historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, s, func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, tx *gorm.DB) error {
		fixture := originNativeCopies(t, ctx, tx, &MongoResponsibilitySnapshot{db: db, metadata: mongoInput.metadata})
		copies, e := VerifySourceCopies(ctx, fixture.inputs())
		if e != nil {
			return e
		}
		b, e := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
		if e != nil {
			return e
		}
		r, e := FreezeHistoricalSourceInputRecipe(ctx, b)
		if e != nil {
			return e
		}
		epoch, e := PrepareHistoricalSourceInputEpoch(ctx, r, sqlInput, mongoInput, snapshotInputNativeFile(t), time.Nanosecond)
		if e == nil || epoch != nil {
			return errors.New("expired input budget returned completion")
		}
		// Driver/server errors are not replaced by a new time or complete flag.
		cmd := bson.D{{Key: "configureFailPoint", Value: "failCommand"}, {Key: "mode", Value: bson.D{{Key: "times", Value: 1}}}, {Key: "data", Value: bson.D{{Key: "failCommands", Value: bson.A{"aggregate"}}, {Key: "errorCode", Value: 286}}}}
		if e = db.Client().Database("admin").RunCommand(t.Context(), cmd).Err(); e != nil {
			return e
		}
		epoch, e = PrepareHistoricalSourceInputEpoch(ctx, r, sqlInput, mongoInput, snapshotInputNativeFile(t), time.Minute)
		if e == nil || epoch != nil {
			return errors.New("actual snapshot error returned completion")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalSourceInputNativeIndependentUnchangedSnapshotAndReusedUUID(t *testing.T) {
	sqlDB, client, db, cfg := originNativeDBs(t, true)
	firstSession := snapshotInputNativeSession(t, client)
	var recipe *HistoricalSourceInputRecipe
	var first, second *HistoricalSourceInputEpoch
	err := historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, firstSession, func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, tx *gorm.DB) error {
		fixture := originNativeCopies(t, ctx, tx, &MongoResponsibilitySnapshot{db: db, metadata: mongoInput.metadata})
		copies, e := VerifySourceCopies(ctx, fixture.inputs())
		if e != nil {
			return e
		}
		binding, e := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
		if e != nil {
			return e
		}
		recipe, e = FreezeHistoricalSourceInputRecipe(ctx, binding)
		if e != nil {
			return e
		}
		first, e = PrepareHistoricalSourceInputEpoch(ctx, recipe, sqlInput, mongoInput, snapshotInputNativeFile(t), time.Minute)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	// No Mongo writes, marker documents, timestamp retries or imported session
	// state occur between these two actual captures. Ending the first session
	// lets the driver reuse its server UUID in a NEW native session instance.
	firstSession.EndSession(t.Context())
	secondSession := snapshotInputNativeSession(t, client)
	err = historicalSourceInputNativeEpoch(t, sqlDB, db, cfg, secondSession, func(ctx context.Context, sqlInput *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, _ *gorm.DB) error {
		var e error
		second, e = PrepareHistoricalSourceInputEpoch(ctx, recipe, sqlInput, mongoInput, snapshotInputNativeFile(t), time.Minute)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	secondSession.EndSession(t.Context())
	equalTime := first.mongoTime == second.mongoTime
	reusedUUID := bytes.Equal(first.mongoSessionID, second.mongoSessionID)
	t.Logf("actual_independent_sessions=true snapshot_equal=%t pooled_uuid_reused=%t mongo_writes_between=0", equalTime, reusedUUID)
	if firstSession == secondSession || first.sqlConnection == second.sqlConnection || first.sqlCycleID == second.sqlCycleID || !reusedUUID {
		t.Fatal("native distinct transaction/session and real pooled UUID case not observed")
	}
	pair, err := CompareIndependentHistoricalSourceInputs(t.Context(), first, second)
	if err != nil || pair.ValidateFrozen(t.Context()) != nil || first.mongo.CompareFreshInput(t.Context(), second.mongo) != nil || pair.Summary().CASAuthorized || pair.Summary().DropReady {
		t.Fatal("independent unchanged inputs or real UUID reuse rejected", err)
	}
	// Controlled negative probes only change the private in-memory comparator
	// inputs; no server state or stored frame is rewritten to create freshness.
	second.mongoTime = first.mongoTime
	second.mongoTime.T--
	if _, err = CompareIndependentHistoricalSourceInputs(t.Context(), first, second); !errors.Is(err, ErrSourceOriginFresh) {
		t.Fatal("older source input time accepted", err)
	}
	second.mongoTime = second.mongo.snapshot
	actualTime := second.mongo.snapshot
	second.mongo.snapshot = first.mongo.snapshot
	second.mongo.snapshot.T--
	if first.mongo.CompareFreshInput(t.Context(), second.mongo) == nil {
		t.Fatal("older Mongo input time accepted")
	}
	second.mongo.snapshot = actualTime
	actualSession, actualSQL, actualCycle := second.mongoSession, second.sqlConnection, second.sqlCycleID
	second.mongoSession = first.mongoSession
	if _, err = CompareIndependentHistoricalSourceInputs(t.Context(), first, second); !errors.Is(err, ErrSourceOriginFresh) {
		t.Fatal("same native session accepted", err)
	}
	second.mongoSession, second.sqlConnection = actualSession, first.sqlConnection
	if _, err = CompareIndependentHistoricalSourceInputs(t.Context(), first, second); !errors.Is(err, ErrSourceOriginFresh) {
		t.Fatal("same SQL transaction accepted", err)
	}
	second.sqlConnection, second.sqlCycleID = actualSQL, first.sqlCycleID
	if _, err = CompareIndependentHistoricalSourceInputs(t.Context(), first, second); !errors.Is(err, ErrSourceOriginFresh) {
		t.Fatal("same SQL cycle accepted", err)
	}
	second.sqlCycleID = actualCycle
	if pair.ValidateFrozen(t.Context()) != nil {
		t.Fatal("restored native inputs failed")
	}
}

// The same owned source/SQL/Mongo fixture supplies all actual frames and both
// native read scopes. Pure planning never consumes an old joint or creates CAS.
func TestHistoricalSourceInputNativeOwnerComponentPlanner(t *testing.T) {
	sqlDB, client, db, config, _ := historicalSpoolNativeFixture(t)
	emptySheet := mongoLocalSheet()
	emptySheet.DomainID = meta.FromUint64(10043)
	insertMongoLocalSheet(t, db, emptySheet)
	submitted, err := mongoSubmissionPayload(emptySheet)
	if err != nil {
		t.Fatal(err)
	}
	emptyEvent := mongoBatchNativeEvent(t, submitted, "answersheet.submitted", "source-planner-actual-empty-sheet")
	raw, err := domainwire.EncodeEvent(emptyEvent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Collection("domain_event_outbox").InsertOne(t.Context(), setMongoField(fixtureMongoRow(t, raw, int64(3)), "org_id", int64(7))); err != nil {
		t.Fatal(err)
	}
	crossMongoNativeWire(t, sqlDB, emptyEvent, "held", "replayed", "source-planner-empty", nil)
	var recipe *HistoricalSourceInputRecipe
	var index *WholeSourceJointIndex
	var first, second *HistoricalSourceInputEpoch
	var pair *HistoricalSourceInputPair
	var components *HistoricalCASComponents
	if err = historicalSourceInputNativeEpoch(t, sqlDB, db, config, snapshotInputNativeSession(t, client), func(ctx context.Context, current *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, tx *gorm.DB) error {
		copies := originNativeCopies(t, ctx, tx, &MongoResponsibilitySnapshot{db: db, metadata: mongoInput.metadata})
		auth, e := VerifySourceCopies(ctx, copies.inputs())
		if e != nil {
			return e
		}
		bound, e := BindOriginCopies(ctx, auth, copies.inputs(), DefaultSourceOriginLimits())
		if e != nil {
			return e
		}
		recipe, e = FreezeHistoricalSourceInputRecipe(ctx, bound)
		if e != nil {
			return e
		}
		index, e = PrepareHistoricalSourceInputIndex(ctx, coordinatorBinding(), recipe, wholeJointCopies(copies), DefaultWholeSourceJointLimits())
		if e != nil {
			return e
		}
		first, e = PrepareHistoricalSourceInputEpoch(ctx, recipe, current, mongoInput, snapshotInputNativeFile(t), time.Minute)
		if e != nil {
			return e
		}
		return first.StopCapture(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(t.TempDir(), "source-owner-recipes-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := file.Close(); e != nil {
			t.Error(e)
		}
	})
	spool, err := sqlevaluation.NewSQLHistoricalCASSpool(file, 64<<20, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err = historicalSourceInputNativeEpoch(t, sqlDB, db, config, snapshotInputNativeSession(t, client), func(ctx context.Context, current *SQLResponsibilitySnapshot, mongoInput *MongoSnapshotInputEpoch, _ *gorm.DB) error {
		var e error
		second, e = PrepareHistoricalSourceInputEpoch(ctx, recipe, current, mongoInput, snapshotInputNativeFile(t), time.Minute)
		if e != nil {
			return e
		}
		pair, e = CompareIndependentHistoricalSourceInputs(ctx, first, second)
		if e != nil {
			return e
		}
		// Same physical page carries two real owners, including actual SQL absence.
		limits := DefaultHistoricalCASComponentLimits()
		limits.MaxSources = 5
		components, e = PlanHistoricalSourceOwnerComponents(ctx, pair, index, current, mongoInput, spool, limits)
		if e != nil {
			return e
		}
		if len(components.Components()) != 2 {
			return errors.New("actual same-page positive/absent owners did not plan separately")
		}
		var sourceCount, rowCount int
		for _, component := range components.Components() {
			for _, input := range component.Inputs() {
				sourceCount += len(input.sources)
				rowCount += len(input.rows)
				if input.inputPair != pair || input.mongoCAS != nil || input.mongoRead.snapshotOriginalInput != mongoInput || input.seal != input.digest() {
					return ErrHistoricalCASComponents
				}
				if e = input.sqlRecipe.RowDependencies(func(_ string, _ uint64, _ string, _ uint64, write bool) error {
					if write {
						return ErrHistoricalCASComponents
					}
					return nil
				}); e != nil {
					return e
				}
			}
		}
		if sourceCount != 5 || rowCount == 0 || components.ValidateInputSources(ctx, pair) != nil {
			return ErrHistoricalCASComponents
		}
		limits.MaxSources = 2
		if partial, e := PlanHistoricalSourceOwnerComponents(ctx, pair, index, current, mongoInput, spool, limits); partial != nil || !errors.Is(e, ErrWholeSourceJointBounds) {
			return errors.New("actual owner closure was cut to fit the physical page budget")
		}
		original := append([]string(nil), index.byAssessment[42]...)
		index.byAssessment[42] = nil
		if bad, e := PlanHistoricalSourceOwnerComponents(ctx, pair, index, current, mongoInput, spool, limits); bad != nil || e == nil {
			return errors.New("changed source owner links accepted")
		}
		index.byAssessment[42] = original
		if bad, e := PrepareHistoricalCASComponents(ctx, index, components.Components()[0].Inputs(), limits); bad != nil || e == nil {
			return errors.New("pure planning bypassed original coordinator authority")
		}
		if e = second.StopCapture(ctx); e != nil {
			return e
		}
		if bad, e := PlanHistoricalSourceOwnerComponents(ctx, pair, index, current, mongoInput, spool, limits); bad != nil || e == nil {
			return errors.New("ended actual capture produced more planning input")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if index.ReleaseInputAuthentication(t.Context(), pair) != nil || pair.ReleaseCaptureIndex(t.Context()) != nil || components.ValidateInputSources(t.Context(), pair) != nil {
		t.Fatal("released authentication lost immutable planned inputs")
	}
	firstInput := components.Components()[0].Inputs()[0]
	originalSHA := firstInput.rows[0].sha
	firstInput.rows[0].sha = "changed-private-row"
	if components.ValidateInputSources(t.Context(), pair) == nil {
		t.Fatal("changed component input remained sealed")
	}
	firstInput.rows[0].sha = originalSHA
	if components.ValidateInputSources(t.Context(), pair) != nil {
		t.Fatal("restored original input rejected")
	}
	t.Log("actual_two_capture_source_inputs=true positive_and_sql_absent_owner_components=2 all_related_ranges_preserved=true groups_nil=true cas_authority=false")
}
