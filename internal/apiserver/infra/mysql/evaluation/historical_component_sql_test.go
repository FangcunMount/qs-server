package evaluation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSQLHistoricalComponentClosedScopeAndNoImportedAuthority(t *testing.T) {
	s := SQLCrossStoreSelectors{EventIDs: []string{"exact-event"}, AssessmentIDs: []uint64{42}, OrganizationIDs: []uint64{7}, MongoOwners: []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}}}
	for _, spec := range sqlResponsibilityTables {
		q, args, err := componentPredicate(spec, s, s.EventIDs)
		if err != nil || q == "" {
			t.Fatal("fixed ledger selector missing", spec.name, err)
		}
		if spec.name == "rm_outbox" || spec.name == "retry_event_hold" || spec.name == "event_delivery_dead_letter" {
			if !strings.Contains(q, "FROM_BASE64") || !strings.Contains(q, "COALESCE(JSON_VALID") || !strings.Contains(q, "$.data.assessment_id") || len(args) < 4 {
				t.Fatal("actual revision/invalid/owner selectors omitted", spec.name)
			}
			if strings.Contains(q, "org_id IN") || strings.Contains(q, "deleted_at") {
				t.Fatal("organization or deletion hides contradictory owner")
			}
		}
	}
	r := &SQLHistoricalComponentRecipe{}
	if json.Unmarshal([]byte(`{}`), r) == nil {
		t.Fatal("editable recipe imported")
	}
	if o, err := PrepareSQLHistoricalComponentObservation(context.Background(), r, time.Second, false); err == nil || o != nil {
		t.Fatal("forged input creates native observation")
	}
	if o, err := PrepareSQLHistoricalComponentObservation(context.Background(), r, 21*time.Second, true); err == nil || o != nil {
		t.Fatal("oversized epoch accepted")
	}
	o := &SQLHistoricalComponentObservation{}
	o.self = o
	if o.Report().BusinessMatched || o.Report().DropReady {
		t.Fatal("empty seal called successful")
	}
	if _, err := o.apply(context.Background()); err == nil {
		t.Fatal("empty observation writes")
	}
	if _, err := (&SQLHistoricalComponentStatement{}).VerifyIndependentPersisted(context.Background(), time.Second); err == nil {
		t.Fatal("forged statement readback accepted")
	}
	if r, err := FreezeSQLHistoricalComponentRecipeToSpool(context.Background(), nil, nil, nil, nil, &SQLHistoricalCASSpool{}); r != nil || err == nil {
		t.Fatal("private spool fabricated original native inputs")
	}
	if r, err := FreezeSQLHistoricalAbsentOwnerSelectorRecipe(context.Background(), nil, nil, []string{"exact-event"}, []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: "10042"}}, []uint64{10042}, nil); r != nil || err == nil {
		t.Fatal("caller selectors fabricated a live original SQL-empty range")
	}
	if recipes, err := FreezeSQLHistoricalOwnerPlanningRecipes(context.Background(), nil, nil, s, nil, map[string]uint64{"exact-event": 42}); recipes != nil || err == nil {
		t.Fatal("editable selectors fabricated actual owner planning input")
	}
	copied := componentCopySelectors(s)
	copied.EventIDs[0] = "changed"
	copied.AssessmentIDs[0] = 9
	copied.MongoOwners[0].ID = "other"
	if s.EventIDs[0] != "exact-event" || s.AssessmentIDs[0] != 42 || s.MongoOwners[0].ID != "10042" {
		t.Fatal("recipe selectors share editable source slice")
	}
}

