//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
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

func TestSQLHistoricalComponentNativeSourceOwnerSelectors(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	insertHistoricalAssessment(t, db, 43)
	if err := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42, 43}, AnswerSheetIDs: []uint64{10042, 10043}}, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		catalog, err := PrepareSQLHistoricalCrossStoreCatalog(ctx, c, DefaultSQLCrossStoreLimits())
		if err != nil {
			return err
		}
		cross, err := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"mongo-submitted", "mongo-generated"}, AssessmentIDs: []uint64{42, 43}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}, {Kind: "ReportGeneration", ID: "77"}}})
		if err != nil {
			return err
		}
		baseline, err := SealSQLHistoricalCASReadBaseline(ctx, batch)
		if err != nil {
			return err
		}
		selected := map[string]uint64{"mongo-submitted": 42, "mongo-generated": 43}
		recipes, err := FreezeSQLHistoricalOwnerComponentRecipes(ctx, batch, cross, nil, baseline, nil, selected)
		if err != nil || len(recipes) != 2 {
			return errors.New("actual Mongo source selectors did not partition captured SQL owners")
		}
		selected["mongo-generated"] = 42
		for i, recipe := range recipes {
			owners, e := recipe.OwnerIdentities()
			scope, se := recipe.OriginalSelectors()
			bindings, be := recipe.SourceOwnerBindings()
			if e != nil || se != nil || be != nil || len(owners) != 1 || owners[0].AssessmentID != uint64(42+i) || len(scope.EventIDs) != 1 || len(bindings) != 1 || bindings[0].SQLOwnerPresent || !recipe.OwnerPartitionResolved() {
				return errors.New("pure selector was mutable or promoted to authenticated source binding")
			}
			// A generation without a SQL-readable identity keeps its complete
			// original negative range as shared read-only input in both children.
			if !slices.Contains(scope.MongoOwners, SQLCrossStoreOwnerReference{Kind: "ReportGeneration", ID: "77"}) {
				return errors.New("generation negative responsibility range dropped")
			}
		}
		for _, invalid := range []map[string]uint64{{"not-original": 42}, {"mongo-generated": 99}, {"mongo-generated": 0}} {
			if _, e := FreezeSQLHistoricalOwnerComponentRecipes(ctx, batch, cross, nil, baseline, nil, invalid); e == nil {
				return errors.New("source or owner outside the actual original scope admitted")
			}
		}
		plan, err := PrepareSQLHistoricalBatchCAS(ctx, batch, []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, batch, 42, 0, "mongo-submitted", "evaluation.requested", nil)})
		if err != nil {
			return err
		}
		provenance, err := SealSQLHistoricalCASProvenance(ctx, plan, batch)
		if err != nil {
			return err
		}
		if _, e := FreezeSQLHistoricalOwnerComponentRecipes(ctx, batch, cross, provenance, nil, nil, map[string]uint64{"mongo-submitted": 43, "mongo-generated": 43}); e == nil {
			return errors.New("selector contradicted an actual original attachment")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoricalComponentNativeAbsentOwnerSelector(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	file, err := os.CreateTemp(t.TempDir(), "absent-owner-input-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	spool, err := NewSQLHistoricalCASSpool(file, 8<<20, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	var recipe *SQLHistoricalComponentRecipe
	if err = batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		batch, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}, AnswerSheetIDs: []uint64{10042, 10090, 10091}}, DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		catalog, e := PrepareSQLHistoricalCrossStoreCatalog(ctx, c, DefaultSQLCrossStoreLimits())
		if e != nil {
			return e
		}
		cross, e := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"sheet-90", "sheet-91"}, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}, {Kind: "AnswerSheet", ID: "10090"}, {Kind: "AnswerSheet", ID: "10091"}}})
		if e != nil {
			return e
		}
		recipe, e = FreezeSQLHistoricalAbsentOwnerSelectorRecipe(ctx, batch, cross, []string{"sheet-90"}, []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10090"}}, []uint64{10090}, spool)
		if e != nil {
			return e
		}
		for _, invalid := range []struct {
			event, sheet string
			id           uint64
		}{{"sheet-90", "10042", 10042}, {"outside", "10090", 10090}, {"sheet-90", "10092", 10092}, {"sheet-90", "10091", 10090}} {
			if _, e = FreezeSQLHistoricalAbsentOwnerSelectorRecipe(ctx, batch, cross, []string{invalid.event}, []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: invalid.sheet}}, []uint64{invalid.id}, nil); e == nil {
				return errors.New("nonempty or mismatched original empty range admitted")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	owners, e := recipe.OwnerIdentities()
	scope, se := recipe.OriginalSelectors()
	if e != nil || se != nil || len(owners) != 0 || len(scope.EventIDs) != 1 || scope.EventIDs[0] != "sheet-90" || len(scope.MongoOwners) != 1 || scope.MongoOwners[0].ID != "10090" || !recipe.OwnerPartitionResolved() {
		t.Fatal("actual SQL-empty subset was lost or called a Mongo qualification", e, se)
	}
	if e = componentSQLNativeObserve(t, db, recipe, false, nil); e != nil {
		t.Fatal("actual unchanged empty scope rejected", e)
	}
	insertHistoricalAssessment(t, db, 90)
	if e = componentSQLNativeObserve(t, db, recipe, false, nil); e == nil {
		t.Fatal("new owner in the exact originally empty sheet scope hidden")
	}
	crossSQLNativeReplay(t, db, "sheet-91")
	if e = batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		batch, be := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: []uint64{10091}}, DefaultSQLHistoricalOwnerBatchLimits())
		if be != nil {
			return be
		}
		catalog, ce := PrepareSQLHistoricalCrossStoreCatalog(ctx, c, DefaultSQLCrossStoreLimits())
		if ce != nil {
			return ce
		}
		cross, ce := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"sheet-91"}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10091"}}})
		if ce != nil {
			return ce
		}
		if _, ce = FreezeSQLHistoricalAbsentOwnerSelectorRecipe(ctx, batch, cross, []string{"sheet-91"}, []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10091"}}, []uint64{10091}, nil); ce == nil {
			return errors.New("actual replay member outside this source subset was cut away")
		}
		return nil
	}); e != nil {
		t.Fatal("native complete replay subset rejection failed", e)
	}
}

