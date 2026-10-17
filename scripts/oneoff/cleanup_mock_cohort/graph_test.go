package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type graphFake struct {
	sqlRows        map[string][]record
	mongoRows      map[string][]record
	calls          []string
	fail           string
	mongoRootScans map[string]int
}

func (s *graphFake) sqlPages(_ context.Context, name, where string, args []any, emit func(record) error) error {
	s.calls = append(s.calls, "mysql/"+name+":"+where)
	if name == s.fail {
		return errors.New("readonly page interrupted")
	}
	for _, r := range s.sqlRows[name] {
		matched := true
		for _, column := range []string{"id", "testee_id", "assessment_id", "outcome_id", "report_id", "task_id", "business_object_id", "source_ref", "profile_id", "user_id"} {
			marker := "x.`" + column + "` IN ("
			start := strings.Index(where, marker)
			if start < 0 {
				marker = "x." + column + " IN ("
				start = strings.Index(where, marker)
			}
			if start < 0 {
				continue
			}
			end := strings.Index(where[start:], ")")
			if end < 0 {
				panic("bad fixed fake IN")
			}
			offset := strings.Count(where[:start], "?")
			n := strings.Count(where[start:start+end], "?")
			value, _ := graphSQLValue(r, column)
			found := false
			for _, arg := range args[offset : offset+n] {
				found = found || arg == value
			}
			matched = matched && found
		}
		for _, column := range []string{"org_id", "source_type", "fact_type"} {
			marker := "x." + column + "=?"
			at := strings.Index(where, marker)
			if at >= 0 {
				value, _ := graphSQLValue(r, column)
				matched = matched && args[strings.Count(where[:at], "?")] == value
			}
		}
		if name == "event_delivery_dead_letter" {
			state, _ := graphSQLValue(r, "retry_disposition")
			if state == "archived_mock" || state == "terminal" {
				matched = false
			}
		}
		if name == "retry_event_hold" {
			state, _ := graphSQLValue(r, "status")
			if state == "replayed" {
				matched = false
			}
		}
		if !matched {
			continue
		}
		if e := emit(r); e != nil {
			return e
		}
	}
	return nil
}

func (s *graphFake) sqlOneCursor(_ context.Context, name, where string, _ []any, emit func(record) error) error {
	// Supply a candidate superset: the planner must still check the named
	// typed-parent domains. Scalar IN(?) parsing does not model JSON_TABLE.
	s.calls = append(s.calls, "mysql/"+name+":"+where)
	if name == s.fail {
		return errors.New("readonly page interrupted")
	}
	for _, r := range s.sqlRows[name] {
		if e := emit(r); e != nil {
			return e
		}
	}
	return nil
}