func TestSQLHistoricalComponentSemanticViewRequiresActualObservation(t *testing.T) {
	ctx := context.Background()
	for _, o := range []*SQLHistoricalComponentObservation{nil, {}} {
		if o.ValidateBorrowedObservation(ctx) == nil {
			t.Fatal("absent native pool/transaction accepted")
		}
		if v, err := o.SemanticView(ctx); err == nil || v != nil {
			t.Fatal("editable or absent observer fabricated fresh semantic facts")
		}
	}
	v := &SQLHistoricalComponentSemanticView{}
	if json.Unmarshal([]byte(`{}`), v) == nil {
		t.Fatal("editable semantic view imported")
	}
	if _, err := json.Marshal(v); err == nil {
		t.Fatal("semantic view serialized as a portable capability")
	}
	if v.ValidateBorrowedSnapshot(ctx) == nil || v.MatchesInput(ctx, &SQLHistoricalComponentRecipe{}) == nil {
		t.Fatal("empty semantic view claimed actual scope/input")
	}
	if _, err := v.OwnerByAssessment(ctx, 42); err == nil {
		t.Fatal("forged view yielded an owner")
	}
	if _, err := v.OwnerByAnswerSheet(ctx, 10042); err == nil {
		t.Fatal("forged view yielded a sheet association")
	}
	if _, err := v.Responsibilities(ctx); err == nil {
		t.Fatal("forged view claimed a negative closure")
	}
	if _, err := v.OutcomeRecord(ctx, 9001); err == nil || v.OriginalOutcomeRunAbsent(ctx, 9001, "42:1") == nil {
		t.Fatal("forged view claimed an original Outcome/Run")
	}
	if _, err := v.BusinessBinding(ctx, 42, 0, "evaluation.requested", nil); err == nil {
		t.Fatal("forged view minted a business anchor")
	}
}

// This directly checks the private pure graph algorithm. These synthetic
// metadata values never pass a live factory or create a physical qualification.
func TestSQLHistoricalComponentOwnerGraphKeepsReplayAtomic(t *testing.T) {
	plan := &SQLHistoricalBatchCASPlan{request: SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42, 43}}, before: sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{"assessment": {cycleTestRow(map[string]string{"id": "42", "org_id": "7", "answer_sheet_id": "10042"}), cycleTestRow(map[string]string{"id": "43", "org_id": "7", "answer_sheet_id": "10043"})}}}}
	input := &SQLHistoricalCASFrozenInput{before: plan.before, writes: map[string]bool{}}
	input.self, input.seal = input, input.digest()
	r := &SQLHistoricalComponentRecipe{plan: plan, input: input, selectors: SQLCrossStoreSelectors{EventIDs: []string{"first", "second"}, AssessmentIDs: []uint64{42, 43}}, responsibility: sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}}}
	r.self, r.seal = r, r.digest()
	cycle := &SQLHistoricalResponsibilityCycle{observations: []SQLResponsibilityObservation{{EventID: "first", AssessmentID: 42}, {EventID: "second", AssessmentID: 43}}, byEvent: map[string][]int{"first": {0}, "second": {1}}}
	c := &SQLHistoricalCrossStoreCatalog{cycle: cycle, report: SQLCrossStoreCatalogReport{Complete: true}, byRequest: map[string][]int{cyclePair(7, "pair"): {0, 1}}}
	parts, err := componentOriginalOwnerPartitions(r, c)
	if err != nil || len(parts) != 2 {
		t.Fatal("unrelated source-page owners were joined", err)
	}
	for _, selected := range []map[string]uint64{{"first": 43}, {"outside": 42}, {"first": 99}} {
		if _, err = componentOriginalOwnerPartitions(r, c, selected); err == nil {
			t.Fatal("pure selector contradicted current bindings or expanded original scope")
		}
	}
	cycle.observations[0].OrgID = 8
	if _, err = componentOriginalOwnerPartitions(r, c, map[string]uint64{"first": 42}); err == nil {
		t.Fatal("current cross-organization binding hidden by pure selector")
	}
	cycle.observations[0].OrgID = 0
	r.responsibility.rows["qs_rm_replay_items"] = []historicalSQLRow{cycleTestRow(map[string]string{"org_id": "7", "request_id": "pair"})}
	r.seal = r.digest()
	parts, err = componentOriginalOwnerPartitions(r, c)
	if err != nil || len(parts) != 1 || len(parts[0].ids) != 2 || len(parts[0].events) != 2 {
		t.Fatal("complete cross-owner replay dependency was split", err)
	}
	cycle.observations = append(cycle.observations, SQLResponsibilityObservation{EventID: "outside", AssessmentID: 99})
	c.byRequest[cyclePair(7, "pair")] = append(c.byRequest[cyclePair(7, "pair")], 2)
	if _, err = componentOriginalOwnerPartitions(r, c); err == nil {
		t.Fatal("outside captured owner was silently dropped")
	}
	if _, err = FreezeSQLHistoricalOwnerComponentRecipes(context.Background(), nil, nil, nil, nil, nil); err == nil {
		t.Fatal("private graph metadata manufactured a live original recipe")
	}
}