func TestSQLHistoricalComponentNativeOwnerPlanningRecipes(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	insertHistoricalAssessment(t, db, 43)
	selectors := SQLCrossStoreSelectors{EventIDs: []string{"planning-owner-42", "planning-owner-43", "planning-empty-sheet"}, AssessmentIDs: []uint64{42, 43}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}, {Kind: "AnswerSheet", ID: "10043"}, {Kind: "AnswerSheet", ID: "10090"}}}
	present := map[string]uint64{"planning-owner-42": 42, "planning-owner-43": 43}
	absent := map[string]uint64{"planning-empty-sheet": 10090}
	var recipes []*SQLHistoricalComponentRecipe
	var retainedContext context.Context
	var retainedBatch *SQLHistoricalOwnerBatch
	var retainedCatalog *SQLHistoricalCrossStoreCatalog
	capture := func(mixedReplay bool) error {
		return batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
			batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42, 43}, AnswerSheetIDs: []uint64{10042, 10043, 10090}}, DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			catalog, err := PrepareSQLHistoricalCrossStoreCatalog(ctx, cycle, DefaultSQLCrossStoreLimits())
			if err != nil {
				return err
			}
			recipes, err = FreezeSQLHistoricalOwnerPlanningRecipes(ctx, batch, catalog, selectors, nil, present, absent)
			if err != nil {
				return err
			}
			retainedContext, retainedBatch, retainedCatalog = ctx, batch, catalog
			if mixedReplay {
				if len(recipes) != 1 || recipes[0].OwnerPartitionResolved() || len(recipes[0].responsibility.rows["qs_rm_replay_items"]) != 2 || !slices.Equal(recipes[0].selectors.EventIDs, selectors.EventIDs) || len(recipes[0].plan.request.AnswerSheetIDs) != 3 {
					return errors.New("planning input cut the actual mixed replay or original negative ranges")
				}
			} else if len(recipes) != 3 {
				return errors.New("planning input joined unrelated positive and absent owners")
			}
			for _, recipe := range recipes {
				if len(recipe.plan.groups) != 0 || len(recipe.plan.attachments) != 0 || len(recipe.input.writes) != 0 {
					return errors.New("pure planning input invented intended evidence or write groups")
				}
				if err := recipe.RowDependencies(func(_ string, _ uint64, _ string, _ uint64, write bool) error {
					if write {
						return errors.New("planning metadata granted a physical write edge")
					}
					return nil
				}); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := capture(false); err != nil {
		t.Fatal(err)
	}
	if _, err := FreezeSQLHistoricalOwnerPlanningRecipes(retainedContext, retainedBatch, retainedCatalog, selectors, nil, present, absent); err == nil {
		t.Fatal("ended host transaction created new planning input")
	}
	for _, recipe := range recipes {
		if err := componentSQLNativeObserve(t, db, recipe, true, func(ctx context.Context, observed *SQLHistoricalComponentObservation) error {
			if _, err := observed.apply(ctx); err == nil {
				return errors.New("pure planning recipe authorized actual CAS")
			}
			return nil
		}); err != nil {
			t.Fatal("genuine fresh planning baseline rejected", err)
		}
	}
	crossSQLNativeReplay(t, db, "planning-owner-42")
	if err := db.Exec("UPDATE qs_rm_replay_items SET event_id=? WHERE org_id=7 AND request_id=? AND ordinal=1", []byte("planning-empty-sheet"), []byte("native-cross-replay")).Error; err != nil {
		t.Fatal(err)
	}
	replay := standard.ReplayRequest{OrgID: 7, RequestID: "native-cross-replay", Store: "mongo-domain-events", Reason: "native complete input", Targets: []standard.ReplayTarget{{EventID: "planning-owner-42", ExpectedFailureCount: 3}, {EventID: "planning-empty-sheet", ExpectedFailureCount: 4}}}
	hash, err := replay.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("UPDATE qs_rm_replay_requests SET input_hash=? WHERE org_id=7 AND request_id=?", hash[:], []byte(replay.RequestID)).Error; err != nil {
		t.Fatal(err)
	}
	if err = capture(true); err != nil {
		t.Fatal("actual mixed planning replay closure failed", err)
	}
}

