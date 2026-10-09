//go:build integration

package retirement

import (
	"context"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"gorm.io/gorm"
)

func TestCoordinatorOriginNativeSnapshotAdaptersWithActualIndependentTransactions(t *testing.T) {
	db, client, mgo, config := originNativeDBs(t, false)
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	var epoch *SourceOriginEpoch
	var fixture authFixture
	err = originNativeEpoch(t, db, mgo, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
		fixture = originNativeCopies(t, ctx, tx, global)
		c, e := PrepareHistoricalCoordinator(ctx, coordinatorBinding(), fixture.inputs(), DefaultHistoricalCoordinatorLimits())
		if e != nil {
			return e
		}
		binding, e := c.BindOriginCopies(ctx, fixture.inputs(), DefaultSourceOriginLimits())
		if e != nil {
			return e
		}
		epoch, e = PrepareSourceOriginSnapshotEpoch(ctx, binding, &SQLResponsibilitySnapshot{cycle: cycle}, global, originTestReaders(fixture))
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	err = originNativeEpoch(t, db, mgo, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
		proof, e := epoch.RecheckSnapshots(ctx, &SQLResponsibilitySnapshot{cycle: cycle}, global, originTestReaders(fixture))
		if e != nil {
			return e
		}
		r := proof.Report()
		if !r.ActualOriginMatched || !r.IndependentEpochRechecked || r.DropReady || r.CASAuthorized || !r.IndependentApprovalRequired || !r.WriterFenceRequired {
			t.Fatal("unearned authority")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
