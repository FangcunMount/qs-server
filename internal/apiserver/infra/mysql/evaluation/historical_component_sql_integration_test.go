//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"gorm.io/gorm"
)

func componentSQLNativeRecipe(t *testing.T, db *gorm.DB, checkOriginal bool) *SQLHistoricalComponentRecipe {
	return componentSQLNativeRecipeOnSpool(t, db, checkOriginal, nil)
}

func componentSQLNativeRecipeOnSpool(t *testing.T, db *gorm.DB, checkOriginal bool, spool *SQLHistoricalCASSpool) *SQLHistoricalComponentRecipe {
	t.Helper()
	var recipe *SQLHistoricalComponentRecipe
	if err := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}, AnswerSheetIDs: []uint64{10042}}, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		catalog, err := PrepareSQLHistoricalCrossStoreCatalog(ctx, c, DefaultSQLCrossStoreLimits())
		if err != nil {
			return err
		}
		cross, err := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"old-original"}, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}}})
		if err != nil {
			return err
		}
		plan, err := PrepareSQLHistoricalBatchCAS(ctx, batch, []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, batch, 42, 0, "old-original", "evaluation.requested", nil)})
		if err != nil {
			return err
		}
		provenance, err := SealSQLHistoricalCASProvenance(ctx, plan, batch)
		if err != nil {
			return err
		}
		if spool == nil {
			recipe, err = FreezeSQLHistoricalComponentRecipe(ctx, batch, cross, provenance, nil)
		} else {
			recipe, err = FreezeSQLHistoricalComponentRecipeToSpool(ctx, batch, cross, provenance, nil, spool)
		}
		if err != nil {
			return err
		}
		if checkOriginal {
			o, e := PrepareSQLHistoricalComponentObservation(ctx, recipe, 20*time.Second, false)
			if e == nil || o != nil {
				return errors.New("original actual transaction accepted as fresh")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return recipe
}

func TestSQLHistoricalComponentNativeOwnedSpoolRejectsTampering(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	file, err := os.CreateTemp(t.TempDir(), "component-input-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	spool, err := NewSQLHistoricalCASSpool(file, 8<<20, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := componentSQLNativeRecipeOnSpool(t, db, true, spool)
	if r.plan != nil || r.input != nil || r.anchors != nil || r.responsibility.rows != nil || r.frame.Length <= 0 {
		t.Fatal("raw component images retained in the frozen recipe")
	}
	var rows int
	if err = r.RowDependencies(func(_ string, _ uint64, _ string, _ uint64, _ bool) error { rows++; return nil }); err != nil || rows != 2 {
		t.Fatal("actual original dependency lost in disk input", err, rows)
	}
	if err = componentSQLNativeObserve(t, db, r, false, nil); err != nil {
		t.Fatal("genuine spooled physical input rejected", err)
	}
	raw := []byte{0}
	if _, err = file.ReadAt(raw, r.frame.Offset); err != nil {
		t.Fatal(err)
	}
	raw[0] ^= 0xff
	if _, err = file.WriteAt(raw, r.frame.Offset); err != nil || file.Sync() != nil {
		t.Fatal("owned spool tamper setup failed", err)
	}
	if _, err = r.InputSHA256(); err == nil {
		t.Fatal("changed FD bytes admitted as the original input")
	}
	if err = componentSQLNativeObserve(t, db, r, false, nil); err == nil {
		t.Fatal("tampered original input admitted to a fresh physical read")
	}
}

func TestSQLHistoricalComponentNativeOriginalOwnerPartition(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	insertHistoricalAssessment(t, db, 43)
	file, err := os.CreateTemp(t.TempDir(), "owner-input-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	spool, err := NewSQLHistoricalCASSpool(file, 8<<20, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	var recipes []*SQLHistoricalComponentRecipe
	var unresolved *SQLHistoricalComponentRecipe
	if err = batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		batch, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42, 43}, AnswerSheetIDs: []uint64{10042, 10043}}, DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		catalog, e := PrepareSQLHistoricalCrossStoreCatalog(ctx, c, DefaultSQLCrossStoreLimits())
		if e != nil {
			return e
		}
		cross, e := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"owner-42", "owner-43"}, AssessmentIDs: []uint64{42, 43}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}, {Kind: "AnswerSheet", ID: "10043"}}})
		if e != nil {
			return e
		}
		plan, e := PrepareSQLHistoricalBatchCAS(ctx, batch, []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, batch, 42, 0, "owner-42", "evaluation.requested", nil), casNativeEntry(t, ctx, batch, 43, 0, "owner-43", "evaluation.requested", nil)})
		if e != nil {
			return e
		}
		provenance, e := SealSQLHistoricalCASProvenance(ctx, plan, batch)
		if e != nil {
			return e
		}
		recipes, e = FreezeSQLHistoricalOwnerComponentRecipes(ctx, batch, cross, provenance, nil, spool)
		if e != nil {
			return e
		}
		scoped, e := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"owner-42", "owner-43", "unbound-mongo-source"}, AssessmentIDs: []uint64{42, 43}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "ReportGeneration", ID: "77"}}})
		if e != nil {
			return e
		}
		pending, e := FreezeSQLHistoricalOwnerComponentRecipes(ctx, batch, scoped, provenance, nil, nil)
		if e != nil {
			return e
		}
		if len(pending) != 1 {
			return errors.New("unknown owner was guessed or discarded")
		}
		unresolved = pending[0]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(recipes) != 2 {
		t.Fatal("source packet joined unrelated genuine owners", len(recipes))
	}
	if unresolved == nil || unresolved.OwnerPartitionResolved() {
		t.Fatal("unbound Mongo pure input called resolved")
	}
	scope, e := unresolved.OriginalSelectors()
	if e != nil || len(scope.EventIDs) != 3 || len(scope.MongoOwners) != 1 || scope.MongoOwners[0].ID != "77" {
		t.Fatal("exact unresolved source/negative range discarded", e)
	}
	bindings, e := unresolved.SourceOwnerBindings()
	if e != nil || len(bindings) != 3 || bindings[2].SQLOwnerPresent || bindings[2].AssessmentID != 0 {
		t.Fatal("Mongo-only input acquired an invented SQL owner", e)
	}
	for i, r := range recipes {
		owners, e := r.OwnerIdentities()
		if e != nil || len(owners) != 1 || owners[0].AssessmentID != uint64(42+i) || !r.OwnerPartitionResolved() {
			t.Fatal("actual owner identity omitted", e, owners)
		}
		bound, e := r.SourceOwnerBindings()
		if e != nil || len(bound) != 1 || bound[0].AssessmentID != uint64(42+i) || !bound[0].SQLOwnerPresent {
			t.Fatal("actual source binding omitted", e)
		}
		events, e := r.SourceEventIDs()
		if e != nil || len(events) != 1 {
			t.Fatal("original source identity lost", e)
		}
		selectors, e := r.OriginalSelectors()
		if e != nil || len(selectors.MongoOwners) != 1 || len(selectors.AssessmentIDs) != 1 {
			t.Fatal("negative owner range lost", e)
		}
		var rows int
		if e = r.RowDependencies(func(table string, id uint64, _ string, _ uint64, _ bool) error {
			rows++
			if table == "assessment" && id != uint64(42+i) {
				return errors.New("other owner leaked into child")
			}
			return nil
		}); e != nil || rows != 2 {
			t.Fatal("exact owner read/write input omitted", e, rows)
		}
		if e = componentSQLNativeObserve(t, db, r, false, nil); e != nil {
			t.Fatal("actual fresh child read rejected", e)
		}
	}
}

