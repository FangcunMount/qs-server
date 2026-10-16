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
	if strings.Contains(where, "x.org_id") && name == "interpretation_attention_projection" {
		return errors.New("unknown physical org_id column")
	}
	for _, r := range s.sqlRows[name] {
		if strings.HasPrefix(where, "x.id IN") && name != "testee" {
			id, _ := graphSQLValue(r, "id")
			match := false
			for _, a := range args {
				if a == id {
					match = true
				}
			}
			if !match {
				continue
			}
		} else if name == "assessment" {
			testee, _ := graphSQLValue(r, "testee_id")
			if testee != "1" {
				continue
			}
		}
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
			v := bson.Raw(r.BSON).Lookup(filter[0].Key)
			if v.Type != bson.TypeInt64 {
				continue
			}
			ids := filter[0].Value.(bson.D)[0].Value.(bson.A)
			found := false
			for _, id := range ids {
				if id == v.Int64() {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		if e := emit(r); e != nil {
			return e
		}
	}
	return nil
}
func graphSQLFixture(name, id string, fields map[string]string) record {
	r := record{Object: key(name, id), Definition: []byte("fixed original ddl/session"), Columns: []cell{{Name: "id", Type: "UNSIGNED BIGINT", Raw: []byte(id)}}}
	for _, name := range []string{"org_id", "testee_id", "source", "assessment_id", "outcome_id", "plan_id", "enrollment_id", "status"} {
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
	c := cohortSelector{Format: 1, Kind: "mock_cohort_selector", Operation: "graph-test", OwnerEvidenceSHA: strings.Repeat("a", 64), ProtectionSHA: digest(encode(p)), Binding: binding{SQLUUID: "fixture", SQLDatabase: "qs", MongoIdentity: strings.Repeat("b", 64), MongoDatabase: "qs", SQLVersion: 101, MongoVersion: 39}, Selection: selection{Source: "daily_simulation", OrgID: "1", From: "2025-01-01T00:00:00+08:00", Through: "2026-10-01T00:00:00+08:00", Evidence: "owner confirmed dedicated fixed roots"}, TesteeIDs: []string{"1"}, RetainPlanIDs: []string{"500"}}
	s := &graphFake{sqlRows: map[string][]record{}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
	s.sqlRows["testee"] = []record{graphSQLFixture("testee", "1", map[string]string{"org_id": "1", "source": "daily_simulation"})}
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
	s.mongoRows["report_generations"] = []record{graphMongoFixture(t, "report_generations", bson.D{{Key: "domain_id", Value: int64(800)}, {Key: "outcome_id", Value: int64(200)}, {Key: "latest_run_id", Value: int64(900)}, {Key: "report_id", Value: int64(700)}})}
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
	for _, name := range []string{"answersheets", "interpret_report_artifacts", "report_query_catalog"} {
		if s.mongoRootScans[name] != 1 {
			t.Fatal("unindexed root lookup must scan each collection only once")
		}
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
	if index.Holds["interpret_report_artifacts:assessment_ownership_conflict"] != 1 {
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
		if h.Reason == "interpret_report_artifacts:assessment_ownership_conflict" {
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
		o.Fingerprint = strings.Repeat("a", 64)
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