func (s *graphFake) mongoPages(_ context.Context, name string, filter bson.D, emit func(record) error) error {
	s.calls = append(s.calls, "mongo/"+name)
	if len(filter) == 0 {
		s.mongoRootScans[name]++
	}
	for _, r := range s.mongoRows[name] {
		if len(filter) > 0 && filter[0].Key != "$or" {
			value := bson.Raw(r.BSON).Lookup(filter[0].Key)
			if expr, ok := filter[0].Value.(bson.D); ok && len(expr) > 0 && expr[0].Key == "$in" {
				found := false
				for _, id := range expr[0].Value.(bson.A) {
					if n, ok := id.(int64); ok {
						found = found || graphRawIDMember(bson.Raw(r.BSON), filter[0].Key, map[string]bool{strconv.FormatInt(n, 10): true})
					}
				}
				if !found {
					continue
				}
			} else if text, ok := filter[0].Value.(string); ok {
				if value.Type != bson.TypeString || value.StringValue() != text {
					continue
				}
			}
		}
		if name == "rm_outbox" && graphMongoString(bson.Raw(r.BSON), "state") == "published" {
			continue
		}

		if e := emit(r); e != nil {
			return e
		}
	}
	return nil
}
func (s *graphFake) sqlOwnerPages(ctx context.Context, name string, ids []string, emit func(record) error) error {
	slots, args := placeholders(ids)
	return s.sqlPages(ctx, name, "x.id IN ("+slots+")", args, emit)
}
func (s *graphFake) mongoOwnerPages(ctx context.Context, name string, ids []string, emit func(record) error) error {
	return s.mongoPages(ctx, name, bson.D{{Key: "domain_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(ids)}}}}, emit)
}

func graphSQLFixture(name, id string, fields map[string]string) record {
	r := record{Object: key(name, id), Definition: []byte("fixed original ddl/session"), Columns: []cell{{Name: "id", Type: "UNSIGNED BIGINT", Raw: []byte(id)}}}
	for _, name := range []string{"org_id", "testee_id", "profile_id", "source", "assessment_id", "answer_sheet_id", "answersheet_id", "outcome_id", "plan_id", "enrollment_id", "status", "state", "source_type", "fact_type", "source_ref", "business_object_id", "report_id", "task_id"} {
		if v, ok := fields[name]; ok {
			r.Columns = append(r.Columns, cell{Name: name, Type: "VARCHAR", Raw: []byte(v)})
		}
	}
	return r
}
func graphMongoFixture(t *testing.T, name string, fields bson.D) record {
	t.Helper()
	raw, e := bson.Marshal(append(bson.D{{Key: "_id", Value: primitive.NewObjectID()}}, fields...))
	if e != nil {
		t.Fatal(e)
	}
	r, e := graphMongoObject(name, raw, []byte("fixed namespace/UUID/indexes"))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func graphFixture(t *testing.T) (*graphFake, cohortSelector, protected, string) {
	t.Helper()
	dir := privateTestDir(t)
	p := protected{Format: 1, Kind: "mock_cohort_protected", IAM: []byte(`{"user_ids":["10001"],"profile_ids":[],"login_identity_ids":[]}`), QS: protectionScope{System: []object{key("testee", "42")}, Clack: []object{key("testee", "43")}, Other: []object{}}}
	c := cohortSelector{Format: 2, Kind: "mock_cohort_selector", Operation: "graph-test", OwnerEvidenceSHA: strings.Repeat("a", 64), ProtectionSHA: digest(encode(p)), Binding: binding{SQLUUID: "fixture", SQLDatabase: "qs", MongoIdentity: strings.Repeat("b", 64), MongoDatabase: "qs", SQLVersion: 101, MongoVersion: 39}, Selection: selection{Source: "daily_simulation", OrgID: "1", From: "2025-01-01T00:00:00+08:00", Through: "2026-10-01T00:00:00+08:00", Evidence: "owner confirmed dedicated fixed roots"}, IdentityLinksSHA: strings.Repeat("d", 64), IdentityGroups: []cohortIdentityGroup{{UserIDs: []string{"10"}, ProfileIDs: []string{"11"}, TesteeIDs: []string{"1"}}}, TesteeIDs: []string{"1"}, RetainPlanIDs: []string{"500"}}
	s := &graphFake{sqlRows: map[string][]record{}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
	s.sqlRows["testee"] = []record{graphSQLFixture("testee", "1", map[string]string{"org_id": "1", "profile_id": "11", "source": "daily_simulation"})}
	s.sqlRows["assessment"] = []record{graphSQLFixture("assessment", "100", map[string]string{"org_id": "1", "testee_id": "1"}), graphSQLFixture("assessment", "101", map[string]string{"org_id": "1", "testee_id": "2"})}
	s.sqlRows["evaluation_outcome"] = []record{graphSQLFixture("evaluation_outcome", "200", map[string]string{"org_id": "1", "testee_id": "1", "assessment_id": "100"})}
	s.sqlRows["assessment_task"] = []record{graphSQLFixture("assessment_task", "400", map[string]string{"org_id": "1", "testee_id": "1", "plan_id": "500", "enrollment_id": "600", "status": "completed"})}
	s.sqlRows["plan_enrollment"] = []record{graphSQLFixture("plan_enrollment", "600", map[string]string{"org_id": "1", "testee_id": "1", "plan_id": "500"})}
	for i, purpose := range []string{"independent_questionnaire", "assessment"} {
		fields := bson.D{{Key: "domain_id", Value: int64(20 + i)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(1)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: purpose}}}}
		s.mongoRows["answersheets"] = append(s.mongoRows["answersheets"], graphMongoFixture(t, "answersheets", fields))
	}
	s.mongoRows["answersheets"] = append(s.mongoRows["answersheets"], graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(23)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(2)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "assessment"}}}}))
	s.mongoRows["interpret_report_artifacts"] = []record{graphMongoFixture(t, "interpret_report_artifacts", bson.D{{Key: "domain_id", Value: int64(700)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(1)}, {Key: "assessment_id", Value: int64(100)}, {Key: "outcome_id", Value: int64(200)}, {Key: "generation_id", Value: int64(800)}, {Key: "interpretation_run_id", Value: int64(900)}})}
	s.mongoRows["report_generations"] = []record{graphMongoFixture(t, "report_generations", bson.D{{Key: "domain_id", Value: int64(800)}, {Key: "outcome_id", Value: int64(200)}, {Key: "status", Value: "generated"}, {Key: "latest_run_id", Value: int64(900)}, {Key: "report_id", Value: int64(700)}})}
	s.mongoRows["interpretation_runs"] = []record{graphMongoFixture(t, "interpretation_runs", bson.D{{Key: "domain_id", Value: int64(900)}, {Key: "generation_id", Value: int64(800)}, {Key: "status", Value: "succeeded"}})}
	s.mongoRows["report_query_catalog"] = []record{graphMongoFixture(t, "report_query_catalog", bson.D{{Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(1)}, {Key: "assessment_id", Value: int64(100)}, {Key: "outcome_id", Value: int64(200)}, {Key: "generation_id", Value: int64(800)}, {Key: "source_kind", Value: "artifact"}, {Key: "source_id", Value: int64(700)}, {Key: "sort_report_id", Value: int64(700)}})}
	return s, c, p, dir
}
func TestGraphPlannerFreezesIndependentAndPlanChainsWithoutSharedRoots(t *testing.T) {
	s, c, p, dir := graphFixture(t)
	runtime := graphSQLFixture("runtime_checkpoint", "1100", map[string]string{"assessment_id": "100"})
	runtime.Columns = append(runtime.Columns, cell{Name: "scope", Type: "VARCHAR", Raw: []byte("evaluation_run")})
	s.sqlRows["runtime_checkpoint"] = []record{runtime}
	reminder := graphSQLFixture("task_opened_reminder_delivery", "1200", map[string]string{"org_id": "1"})
	reminder.Columns = append(reminder.Columns, cell{Name: "task_id", Type: "VARBINARY", Raw: []byte("400")})
	s.sqlRows["task_opened_reminder_delivery"] = []record{reminder}
	ref := record{Object: object{Store: "mysql", Name: "qs_rm_evaluation_request_ref", Key: []cell{{Name: "event_id", Type: "bytes", Raw: []byte("fixture-evaluation")}}}, Definition: []byte("fixed definition"), Columns: []cell{{Name: "event_id", Type: "VARBINARY", Raw: []byte("fixture-evaluation")}, {Name: "org_id", Type: "BIGINT", Raw: []byte("1")}, {Name: "assessment_id", Type: "UNSIGNED BIGINT", Raw: []byte("100")}}}
	s.sqlRows["qs_rm_evaluation_request_ref"] = []record{ref}

	index, e := planCohort(t.Context(), s, c, p, dir, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	if !index.ReadOnly || !index.DiscoveryComplete || !index.WritePending || index.Released {
		t.Fatal("discovery cannot grant released/writable whole-cohort authority")
	}
	if index.Counts["mysql/assessment"] != 1 || index.Counts["mongo/answersheets"] != 2 || index.Counts["mongo/report_generations"] != 1 || index.Counts["mongo/interpretation_runs"] != 1 || index.Retained["assessment_plan/500"] == "" {
		t.Fatalf("fixed graph counts/shared root mismatch: %#v", index.Counts)
	}
	if s.mongoRootScans["answersheets"] != 1 || s.mongoRootScans["interpret_report_artifacts"] != 0 || s.mongoRootScans["report_query_catalog"] != 0 {
		t.Fatal("only the unindexed AnswerSheet root relation may require one technical scan")
	}
	for _, call := range s.calls {
		if strings.Contains(call, "mysql/assessment_plan:") || strings.Contains(call, "mysql/assessment_entry:") {
			t.Fatal("shared definitions must not become root-owned targets")
		}
	}
	for _, shard := range index.Shards {
		page, e := readGraphPage(dir, shard)
		if e != nil {
			t.Fatal(e)
		}
		for _, o := range page.Objects {
			if len(o.RootIDs) != 1 || o.RootIDs[0] != "1" {
				t.Fatalf("typed child must preserve exact subset root ownership: %s", o.Object.Name)
			}
		}
	}
}
func TestGraphPlannerRejectsProtectedAndMissingFrozenRootBeforeChildren(t *testing.T) {
	s, c, p, dir := graphFixture(t)
	c.TesteeIDs = []string{"43"}
	if _, e := planCohort(t.Context(), s, c, p, dir, digest(encode(c)), "source"); e == nil || len(s.calls) != 0 {
		t.Fatal("protected root must fail before discovery")
	}
	c.TesteeIDs = []string{"1"}
	s.sqlRows["testee"] = nil
	if _, e := planCohort(t.Context(), s, c, p, dir, digest(encode(c)), "source"); e == nil {
		t.Fatal("filtered root is drift, not a smaller valid cohort")
	}
	if len(s.calls) != 1 {
		t.Fatal("missing root must stop before children")
	}
}
func TestGraphPlannerHoldsRetainedAssessmentCrossLink(t *testing.T) {
	s, c, p, dir := graphFixture(t)
	r := s.mongoRows["interpret_report_artifacts"][0]
	var doc bson.D
	if e := bson.Unmarshal(r.BSON, &doc); e != nil {
		t.Fatal(e)
	}
	for i := range doc {
		if doc[i].Key == "assessment_id" {
			doc[i].Value = int64(101)
		}
	}
	raw, e := bson.Marshal(doc)
	if e != nil {
		t.Fatal(e)
	}
	s.mongoRows["interpret_report_artifacts"][0].BSON = raw
	index, e := planCohort(t.Context(), s, c, p, dir, digest(encode(c)), "source")
	if e != nil {
		t.Fatal(e)
	}
	if index.Holds["interpret_report_artifacts:typed_assessment_id_ownership_conflict"] != 1 {
		t.Fatal("equal numeric cross-link cannot be silently owned from Testee alone")
	}
	if len(index.HoldShards) == 0 {
		t.Fatal("component hold must have bounded physical/root mapping")
	}
	var holds graphHoldPage
	ref := index.HoldShards[0]
	if e := readExact(filepath.Join(dir, ref.File), ref.SHA, int64(ref.Bytes), &holds); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, h := range holds.Holds {
		if h.Reason == "interpret_report_artifacts:typed_assessment_id_ownership_conflict" {
			found = h.PhysicalSHA == objectID(r.Object) && len(h.RootIDs) == 1 && h.RootIDs[0] == "1"
		}
	}
	if !found {
		t.Fatal("aggregate count cannot replace exact held component ownership")
	}

}
func TestGraphScopeShardsBoundCountsAndStopOnTamper(t *testing.T) {
	dir := privateTestDir(t)
	index := graphIndex{Counts: map[string]int64{}, Shards: []graphShard{}}
	w := graphWriter{dir: dir, index: &index, protected: map[string]bool{}}
	for i := 1; i <= 2501; i++ {
		o := key("assessment", strconv.Itoa(i))
		if e := w.add(graphObject{Object: o, Links: map[string]string{"id": strconv.Itoa(i)}, References: nil}, 50); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.flush(); e != nil {
		t.Fatal(e)
	}
	if len(index.Shards) != 3 || index.Shards[0].Count != 1000 || index.Shards[2].Count != 501 {
		t.Fatal("SQL identities must be aggregated into bounded shard files")
	}
	ids := 0
	if e := graphIDs(dir, &index, "mysql", "assessment", "id", 500, func(batch []string) error {
		if len(batch) > 500 {
			t.Fatal("parent query batch overflow")
		}
		ids += len(batch)
		return nil
	}); e != nil || ids != 2501 {
		t.Fatal("bounded parent traversal lost scope")
	}
	file := filepath.Join(dir, index.Shards[0].File)
	if e := os.WriteFile(file, []byte(`{}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readGraphPage(dir, index.Shards[0]); e == nil {
		t.Fatal("raw scope shard drift must reject")
	}
}
func TestGraphDiscoveryFailureCannotPublishIndex(t *testing.T) {
	s, c, p, dir := graphFixture(t)
	s.fail = "assessment"
	index, e := planCohort(t.Context(), s, c, p, dir, digest(encode(c)), "source")
	if e == nil || index.DiscoveryComplete || index.Released {
		t.Fatal("interrupted page is not complete scope")
	}
	if e := readOnlyGraphResult(filepath.Join(dir, "graph-index.json"), index); e == nil {
		t.Fatal("unreleased failed material must not publish success")
	}
	if _, e := os.Stat(filepath.Join(dir, "graph-index.json")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("failed discovery signed an index")
	}
}
func TestGraphMongoBusinessIDsKeepExactActualType(t *testing.T) {
	for _, value := range []any{int32(1), "1", float64(1), int64(-1)} {
		raw, e := bson.Marshal(bson.D{{Key: "testee_id", Value: value}})
		if e != nil {
			t.Fatal(e)
		}
		if _, e := graphMongoID(raw, "testee_id", false); e == nil {
			t.Fatalf("type %T must not coerce into observed int64 identity", value)
		}
	}
}

func TestGraphDeliveryCamelCaseOriginalIDsAndRetainedMissingRoot(t *testing.T) {
	s, c, _, dir := graphFixture(t)
	index := graphIndex{Counts: map[string]int64{}, Retained: map[string]string{}, Holds: map[string]int64{}, RequiredActions: map[string]int64{}}
	w := graphWriter{dir: dir, index: &index, protected: map[string]bool{}}
	planner := cohortPlanner{context: t.Context(), reader: s, selector: c, writer: &w, dir: dir}
	makeRow := func(id, testee, assessment string) record {
		r := graphSQLFixture("event_delivery_dead_letter", id, map[string]string{"org_id": "1"})
		payload := `{"id":"evt-` + id + `","eventType":"evaluation.requested","occurredAt":"2026-01-01T00:00:00Z","aggregateType":"Evaluation","aggregateID":"` + assessment + `","data":{"org_id":1,"testee_id":` + testee + `,"assessment_id":` + assessment + `}}`
		r.Columns = append(r.Columns, cell{Name: "event_id", Type: "VARCHAR", Raw: []byte("evt-" + id)}, cell{Name: "payload_json", Type: "LONGTEXT", Raw: []byte(payload)}, cell{Name: "retry_disposition", Type: "VARCHAR", Raw: []byte("archived_mock")})
		return r
	}
	if e := planner.classifyDeliveries(t.Context(), []record{makeRow("10", "1", "100"), makeRow("11", "999", "999")}, map[string]bool{"1": true}); e != nil {
		t.Fatal(e)
	}
	if index.Counts["mysql/event_delivery_dead_letter"] != 1 || index.Counts["retained/event_delivery_dead_letter/non_target_or_missing_root"] != 1 || len(index.Holds) != 0 {
		t.Fatal("camelCase wire should own only proved current roots; absent historical root stays outside cohort")
	}
	if e := planner.classifyDeliveries(t.Context(), []record{makeRow("12", "1", "101")}, map[string]bool{"1": true}); e == nil {
		t.Fatal("retained Assessment cannot be retired using a conflicting mock Testee")
	}
	big := makeRow("13", "630514119807611438", "630514128649204270")
	candidate, e := deliveryPayload(big)
	if e != nil || candidate.testeeID != "630514119807611438" || candidate.assessmentID != "630514128649204270" || !candidate.recognized {
		t.Fatal("original 64-bit JSON IDs must not round through float64")
	}
}

func (s *graphFake) retainedCounts(_ context.Context, org string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, name := range []string{"event_delivery_dead_letter", "retry_event_hold", "rm_outbox", "qs_rm_replay_requests", "qs_rm_replay_items"} {
		for _, r := range s.sqlRows[name] {
			if v, ok := graphSQLValue(r, "org_id"); ok && v != org {
				continue
			}
			state, _ := graphSQLValue(r, "state")
			if state == "" {
				state, _ = graphSQLValue(r, "status")
			}
			out["mysql/"+name+"/all_org/"+state]++
		}
	}
	return out, nil
}