func componentSQLNativeObserve(t *testing.T, db *gorm.DB, r *SQLHistoricalComponentRecipe, writable bool, fn func(context.Context, *SQLHistoricalComponentObservation) error) error {
	t.Helper()
	return db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		o, err := PrepareSQLHistoricalComponentObservation(ctx, r, 20*time.Second, writable)
		if err != nil {
			return err
		}
		if o.Report().DropReady || o.Report().HostCommitVerified || !o.Report().FullSourcesRequired {
			return errors.New("SQL observer invented whole authority")
		}
		if fn != nil {
			return fn(ctx, o)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: !writable})
}

func TestSQLHistoricalComponentNativeFreshNegativeClosureAndCrossOrganization(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	insertHistoricalAssessment(t, db, 43)
	r := componentSQLNativeRecipe(t, db, true)
	if err := componentSQLNativeObserve(t, db, r, false, nil); err != nil {
		t.Fatal("genuine fresh unchanged scope rejected", err)
	}
	// A valid unrelated owner's terminal message must not force a whole rescan.
	p := eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 43, TesteeID: 21, Reason: "synthetic", FailedAt: time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC)}
	cycleNativeInsert(t, db, cycleTestMessage(t, "other-owner", "evaluation.failed", "Evaluation", "43", p))
	for _, spec := range sqlResponsibilityTables {
		q, args, e := componentPredicate(spec, r.selectors, r.selectors.EventIDs)
		if e != nil {
			t.Fatal(e)
		}
		rows, _, _, e := cycleQuery(db, "SELECT * FROM `"+spec.name+"` WHERE "+q, 131072, args...)
		if e != nil || len(rows) != 0 {
			t.Fatalf("fixed selector store=%s matchingRows=%d error=%v", spec.name, len(rows), e)
		}
	}
	if err := componentSQLNativeObserve(t, db, r, false, nil); err != nil {
		t.Fatal("unrelated new message polluted exact scope", err)
	}
	if err := db.Exec("DELETE FROM rm_outbox").Error; err != nil {
		t.Fatal(err)
	}
	for _, org := range []int64{7, 8} {
		p.OrgID, p.AssessmentID = org, 42
		cycleNativeInsert(t, db, cycleTestMessage(t, "new-owner-responsibility", "evaluation.failed", "Evaluation", "42", p))
		if err := componentSQLNativeObserve(t, db, r, false, nil); err == nil {
			t.Fatal("new matching/cross-org responsibility ignored", org)
		}
		if err := db.Exec("DELETE FROM rm_outbox").Error; err != nil {
			t.Fatal(err)
		}
	}
	crossSQLNativeHeld(t, db, "new-sheet-held")
	if err := componentSQLNativeObserve(t, db, r, false, nil); err == nil {
		t.Fatal("new matching Mongo owner message ignored")
	}
	if err := db.Exec("DELETE FROM retry_event_hold").Error; err != nil {
		t.Fatal(err)
	}
	crossSQLNativeReplay(t, db, "old-original")
	if err := componentSQLNativeObserve(t, db, r, false, nil); err == nil {
		t.Fatal("new complete replay pair ignored")
	}
	if err := db.Exec("DELETE FROM qs_rm_replay_items").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM qs_rm_replay_requests").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at) VALUES('interpretation_run','new-related-run',1,42,'pending',UTC_TIMESTAMP(6))").Error; err != nil {
		t.Fatal(err)
	}
	if err := componentSQLNativeObserve(t, db, r, false, nil); err == nil {
		t.Fatal("new related business row ignored")
	}
}

