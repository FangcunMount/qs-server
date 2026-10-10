//go:build integration

package retirement

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
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
		return e
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
		return e
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
