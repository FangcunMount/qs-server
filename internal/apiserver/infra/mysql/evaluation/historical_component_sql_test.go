package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
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
	for _, observed := range []*SQLHistoricalComponentObservation{nil, o} {
		if statement, err := observed.ApplyHistoricalAttachments(context.Background(), []SQLHistoricalBatchAttachment{{}}); err == nil || statement != nil {
			t.Fatal("forged physical observer accepted imported attachments")
		}
	}
	if report := (&SQLHistoricalComponentStatement{}).Report(); report.StatementApplied || report.HostCommitVerified || report.SourceAuthenticated || report.BusinessClosureVerified || report.DropReady || !report.IndependentReadbackRequired || !report.HostCommitRequired {
		t.Fatal("empty physical statement invented closure or host commit")
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
	if _, err := v.CrossStoreRows(ctx); err == nil {
		t.Fatal("forged view exposed raw responsibility or replay rows")
	}
	if _, err := v.OutcomeRecord(ctx, 9001); err == nil || v.OriginalOutcomeRunAbsent(ctx, 9001, "42:1") == nil {
		t.Fatal("forged view claimed an original Outcome/Run")
	}
	if _, err := v.BusinessBinding(ctx, 42, 0, "evaluation.requested", nil); err == nil {
		t.Fatal("forged view minted a business anchor")
	}
}

// Synthetic decoder inputs prove no native scope or writing permission. The
// public view refuses them; actual same-transaction tests live in integration.
func componentUnitCrossStoreImage(t *testing.T) (sqlHistoricalCASImage, []SQLResponsibilityObservation) {
	t.Helper()
	p := crossUnitReplayPage(t)
	image := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, columns: map[string][]string{}}
	for _, spec := range sqlResponsibilityTables {
		image.rows[spec.name], image.columns[spec.name] = []historicalSQLRow{}, slices.Clone(spec.keys)
	}
	image.rows["rm_outbox"] = []historicalSQLRow{cycleTestMessage(t, "original-a", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload())}
	image.rows["qs_rm_replay_requests"] = []historicalSQLRow{p.rows[0]}
	image.rows["qs_rm_replay_items"] = []historicalSQLRow{p.rows[1], p.rows[2]}
	var observed []SQLResponsibilityObservation
	for _, spec := range sqlResponsibilityTables {
		rows := image.rows[spec.name]
		if len(rows) > 0 {
			columns := []string{}
			for column := range rows[0] {
				columns = append(columns, column)
			}
			sort.Strings(columns)
			image.columns[spec.name] = columns
		}
		for _, row := range rows {
			observed = append(observed, cycleDecode(spec.name, row))
		}
	}
	return image, observed
}

func TestSQLHistoricalComponentCrossStoreWholeRawReplayAndCopies(t *testing.T) {
	image, observed := componentUnitCrossStoreImage(t)
	observed[0].OwnerUnproven = true
	observed[0].Reasons = append(observed[0].Reasons, "actual_owner_diagnostic")
	for i := 2; i < len(observed); i++ {
		observed[i].link.store = "mongo-domain-events"
		observed[i].OwnerUnproven = true
	}
	rows, err := componentCrossStoreRows(image, observed)
	if err != nil || len(rows) != 4 || rows[0].Inner == nil || rows[0].Observation.PrimaryKeySHA256 == "" || rows[0].Observation.RowSHA256 == "" || !rows[0].Observation.OwnerUnproven || len(rows[0].Observation.Reasons) != 1 {
		t.Fatal("whole actual raw row/owner diagnostic lost", err)
	}
	raw := mustCycleInner(image.rows["rm_outbox"][0], "rm_outbox")
	innerSHA, dataSHA := sha256.Sum256(raw), sha256.Sum256(rows[0].Inner.Data)
	if rows[0].LegacyContentSHA256 != hex.EncodeToString(innerSHA[:]) || rows[0].InnerDataSHA256 != hex.EncodeToString(dataSHA[:]) || rows[0].LegacyContentSHA256 == rows[0].Observation.RowSHA256 {
		t.Fatal("original layers of actual bytes collapsed")
	}
	for _, row := range rows[1:] {
		if row.Replay == nil || !row.Replay.FingerprintVerified || len(row.Replay.Items) != 2 || row.Replay.Items[1].EventID != "original-b" {
			t.Fatal("different event in whole replay closure hidden")
		}
	}
	rows[0].Inner.Data[0] ^= 1
	rows[0].Observation.Reasons[0] = "changed"
	rows[1].Replay.Items[0].EventID = "changed"
	if rows[2].Replay.Items[0].EventID != "original-a" {
		t.Fatal("returned replay row shares editable items")
	}
	fresh, err := componentCrossStoreRows(image, observed)
	if err != nil || fresh[0].Inner.Data[0] == rows[0].Inner.Data[0] || fresh[0].Observation.Reasons[0] != "actual_owner_diagnostic" || fresh[1].Replay.Items[0].EventID != "original-a" {
		t.Fatal("returned copies changed captured data", err)
	}
}