func TestSQLHistoricalComponentNativeBoundedRWCommitReadbackAndRollback(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	r := componentSQLNativeRecipe(t, db, false)
	sentinel := errors.New("owned host rollback")
	var rolled *SQLHistoricalComponentStatement
	err := componentSQLNativeObserve(t, db, r, true, func(ctx context.Context, o *SQLHistoricalComponentObservation) error {
		var err error
		rolled, err = o.apply(ctx)
		if err != nil {
			return err
		}
		if _, err = o.apply(ctx); err == nil {
			return errors.New("used epoch replayed")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal("native rollback path failed", err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		_, e := rolled.VerifyIndependentPersisted(hostmysql.WithTx(t.Context(), tx), 20*time.Second)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err == nil {
		t.Fatal("rolled-back statement became committed readback")
	}
	var committed *SQLHistoricalComponentStatement
	if err = componentSQLNativeObserve(t, db, r, true, func(ctx context.Context, o *SQLHistoricalComponentObservation) error {
		var e error
		committed, e = o.apply(ctx)
		return e
	}); err != nil {
		t.Fatal("native commit failed", err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error {
		report, e := committed.VerifyIndependentPersisted(hostmysql.WithTx(t.Context(), tx), 20*time.Second)
		if e == nil && (!report.IndependentPersistedReadMatched || report.HostCommitVerified || report.DropReady || !report.MongoQualificationRequired) {
			return errors.New("paired commit authority invented")
		}
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
		t.Fatal("independent actual server readback failed", err)
	}
}

func TestSQLHistoricalComponentNativeExpiryAndReplayWholePair(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	crossSQLNativeReplay(t, db, "old-original")
	r := componentSQLNativeRecipe(t, db, false)
	if len(r.responsibility.rows["qs_rm_replay_items"]) != 2 || len(r.responsibility.rows["qs_rm_replay_requests"]) != 1 {
		t.Fatal("whole replay pair omitted")
	}
	if err := componentSQLNativeObserve(t, db, r, false, func(ctx context.Context, o *SQLHistoricalComponentObservation) error {
		o.expires = time.Now().Add(-time.Second)
		o.seal = o.digest()
		if o.live(ctx) == nil {
			return errors.New("expired actual epoch accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO qs_rm_replay_items(org_id,request_id,ordinal,event_id,expected_failure_count,authorized,reason) VALUES(7,?,2,'new-negative-member',0,0,'not_found')", []byte("native-cross-replay")).Error; err != nil {
		t.Fatal(err)
	}
	if err := componentSQLNativeObserve(t, db, r, false, nil); err == nil {
		t.Fatal("new member outside original event selector hidden")
	}
}