func TestSQLHistoricalComponentNativeMixedPresentAbsentSelectors(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	var recipes []*SQLHistoricalComponentRecipe
	capture := func(replay bool) error {
		return batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
			batch, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}, AnswerSheetIDs: []uint64{10042, 10090, 10091}}, DefaultSQLHistoricalOwnerBatchLimits())
			if e != nil {
				return e
			}
			catalog, e := PrepareSQLHistoricalCrossStoreCatalog(ctx, c, DefaultSQLCrossStoreLimits())
			if e != nil {
				return e
			}
			cross, e := PrepareSQLHistoricalCrossStorePage(ctx, catalog, batch, SQLCrossStoreSelectors{EventIDs: []string{"present-owner", "empty-sheet-90", "empty-sheet-91"}, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}, {Kind: "AnswerSheet", ID: "10090"}, {Kind: "AnswerSheet", ID: "10091"}}})
			if e != nil {
				return e
			}
			plan, e := PrepareSQLHistoricalBatchCAS(ctx, batch, []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, batch, 42, 0, "present-owner", "evaluation.requested", nil)})
			if e != nil {
				return e
			}
			provenance, e := SealSQLHistoricalCASProvenance(ctx, plan, batch)
			if e != nil {
				return e
			}
			recipes, e = FreezeSQLHistoricalOwnerComponentRecipes(ctx, batch, cross, provenance, nil, nil, map[string]uint64{"present-owner": 42}, map[string]uint64{"empty-sheet-90": 10090, "empty-sheet-91": 10091})
			if e != nil {
				return e
			}
			if replay {
				if len(recipes) != 1 || recipes[0].OwnerPartitionResolved() {
					return errors.New("mixed complete replay was cut into independent partitions")
				}
				scope, se := recipes[0].OriginalSelectors()
				if se != nil || len(scope.EventIDs) != 3 || len(scope.MongoOwners) != 3 || len(recipes[0].plan.request.AnswerSheetIDs) != 3 || len(recipes[0].plan.groups) != 1 || len(recipes[0].responsibility.rows["qs_rm_replay_items"]) != 2 {
					return errors.New("unresolved original replay/negative/write input lost")
				}
				return nil
			}
			if len(recipes) != 3 {
				return errors.New("mixed source page forced unrelated present and absent owners together")
			}
			for _, invalid := range []map[string]uint64{{"empty-sheet-90": 10042}, {"outside-source": 10090}, {"empty-sheet-90": 10092}, {"present-owner": 10090}} {
				if _, e = FreezeSQLHistoricalOwnerComponentRecipes(ctx, batch, cross, provenance, nil, nil, map[string]uint64{"present-owner": 42}, invalid); e == nil {
					return errors.New("forged/contradictory empty selector admitted on mixed page")
				}
			}
			return nil
		})
	}
	if err := capture(false); err != nil {
		t.Fatal(err)
	}
	for i, recipe := range recipes {
		owners, e := recipe.OwnerIdentities()
		events, ee := recipe.SourceEventIDs()
		if e != nil || ee != nil || !recipe.OwnerPartitionResolved() || len(events) != 1 || i == 0 && (len(owners) != 1 || owners[0].AssessmentID != 42) || i > 0 && len(owners) != 0 {
			t.Fatal("exact present/negative pure partition was lost", e, ee)
		}
		if e = componentSQLNativeObserve(t, db, recipe, false, nil); e != nil {
			t.Fatal("actual fresh mixed child baseline failed", e)
		}
	}
	crossSQLNativeReplay(t, db, "present-owner")
	if e := db.Exec("UPDATE qs_rm_replay_items SET event_id=? WHERE org_id=7 AND request_id=? AND ordinal=1", []byte("empty-sheet-90"), []byte("native-cross-replay")).Error; e != nil {
		t.Fatal(e)
	}
	replay := standard.ReplayRequest{OrgID: 7, RequestID: "native-cross-replay", Store: "mongo-domain-events", Reason: "native complete input", Targets: []standard.ReplayTarget{{EventID: "present-owner", ExpectedFailureCount: 3}, {EventID: "empty-sheet-90", ExpectedFailureCount: 4}}}
	hash, e := replay.Fingerprint()
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Exec("UPDATE qs_rm_replay_requests SET input_hash=? WHERE org_id=7 AND request_id=?", hash[:], []byte(replay.RequestID)).Error; e != nil {
		t.Fatal(e)
	}
	if err := capture(true); err != nil {
		t.Fatal("actual wider mixed replay closure failed", err)
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

func TestSQLHistoricalComponentNativeSemanticViewRejectsDatabaseSwitch(t *testing.T) {
	for _, writable := range []bool{false, true} {
		t.Run(map[bool]string{false: "read_only", true: "read_write"}[writable], func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			insertHistoricalAssessment(t, db, 42)
			record, _ := testCommittedReference(t, 9001, 42, "current-outcome-native")
			po := outcomeToPO(record)
			po.CommittedEventID, po.CommittedEventEvidence = nil, nil
			if err := db.Create(po).Error; err != nil {
				t.Fatal(err)
			}
			r := componentSQLNativeRecipe(t, db, false)
			if err := componentSQLNativeObserve(t, db, r, writable, func(ctx context.Context, o *SQLHistoricalComponentObservation) (result error) {
				v, err := o.SemanticView(ctx)
				if err != nil || v.ValidateBorrowedSnapshot(ctx) != nil {
					return errors.New("actual original database view rejected")
				}
				tx, err := historicalTx(ctx)
				if err != nil {
					return err
				}
				originalExpires := o.expires
				restore := "USE `" + strings.ReplaceAll(r.plan.database, "`", "``") + "`"
				defer func() {
					if err := tx.Exec(restore).Error; err != nil {
						result = errors.New("original database restore failed")
					}
				}()
				if err = tx.Exec("USE `information_schema`").Error; err != nil {
					return err
				}
				server, database, err := historicalDatabase(tx)
				if err != nil || server != r.plan.server || database != "information_schema" {
					return errors.New("native database switch not observed")
				}
				var actual sqlResponsibilityTransaction
				if writable {
					actual, err = casActualRW(tx)
				} else {
					actual, err = cycleActualTransaction(tx)
				}
				if err != nil || actual != o.transaction {
					return errors.New("database switch did not retain original native transaction")
				}
				if o.ValidateBorrowedObservation(ctx) == nil || v.ValidateBorrowedSnapshot(ctx) == nil || v.OriginalOutcomeRunAbsent(ctx, 9001, "42:1") == nil {
					return errors.New("switched database retained observation or original Run authority")
				}
				if _, err = v.OwnerByAssessment(ctx, 42); err == nil {
					return errors.New("switched database retained original owner facts")
				}
				if _, err = v.BusinessBinding(ctx, 42, 0, "evaluation.requested", nil); err == nil {
					return errors.New("switched database retained original business binding")
				}
				if _, err = v.CrossStoreRows(ctx); err == nil {
					return errors.New("switched database retained original raw responsibility view")
				}
				if !o.expires.Equal(originalExpires) {
					return errors.New("database identity check extended original deadline")
				}
				if err = tx.Exec(restore).Error; err != nil {
					return err
				}
				return v.ValidateBorrowedSnapshot(ctx)
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSQLHistoricalComponentNativeFreshSemanticView(t *testing.T) {
	for _, missingRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "actual_owner_outcome_and_replay", true: "exact_original_run_gap"}[missingRun], func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			insertHistoricalAssessment(t, db, 42)
			record, _ := testCommittedReference(t, 9001, 42, "current-outcome-native")
			po := outcomeToPO(record)
			po.CommittedEventID, po.CommittedEventEvidence = nil, nil
			if err := db.Create(po).Error; err != nil {
				t.Fatal(err)
			}
			if missingRun {
				if err := db.Exec("DELETE FROM runtime_checkpoint WHERE assessment_id=42").Error; err != nil {
					t.Fatal(err)
				}
			}
			crossSQLNativeReplay(t, db, "old-original")
			crossSQLNativeHeld(t, db, "actual-semantic-held")
			r := componentSQLNativeRecipe(t, db, false)
			var retained *SQLHistoricalComponentSemanticView
			if err := componentSQLNativeObserve(t, db, r, false, func(ctx context.Context, o *SQLHistoricalComponentObservation) error {
				v, err := o.SemanticView(ctx)
				if err != nil || v.ValidateBorrowedSnapshot(ctx) != nil || v.MatchesInput(ctx, r) != nil || o.ValidateBorrowedObservation(ctx) != nil {
					t.Fatal("actual scoped semantic view rejected", err)
				}
				retained = v
				facts, err := v.OwnerByAssessment(ctx, 42)
				if err != nil || facts.Owner.OrgID != 7 || facts.Owner.TesteeID != 21 || facts.Owner.AnswerSheetID != 10042 || facts.Owner.Status != "evaluated" || len(facts.Outcomes) != 1 || facts.Outcomes[0].RunID != "42:1" || facts.Outcomes[0].Invalid || len(facts.Runs) != map[bool]int{false: 1, true: 0}[missingRun] {
					t.Fatal("actual organization/terminal status/OriginalRun facts lost", err)
				}
				if !missingRun && (facts.Runs[0].ResourceID != "42:1" || facts.Runs[0].Attempt != 1 || facts.Runs[0].Status != "succeeded") {
					t.Fatal("current/latest Run substituted")
				}
				facts.Owner.Status = "forged"
				facts.Outcomes[0].RunID = "forged"
				fresh, err := v.OwnerByAnswerSheet(ctx, 10042)
				if err != nil || fresh.Owner.Status != "evaluated" || fresh.Outcomes[0].RunID != "42:1" {
					t.Fatal("returned facts mutate the live view", err)
				}
				if _, err = v.OwnerByAssessment(ctx, 43); err == nil {
					t.Fatal("outside selector declared absent")
				}
				actual, err := v.OutcomeRecord(ctx, 9001)
				if err != nil || actual.RunID() != record.RunID() || actual.VersionToken() != sqlHistoricalFactRecord(record).VersionToken() {
					t.Fatal("native Outcome decoder or immutable body differs", err)
				}
				binding, err := v.BusinessBinding(ctx, 42, 0, "evaluation.requested", nil)
				if err != nil || binding != r.plan.attachments[0].Entry.Proof.BusinessBindingSHA256 {
					t.Fatal("actual scoped business binding differs from original", err)
				}
				if missingRun {
					if v.OriginalOutcomeRunAbsent(ctx, 9001, "42:1") != nil {
						t.Fatal("actual exact absent original Run not retained")
					}
					if _, err = v.BusinessBinding(ctx, 42, 9001, "evaluation.outcome.committed", nil); err != nil {
						t.Fatal("fresh original gap could not bind actual Outcome", err)
					}
				} else {
					if v.OriginalOutcomeRunAbsent(ctx, 9001, "42:1") == nil {
						t.Fatal("retained original Run declared absent")
					}
					if _, err = v.BusinessBinding(ctx, 42, 9001, "evaluation.outcome.committed", &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}); err != nil {
						t.Fatal("actual declared original Run rejected", err)
					}
				}
				if _, err = v.BusinessBinding(ctx, 42, 9001, "evaluation.outcome.committed", &evidence.HistoricalRunReferenceV1{RunID: "latest", Attempt: 2}); err == nil {
					t.Fatal("unobserved original Run accepted")
				}
				rows, err := v.Responsibilities(ctx)
				items, parents := 0, 0
				for _, row := range rows {
					if row.Store == "qs_rm_replay_items" {
						items++
					}
					if row.Store == "qs_rm_replay_requests" {
						parents++
					}
				}
				if err != nil || items != 2 || parents != 1 {
					t.Fatal("full replay negative responsibility closure cut", err)
				}
				cross, err := v.CrossStoreRows(ctx)
				if err != nil || len(cross) != len(rows) {
					t.Fatal("actual raw view did not retain entire responsibility closure", err)
				}
				message, replayRows := 0, 0
				for _, row := range cross {
					if row.Observation.PrimaryKeySHA256 == "" || row.Observation.RowSHA256 == "" {
						t.Fatal("actual primary key or complete row bytes unbound")
					}
					if row.Observation.Store == "retry_event_hold" {
						message++
						if row.Inner == nil || row.Inner.ID != "actual-semantic-held" || row.LegacyContentSHA256 == "" || row.InnerDataSHA256 == "" || row.Observation.RowSHA256 == row.LegacyContentSHA256 {
							t.Fatal("actual wire decoder or separate byte layers lost")
						}
						row.Inner.Data[0] ^= 1
					}
					if row.Replay != nil {
						replayRows++
						if !row.Replay.FingerprintVerified || len(row.Replay.Items) != 2 || row.Replay.Items[1].EventID != "different-original" {
							t.Fatal("whole actual replay input hidden by initial event selection")
						}
						row.Replay.Items[0].EventID = "edited"
					}
				}
				if message != 1 || replayRows != 3 {
					t.Fatal("actual message or parent/member replay view missing")
				}
				freshCross, err := v.CrossStoreRows(ctx)
				if err != nil || len(freshCross) != len(cross) {
					t.Fatal("fresh same-transaction raw view rejected", err)
				}
				for i, row := range freshCross {
					if row.Inner != nil && slices.Equal(row.Inner.Data, cross[i].Inner.Data) || row.Replay != nil && row.Replay.Items[0].EventID != "old-original" {
						t.Fatal("returned raw/replay copies changed the captured observation")
					}
				}
				o.used = true
				if v.ValidateBorrowedSnapshot(ctx) == nil || o.ValidateBorrowedObservation(ctx) == nil {
					t.Fatal("consumed view reused as fresh qualification")
				}
				if _, err = v.CrossStoreRows(ctx); err == nil {
					t.Fatal("consumed view retained raw/replay access")
				}
				o.used = false
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if retained.ValidateBorrowedSnapshot(t.Context()) == nil {
				t.Fatal("ended transaction kept semantic view alive")
			}
			if _, err := retained.CrossStoreRows(t.Context()); err == nil {
				t.Fatal("ended transaction retained raw/replay access")
			}
		})
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

// Entries below are synthetic fixture conclusions. This test verifies the SQL
// physical persistence/readback boundary, not real four-source/AI authority.
func TestSQLHistoricalComponentNativePlanningAttachmentsCommitReadback(t *testing.T) {
	for _, missingRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "original_outcome_run", true: "exact_original_run_gap"}[missingRun], func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			insertHistoricalAssessment(t, db, 42)
			record, _ := testCommittedReference(t, 9001, 42, "current-outcome-native")
			if err := db.Create(outcomeToPO(record)).Error; err != nil {
				t.Fatal(err)
			}
			if missingRun {
				if err := db.Exec("DELETE FROM runtime_checkpoint WHERE assessment_id=42").Error; err != nil {
					t.Fatal(err)
				}
			}
			ids := []string{"planning-request", "planning-outcome"}
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
				recipes, err := FreezeSQLHistoricalOwnerPlanningRecipes(ctx, batch, catalog, SQLCrossStoreSelectors{EventIDs: ids, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}}}, nil, map[string]uint64{ids[0]: 42, ids[1]: 42})
				if err != nil {
					return err
				}
				if len(recipes) != 1 || len(recipes[0].plan.groups) != 0 || len(recipes[0].plan.attachments) != 0 {
					return errors.New("planning input invented a CAS plan")
				}
				recipe = recipes[0]
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			build := func(ctx context.Context, o *SQLHistoricalComponentObservation) ([]SQLHistoricalBatchAttachment, error) {
				v, err := o.SemanticView(ctx)
				if err != nil {
					return nil, err
				}
				var run *evidence.HistoricalRunReferenceV1
				if !missingRun {
					run = &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
				}
				var out []SQLHistoricalBatchAttachment
				for i, eventType := range []string{"evaluation.requested", "evaluation.outcome.committed"} {
					outcome, original := uint64(0), (*evidence.HistoricalRunReferenceV1)(nil)
					if i == 1 {
						outcome, original = 9001, run
					}
					binding, err := v.BusinessBinding(ctx, 42, outcome, eventType, original)
					if err != nil {
						return nil, err
					}
					entry := nativeHistoricalEntry(ids[i], eventType, binding, original)
					entry.Source.Digest = evidence.SourceDigest("mysql-cast-binary-row-v2", []byte(ids[i]))
					entry.Proof.Digest = entry.Source.Digest
					if i == 1 && missingRun {
						entry.Proof.Class = evidence.Unverifiable
						entry.Proof.Verification.Reason = "original_outcome_run_absent"
					}
					out = append(out, SQLHistoricalBatchAttachment{AssessmentID: 42, OutcomeID: outcome, Entry: entry, ContentDigest: evidence.SourceDigest("legacy-domain-json-bytes-v1", []byte(ids[i]))})
				}
				return out, nil
			}
			sentinel := errors.New("host rollback after planning attachment statements")
			var rolled, committed *SQLHistoricalComponentStatement
			if err := componentSQLNativeObserve(t, db, recipe, true, func(ctx context.Context, o *SQLHistoricalComponentObservation) error {
				entries, err := build(ctx, o)
				if err != nil {
					return err
				}
				for _, mutate := range []func(*SQLHistoricalBatchAttachment){
					func(a *SQLHistoricalBatchAttachment) {
						a.Entry.EventID = "not-in-original-scope"
						a.Entry.Proof.EventID = a.Entry.EventID
					},
					func(a *SQLHistoricalBatchAttachment) { a.AssessmentID = 43 },
					func(a *SQLHistoricalBatchAttachment) { a.Entry.Proof.BusinessBindingSHA256 = strings.Repeat("f", 64) },
					func(a *SQLHistoricalBatchAttachment) {
						a.Entry.Run = &evidence.HistoricalRunReferenceV1{RunID: "42:2", Attempt: 2}
					},
				} {
					bad := entries[1]
					bad.Entry = bad.Entry.Clone()
					mutate(&bad)
					if statement, err := o.ApplyHistoricalAttachments(ctx, []SQLHistoricalBatchAttachment{bad}); err == nil || statement != nil {
						return errors.New("physical bridge accepted unrelated identity, binding or Run")
					}
				}
				rolled, err = o.ApplyHistoricalAttachments(ctx, entries)
				if err != nil {
					return err
				}
				if _, err = o.ApplyHistoricalAttachments(ctx, entries); err == nil {
					return errors.New("physical observer retried an effect")
				}
				return sentinel
			}); !errors.Is(err, sentinel) {
				t.Fatal("planning physical rollback failed", err)
			}
			readback := func(s *SQLHistoricalComponentStatement) error {
				return db.Transaction(func(tx *gorm.DB) error {
					r, err := s.VerifyIndependentPersisted(hostmysql.WithTx(t.Context(), tx), 20*time.Second)
					if err == nil && (!r.IndependentPersistedReadMatched || r.HostCommitVerified || r.WholeRetirementComplete || r.DropReady) {
						return errors.New("physical server readback invented paired commit or DROP")
					}
					return err
				}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
			}
			if err := readback(rolled); err == nil {
				t.Fatal("rolled-back planning statements became persisted proof")
			}
			if err := componentSQLNativeObserve(t, db, recipe, true, func(ctx context.Context, o *SQLHistoricalComponentObservation) error {
				entries, err := build(ctx, o)
				if err != nil {
					return err
				}
				committed, err = o.ApplyHistoricalAttachments(ctx, entries)
				return err
			}); err != nil {
				t.Fatal("planning physical commit failed", err)
			}
			if err := readback(committed); err != nil {
				t.Fatal("independent planning server readback failed", err)
			}
			if r := committed.Report(); !r.StatementApplied || len(r.Locations) != 2 || r.SourceAuthenticated || r.BusinessClosureVerified || r.HostCommitVerified || r.DropReady {
				t.Fatal("physical statements gained source/closure/commit permission", r)
			}
			set := storedHistoricalSet(t, db, "evaluation_outcome", "historical_committed_evidence", 9001)
			if set == nil || len(set.Entries) != 1 || set.Entries[0].EventID != ids[1] || (set.Entries[0].Run == nil) != missingRun {
				t.Fatal("original outcome identity or Run conclusion not persisted")
			}
		})
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

// Genuine captured owner/range recipes represent two original SQL fragments.
// The conclusions are synthetic: this proves the SQL union/host/readback
// boundary, not four-source authentication or paired Mongo/AI acceptance.
func TestSQLHistoricalComponentNativeUnionTwoFragmentsAppendAndReadback(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "current-outcome-native-union")
	if err := db.Create(outcomeToPO(record)).Error; err != nil {
		t.Fatal(err)
	}
	ids := []string{"union-original-first", "union-original-second"}
	var recipes []*SQLHistoricalComponentRecipe
	if err := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}, AnswerSheetIDs: []uint64{10042}}, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		catalog, err := PrepareSQLHistoricalCrossStoreCatalog(ctx, c, DefaultSQLCrossStoreLimits())
		if err != nil {
			return err
		}
		for _, id := range ids {
			frozen, err := FreezeSQLHistoricalOwnerPlanningRecipes(ctx, batch, catalog, SQLCrossStoreSelectors{EventIDs: []string{id}, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}}}, nil, map[string]uint64{id: 42})
			if err != nil {
				return err
			}
			if len(frozen) != 1 || len(frozen[0].plan.groups) != 0 {
				return errors.New("planning fragment invented write permission")
			}
			recipes = append(recipes, frozen[0])
		}
		return nil
	}); err != nil {
		t.Fatal("original fragments capture", err)
	}
	apply := func(ctx context.Context) (*SQLHistoricalComponentStatement, error) {
		var observations []*SQLHistoricalComponentObservation
		var attachments [][]SQLHistoricalBatchAttachment
		for i, recipe := range recipes {
			o, err := PrepareSQLHistoricalComponentObservation(ctx, recipe, 20*time.Second, true)
			if err != nil {
				return nil, err
			}
			view, err := o.SemanticView(ctx)
			if err != nil {
				return nil, err
			}
			original := &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
			binding, err := view.BusinessBinding(ctx, 42, 9001, "evaluation.outcome.committed", original)
			if err != nil {
				return nil, err
			}
			entry := nativeHistoricalEntry(ids[i], "evaluation.outcome.committed", binding, original)
			entry.Source.Digest = evidence.SourceDigest("mysql-cast-binary-row-v2", []byte(ids[i]))
			entry.Proof.Digest = entry.Source.Digest
			observations = append(observations, o)
			attachments = append(attachments, []SQLHistoricalBatchAttachment{{AssessmentID: 42, OutcomeID: 9001, Entry: entry, ContentDigest: evidence.SourceDigest("legacy-domain-json-bytes-v1", []byte(ids[i]))}})
		}
		// An event from the other fragment does not expand the original source
		// selector. Rejected preparation must not poison a not-yet-used input.
		if statement, err := ApplySQLHistoricalComponentAttachments(ctx, observations, [][]SQLHistoricalBatchAttachment{attachments[1], attachments[0]}); err == nil || statement != nil {
			return nil, errors.New("union expanded a fragment source selector")
		}
		statement, err := ApplySQLHistoricalComponentAttachments(ctx, observations, attachments)
		if err != nil {
			return nil, err
		}
		if statement == nil || len(statement.observations) != 2 || len(statement.plan.groups) != 1 || len(statement.plan.groups[0].entries) != 2 || len(statement.statement.expected.rows["evaluation_outcome"]) != 1 {
			return nil, errors.New("overlap was not one physical target with both entries")
		}
		for i, o := range observations {
			if !o.used || o.ValidateBorrowedObservation(ctx) == nil {
				return nil, errors.New("original fragment not poisoned before effect")
			}
			if repeated, err := o.ApplyHistoricalAttachments(ctx, attachments[i]); err == nil || repeated != nil {
				return nil, errors.New("original fragment allowed a second effect")
			}
		}
		if repeated, err := ApplySQLHistoricalComponentAttachments(ctx, observations, attachments); err == nil || repeated != nil {
			return nil, errors.New("union effect was reused")
		}
		return statement, nil
	}
	readback := func(statement *SQLHistoricalComponentStatement) error {
		return db.Transaction(func(tx *gorm.DB) error {
			report, err := statement.VerifyIndependentPersisted(hostmysql.WithTx(t.Context(), tx), 20*time.Second)
			if err == nil && (!report.IndependentPersistedReadMatched || report.HostCommitVerified || report.DropReady) {
				return errors.New("SQL union invented paired commit or DROP")
			}
			return err
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	}
	sentinel := errors.New("original host rollback after union")
	var rolled, committed *SQLHistoricalComponentStatement
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		rolled, err = apply(hostmysql.WithTx(t.Context(), tx))
		if err != nil {
			return err
		}
		return sentinel
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead}); !errors.Is(err, sentinel) {
		t.Fatal("actual union rollback", err)
	}
	if err := readback(rolled); err == nil {
		t.Fatal("rolled-back union became persisted proof")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		committed, err = apply(hostmysql.WithTx(t.Context(), tx))
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead}); err != nil {
		t.Fatal("actual same-owner union append", err)
	}
	if err := readback(committed); err != nil {
		t.Fatal("final union independent readback", err)
	}
	set := storedHistoricalSet(t, db, "evaluation_outcome", "historical_committed_evidence", 9001)
	if set == nil || len(set.Entries) != 2 || set.Entries[0].EventID != ids[0] || set.Entries[1].EventID != ids[1] || set.Entries[0].Run == nil || set.Entries[0].Run.RunID != "42:1" || set.Entries[1].Run == nil || set.Entries[1].Run.RunID != "42:1" {
		t.Fatal("union lost original event IDs or run identity")
	}
	if err := componentSQLNativeObserve(t, db, recipes[1], true, nil); err == nil {
		t.Fatal("old original fragment baseline accepted after real evidence change")
	}
	if err := db.Exec("UPDATE evaluation_outcome SET historical_committed_evidence=JSON_REMOVE(historical_committed_evidence,'$.entries[0]') WHERE id=9001").Error; err != nil {
		t.Fatal("owned actual readback conflict setup", err)
	}
	if err := readback(committed); err == nil {
		t.Fatal("union accepted actual persisted evidence missing one original entry")
	}
}