func TestSQLHistoricalComponentCrossStoreClosureAndHashDrift(t *testing.T) {
	for _, name := range []string{"missing_table", "missing_observation", "extra_observation", "owner_drift", "wrong_row_hash", "duplicate_key", "missing_parent", "ordinal_gap", "wrong_hash", "missing_member", "missing_column"} {
		t.Run(name, func(t *testing.T) {
			image, observed := componentUnitCrossStoreImage(t)
			switch name {
			case "missing_table":
				delete(image.rows, "event_delivery_dead_letter")
			case "missing_observation":
				observed = observed[:len(observed)-1]
			case "extra_observation":
				observed = append(observed, observed[0])
			case "owner_drift":
				observed[0].OrgID = 8
			case "wrong_row_hash":
				observed[0].RowSHA256 = strings.Repeat("0", 64)
			case "duplicate_key":
				image.rows["rm_outbox"] = append(image.rows["rm_outbox"], image.rows["rm_outbox"][0])
				observed = append(observed[:1], append([]SQLResponsibilityObservation{observed[0]}, observed[1:]...)...)
			case "missing_parent":
				image.rows["qs_rm_replay_requests"] = nil
				observed = append(observed[:1], observed[2:]...)
			case "ordinal_gap":
				image.rows["qs_rm_replay_items"][1]["ordinal"] = strptr("2")
				observed[3] = cycleDecode("qs_rm_replay_items", image.rows["qs_rm_replay_items"][1])
			case "wrong_hash":
				image.rows["qs_rm_replay_requests"][0]["input_hash"] = strptr(string(make([]byte, 32)))
			case "missing_member":
				image.rows["qs_rm_replay_items"] = image.rows["qs_rm_replay_items"][:1]
				observed = observed[:3]
			case "missing_column":
				delete(image.rows["rm_outbox"][0], "payload")
			}
			rows, err := componentCrossStoreRows(image, observed)
			if name == "wrong_hash" || name == "missing_member" {
				if err == nil && (rows[1].Replay == nil || rows[1].Replay.FingerprintVerified) {
					t.Fatal("changed/incomplete replay input declared fingerprint verified")
				}
			} else if err == nil {
				t.Fatal("changed raw closure or key/hash binding accepted")
			}
		})
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

func TestSQLHistoricalComponentUnionCannotImportOrReuseObservers(t *testing.T) {
	for _, observers := range [][]*SQLHistoricalComponentObservation{nil, {nil}, {{}}, {{}, {}}} {
		attachments := make([][]SQLHistoricalBatchAttachment, len(observers))
		for i := range attachments {
			attachments[i] = []SQLHistoricalBatchAttachment{{}}
		}
		if statement, err := ApplySQLHistoricalComponentAttachments(context.Background(), observers, attachments); err == nil || statement != nil {
			t.Fatal("imported or absent native union observer accepted")
		}
	}
	o := &SQLHistoricalComponentObservation{}
	if statement, err := ApplySQLHistoricalComponentAttachments(context.Background(), []*SQLHistoricalComponentObservation{o, o}, [][]SQLHistoricalBatchAttachment{{{}}, {{}}}); err == nil || statement != nil {
		t.Fatal("duplicate original observer accepted")
	}
}

// Pure image tests exercise overlap and bounds only; they create no native
// observation, SQL statement, source proof or writing authority.
func TestSQLHistoricalComponentUnionPreservesExactOriginalRowsAndBounds(t *testing.T) {
	makeObserver := func(id string) *SQLHistoricalComponentObservation {
		image := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{"assessment": {cycleTestRow(map[string]string{"id": id, "org_id": "7", "answer_sheet_id": "10042"})}, "runtime_checkpoint": {}, "evaluation_outcome": {}, "cas_migration_head": {cycleTestRow(map[string]string{"version": "99", "dirty": "0"})}}, schema: map[string]string{"assessment": "same"}, columns: map[string][]string{"assessment": {"id", "org_id", "answer_sheet_id"}}}
		return &SQLHistoricalComponentObservation{recipe: &SQLHistoricalComponentRecipe{plan: &SQLHistoricalBatchCASPlan{identity: "identity", server: "server", database: "database", request: SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}, AnswerSheetIDs: []uint64{10042}}, limits: DefaultSQLHistoricalOwnerBatchLimits()}}, business: image}
	}
	first, second := makeObserver("42"), makeObserver("42")
	merged, err := componentSQLUnionPlan([]*SQLHistoricalComponentObservation{first, second})
	if err != nil || len(merged.before.rows["assessment"]) != 1 || len(merged.request.AssessmentIDs) != 1 {
		t.Fatal("same actual original physical row not deduplicated", err)
	}
	*merged.before.rows["assessment"][0]["org_id"] = "changed"
	if valueOrEmpty(first.business.rows["assessment"][0]["org_id"]) != "7" || valueOrEmpty(second.business.rows["assessment"][0]["org_id"]) != "7" {
		t.Fatal("merged plan mutates sealed original images")
	}
	for _, which := range []string{"duplicate_raw", "schema", "columns", "metadata", "transaction", "old_transaction", "database", "limits", "owner_budget", "row_budget", "byte_budget"} {
		t.Run(which, func(t *testing.T) {
			first, second := makeObserver("42"), makeObserver("42")
			switch which {
			case "duplicate_raw":
				second.business.rows["assessment"][0]["org_id"] = strptr("8")
			case "schema":
				second.business.schema["assessment"] = "changed"
			case "columns":
				second.business.columns["assessment"] = []string{"id"}
			case "metadata":
				second.business.rows["cas_migration_head"][0]["dirty"] = strptr("1")
			case "transaction":
				second.transaction.event = 1
			case "old_transaction":
				second.recipe.plan.oldTransaction.event = 1
			case "database":
				second.recipe.plan.database = "other"
			case "limits":
				second.recipe.plan.limits.MaxRows++
			case "owner_budget":
				first.recipe.plan.limits.MaxOwners, second.recipe.plan.limits.MaxOwners = 1, 1
				second.recipe.plan.request.AssessmentIDs = []uint64{43}
				second.business.rows["assessment"][0]["id"] = strptr("43")
			case "row_budget":
				first.recipe.plan.limits.MaxRows, second.recipe.plan.limits.MaxRows = 1, 1
				second.business.rows["assessment"][0]["id"] = strptr("43")
			case "byte_budget":
				first.recipe.plan.limits.MaxBytes, second.recipe.plan.limits.MaxBytes = 1, 1
			}
			if _, err := componentSQLUnionPlan([]*SQLHistoricalComponentObservation{first, second}); err == nil {
				t.Fatal("inconsistent original row/host/bounds accepted")
			}
		})
	}
}
