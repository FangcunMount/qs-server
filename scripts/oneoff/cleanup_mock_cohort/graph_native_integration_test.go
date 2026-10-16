//go:build integration

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// This is a real-provider, read-only graph qualification. It does not exercise
// whole-cohort deletion, message settlement, IAM cleanup or production approval.
// The original primitive native top remains independently selected and intact.
type cohortGraphNative struct {
	h         *cohortNative
	selector  cohortSelector
	protected protected
	catalogID primitive.ObjectID
}

func seedCohortGraphNative(t *testing.T) cohortGraphNative {
	t.Helper()
	h := newCohortNative(t)
	for _, q := range []string{
		"INSERT INTO testee(id,org_id,name,gender,source,tags,created_at) VALUES(42,1,'synthetic-system',1,'retained',JSON_ARRAY(),'2026-01-01 12:00:00'),(43,1,'synthetic-clack',1,'retained',JSON_ARRAY(),'2026-01-01 12:00:00'),(44,1,'synthetic-cohort',1,'daily_simulation',JSON_ARRAY(),'2026-01-01 12:00:00'),(45,1,'synthetic-real',1,'retained',JSON_ARRAY(),'2026-01-01 12:00:00')",
		"INSERT INTO assessment_plan(id,org_id,scale_code,schedule_type,total_times) VALUES(500,1,'synthetic','by_day',1)",
		"INSERT INTO plan_enrollment(id,org_id,plan_id,testee_id,round,start_date,status,joined_at) VALUES(501,1,500,44,1,'2026-01-01','closed','2026-01-01'),(502,1,500,45,1,'2026-01-01','closed','2026-01-01')",
		"INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status) VALUES(101,1,44,'synthetic','v1',701,'plan','completed'),(102,1,45,'synthetic','v1',702,'plan','completed')",
		"INSERT INTO assessment_task(id,plan_id,enrollment_id,seq,org_id,testee_id,scale_code,planned_at,status,assessment_id) VALUES(601,500,501,1,1,44,'synthetic','2026-01-01','completed',101),(602,500,502,1,1,45,'synthetic','2026-01-01','completed',102)",
		"INSERT INTO evaluation_outcome(id,org_id,assessment_id,testee_id,evaluation_run_id,model_kind,model_code,model_version,payload_json,evaluated_at) VALUES(201,1,101,44,'native-run-101','scale','synthetic','v1','{}','2026-01-01'),(202,1,102,45,'native-run-102','scale','synthetic','v1','{}','2026-01-01')",
		"INSERT INTO clinician(id,org_id,name,clinician_type) VALUES(600,1,'shared-synthetic-clinician','synthetic')",
		"INSERT INTO assessment_entry(id,org_id,clinician_id,token,target_type,target_code) VALUES(800,1,600,'native-shared-entry-token','questionnaire','synthetic')",
		"INSERT INTO clinician_relation(id,org_id,clinician_id,testee_id,relation_type,source_type,bound_at) VALUES(801,1,600,44,'synthetic','entry','2026-01-01'),(802,1,600,45,'synthetic','entry','2026-01-01')",
		"INSERT INTO testee_store_history(id,org_id,testee_id,to_store_id,kind,actor_id,created_at,reason,request_id,version) VALUES(803,1,44,900,'synthetic',800,'2026-01-01','synthetic','native-store',1)",
		"INSERT INTO assessment_entry_intake_log(id,org_id,clinician_id,entry_id,testee_id,intake_at) VALUES(804,1,600,800,44,'2026-01-01')",
		"INSERT INTO answering_start(id,org_id,started_by_user_id,testee_id,request_key,request_hash,questionnaire_code,questionnaire_version,origin_type,started_at,ownership_version,contract_version) VALUES(805,1,800,44,'native-start',REPEAT('a',64),'synthetic','v1','adhoc','2026-01-01',1,1)",
		"INSERT INTO runtime_checkpoint(id,scope,resource_id,assessment_id,status,started_at,finished_at) VALUES(806,'evaluation_run','native-run-101',101,'succeeded','2026-01-01','2026-01-01')",
		"INSERT INTO interpretation_attention_projection(event_id,report_id,assessment_id,testee_id,risk_level,status,created_at,updated_at) VALUES('native-attention','501','101',44,'low','complete','2026-01-01','2026-01-01')",
		"INSERT INTO interpretation_admission_failure(id,outcome_id,org_id,assessment_id,testee_id,kind,code,safe_message,fingerprint,decision,first_failed_at,last_failed_at,occurred_at) VALUES(807,201,1,101,44,'synthetic','synthetic','synthetic','native-admission','closed','2026-01-01','2026-01-01','2026-01-01')",
		"INSERT INTO task_opened_reminder_batch(id,org_id,task_id,opening_event_id,schedule_revision,reminder_version,app_id,template_id,recipient_count,state,created_at,updated_at) VALUES(808,1,'601','native-opened',1,1,'synthetic','synthetic',1,'complete','2026-01-01','2026-01-01')",
		"INSERT INTO task_opened_reminder_delivery(id,org_id,task_id,opening_event_id,schedule_revision,user_id,login_identity_id,app_id,template_id,reminder_version,state,created_at,updated_at) VALUES(809,1,'601','native-opened',1,'800','801','synthetic','synthetic',1,'complete','2026-01-01','2026-01-01')",
		"INSERT INTO qs_rm_evaluation_request_ref(event_id,assessment_id,org_id) VALUES('native-evaluation-request',101,1)",
		"INSERT INTO statistics_access_fact(id,org_id,fact_key,core_hash,fact_type,occurred_at,stat_date,source_type,source_ref,testee_id) VALUES(810,1,'native-access',REPEAT('a',64),'intake_confirmed','2026-01-01','2026-01-01','entry_intake','804',44)",
		"INSERT INTO statistics_assessment_fact(id,org_id,fact_key,core_hash,fact_type,occurred_at,stat_date,source_type,source_ref,testee_id,assessment_id,outcome_id) VALUES(811,1,'native-assessment',REPEAT('b',64),'assessment_completed','2026-01-01','2026-01-01','evaluation_outcome','201',44,101,201)",
		"INSERT INTO statistics_plan_fact(id,org_id,fact_key,core_hash,fact_type,occurred_at,stat_date,source_type,source_ref,plan_id,enrollment_id,testee_id,task_id) VALUES(812,1,'native-plan',REPEAT('c',64),'task_completed','2026-01-01','2026-01-01','assessment_task','601',500,501,44,601)",
		"INSERT INTO statistics_store_activity_fact(id,org_id,fact_key,fact_type,business_object_id,occurred_at,stat_date,source_type,source_ref,schema_version,core_hash) VALUES(813,1,'native-sheet-store','answersheet_submitted',700,'2026-01-01','2026-01-01','answersheet_submitted','700',1,REPEAT('d',64)),(814,1,'native-assessment-store','assessment_completed',101,'2026-01-01','2026-01-01','assessment_completed','101',1,REPEAT('e',64))",
	} {
		h.exec(q)
	}
	// Cross a real SQL LIMIT 1000 boundary without holding a large business body.
	values, args := []string{}, []any{}
	for id := 1; id <= 1001; id++ {
		values = append(values, "(?,101,201,44,'synthetic','synthetic',2.5,'low','2026-01-01','2026-01-01')")
		args = append(args, id)
	}
	h.exec("INSERT INTO assessment_score(id,assessment_id,evaluation_outcome_id,testee_id,factor_code,factor_name,raw_score,risk_level,created_at,updated_at) VALUES "+strings.Join(values, ","), args...)

	db := h.client.Database(h.name)
	retainedSheet := primitive.NewObjectID()
	sheets := []any{
		bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: int64(700)}, {Key: "testee_id", Value: int64(44)}, {Key: "org_id", Value: int64(1)}, {Key: "filler_id", Value: int64(800)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "independent_questionnaire"}}}, {Key: "start_context", Value: bson.D{{Key: "id", Value: int64(805)}}}},
		bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: int64(701)}, {Key: "testee_id", Value: int64(44)}, {Key: "org_id", Value: int64(1)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "assessment"}}}, {Key: "attribution", Value: bson.D{{Key: "task_id", Value: "601"}, {Key: "enrollment_id", Value: "501"}, {Key: "plan_id", Value: "500"}}}},
		bson.D{{Key: "_id", Value: retainedSheet}, {Key: "domain_id", Value: int64(702)}, {Key: "testee_id", Value: int64(45)}, {Key: "org_id", Value: int64(1)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "assessment"}}}},
	}
	// Cross a real 500-document cursor/shard boundary, including independent
	// AnswerSheets with no Assessment row. Physical _id is deliberately ObjectID.
	for id := int64(10000); id < 10499; id++ {
		sheets = append(sheets, bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: id}, {Key: "testee_id", Value: int64(44)}, {Key: "org_id", Value: int64(1)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "independent_questionnaire"}}}})
	}
	if _, err := db.Collection("answersheets").InsertMany(t.Context(), sheets); err != nil {
		t.Fatal("owned graph AnswerSheet seed failed:", err)
	}
	catalogID := primitive.NewObjectID()
	for name, doc := range map[string]bson.D{
		"report_generations":         {{Key: "domain_id", Value: int64(301)}, {Key: "outcome_id", Value: int64(201)}, {Key: "latest_run_id", Value: int64(401)}, {Key: "report_id", Value: int64(501)}},
		"interpretation_runs":        {{Key: "domain_id", Value: int64(401)}, {Key: "generation_id", Value: int64(301)}, {Key: "status", Value: "succeeded"}},
		"interpret_report_artifacts": {{Key: "domain_id", Value: int64(501)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(44)}, {Key: "assessment_id", Value: int64(101)}, {Key: "outcome_id", Value: int64(201)}, {Key: "generation_id", Value: int64(301)}, {Key: "interpretation_run_id", Value: int64(401)}},
		"report_query_catalog":       {{Key: "_id", Value: catalogID}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(44)}, {Key: "assessment_id", Value: int64(101)}, {Key: "outcome_id", Value: int64(201)}, {Key: "generation_id", Value: int64(301)}, {Key: "source_kind", Value: "artifact"}, {Key: "source_id", Value: int64(501)}, {Key: "sort_report_id", Value: int64(501)}},
	} {
		if _, err := db.Collection(name).InsertOne(t.Context(), doc); err != nil {
			t.Fatal("owned graph report seed failed:", name, err)
		}
	}
	for _, tc := range []struct{ id, testee, assessment string }{{"native-target-wire", "44", "101"}, {"native-retained-wire", "45", "102"}} {
		wire, err := json.Marshal(domainwire.Envelope{ID: tc.id, EventType: "evaluation.requested", OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), AggregateType: "Evaluation", AggregateID: tc.assessment, Data: json.RawMessage(fmt.Sprintf(`{"org_id":1,"testee_id":"%s","assessment_id":"%s"}`, tc.testee, tc.assessment))})
		if err != nil {
			t.Fatal(err)
		}
		// The actual historical domain envelope uses camelCase eventType etc.
		// Matching topic, date or a numeric event string alone is not ownership.
		h.exec("INSERT INTO event_delivery_dead_letter(message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,failed_at) VALUES(?,?,1,'synthetic','synthetic','synthetic',1,?,'2026-01-01')", tc.id, tc.id, wire)
		h.exec("INSERT INTO retry_event_hold(event_id,message_id,org_id,provider,topic_name,channel_name,payload_json,original_delivery_attempt,blocked_reason,blocked_at) VALUES(?,?,1,'synthetic','synthetic','synthetic',?,1,'synthetic','2026-01-01')", tc.id, tc.id, wire)
	}
	p := protected{Format: 1, Kind: "mock_cohort_protected", IAM: []byte(`{"user_ids":[],"profile_ids":[],"login_identity_ids":[]}`), QS: protectionScope{System: []object{keySQL("testee", "42")}, Clack: []object{keySQL("testee", "43")}, Other: []object{keySQL("testee", "45"), keySQL("assessment_plan", "500"), keySQL("plan_enrollment", "502"), keySQL("assessment", "102"), keySQL("clinician", "600"), keySQL("assessment_entry", "800")}}}
	retained := nativeMongoObject(retainedSheet)
	retained.Name = "answersheets"
	p.QS.Other = append(p.QS.Other, retained)
	c := cohortSelector{Format: 1, Kind: "mock_cohort_selector", Operation: "native-graph-" + strings.TrimPrefix(h.name, "qs_mock_cohort_"), OwnerEvidenceSHA: strings.Repeat("a", 64), ProtectionSHA: digest(encode(p)), Binding: h.binding, Selection: selection{Source: "daily_simulation", OrgID: "1", From: "2026-01-01T00:00:00+08:00", Through: "2026-01-02T00:00:00+08:00", Evidence: "owner-confirmed synthetic graph roots; no production deletion authority"}, TesteeIDs: []string{"44"}, RetainPlanIDs: []string{"500"}}
	return cohortGraphNative{h, c, p, catalogID}
}

// Whole rows/raw BSON are hashed incrementally and never logged. A fresh store
// is used each time so namespace definitions are read again, not cache-compared.
func graphNativeSnapshot(t *testing.T, h *cohortNative) map[string]string {
	t.Helper()
	store := newGraphStore(h.store)
	out := map[string]string{}
	names := make([]string, 0, len(graphSQLTables))
	for name := range graphSQLTables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		hash := sha256.New()
		if err := store.sqlPages(t.Context(), name, "1=1", nil, func(r record) error { _, _ = hash.Write(encode(r)); return nil }); err != nil {
			t.Fatal("owned graph SQL snapshot failed:", name, err)
		}
		out["mysql/"+name] = hex.EncodeToString(hash.Sum(nil))
	}
	for _, name := range []string{"answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts", "report_query_catalog"} {
		hash := sha256.New()
		if err := store.mongoPages(t.Context(), name, bson.D{}, func(r record) error { _, _ = hash.Write(encode(r)); return nil }); err != nil {
			t.Fatal("owned graph Mongo snapshot failed:", name, err)
		}
		out["mongo/"+name] = hex.EncodeToString(hash.Sum(nil))
	}
	return out
}

func TestMockCohortGraphNativeOwnedDiscovery(t *testing.T) {
	f := seedCohortGraphNative(t)
	h := f.h
	before := graphNativeSnapshot(t, h)
	guards, err := observeGraphProtection(t.Context(), h.store, f.protected)
	if err != nil {
		t.Fatal(err)
	}
	dir := privateTestDir(t)
	store := newGraphStore(h.store)
	index, err := planCohort(t.Context(), store, f.selector, f.protected, dir, digest(encode(f.selector)), toolSource)
	if err != nil {
		t.Fatal("real-provider graph discovery failed:", err)
	}
	if !index.ReadOnly || !index.DiscoveryComplete || !index.WritePending || index.Released || len(index.Holds) != 0 {
		t.Fatalf("readonly graph authority/hold mismatch: complete=%t write_pending=%t released=%t holds=%v", index.DiscoveryComplete, index.WritePending, index.Released, index.Holds)
	}
	for _, name := range []string{"testee", "clinician_relation", "testee_store_history", "assessment_entry_intake_log", "answering_start", "plan_enrollment", "assessment_task", "assessment", "evaluation_outcome", "runtime_checkpoint", "interpretation_attention_projection", "interpretation_admission_failure", "task_opened_reminder_batch", "task_opened_reminder_delivery", "qs_rm_evaluation_request_ref", "statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact", "event_delivery_dead_letter", "retry_event_hold"} {
		if got := index.Counts["mysql/"+name]; got != 1 {
			t.Fatalf("real graph branch %s: got %d want 1", name, got)
		}
	}
	for name, want := range map[string]int64{"mysql/assessment_score": 1001, "mysql/statistics_store_activity_fact": 2, "mongo/answersheets": 501, "mongo/report_generations": 1, "mongo/interpretation_runs": 1, "mongo/interpret_report_artifacts": 1, "mongo/report_query_catalog": 1} {
		if index.Counts[name] != want {
			t.Fatalf("bounded real graph count %s: got %d want %d", name, index.Counts[name], want)
		}
	}
	if index.Retained["assessment_plan/500"] == "" || index.RequiredActions["settle_event_delivery_dead_letter"] != 1 || index.RequiredActions["settle_retry_event_hold"] != 1 {
		t.Fatal("shared Plan or unresolved historical responsibility was promoted to write authority")
	}
	pageCounts := map[string][]int{}
	seen := map[string]bool{}
	independent := false
	for _, shard := range index.Shards {
		page, err := readGraphPage(dir, shard)
		if err != nil {
			t.Fatal(err)
		}
		pageCounts[shard.Store+"/"+shard.Name] = append(pageCounts[shard.Store+"/"+shard.Name], shard.Count)
		for _, o := range page.Objects {
			id := objectID(o.Object)
			if seen[id] || !hashPattern.MatchString(o.Object.Fingerprint) {
				t.Fatal("physical target duplicated or original fingerprint missing")
			}
			seen[id] = true
			if o.Object.Name == "answersheets" && o.Links["domain_id"] == "700" {
				independent = o.Links["purpose"] == "independent_questionnaire" && o.Object.Key[0].Type == "bson:07"
			}
		}
	}
	if !reflect.DeepEqual(pageCounts["mysql/assessment_score"], []int{1000, 1}) || !reflect.DeepEqual(pageCounts["mongo/answersheets"], []int{500, 1}) || !independent {
		t.Fatal("real SQL/Mongo page boundary or independent AnswerSheet scope missing")
	}
	if _, err := store.verifyDefinitions(t.Context()); err != nil {
		t.Fatal(err)
	}
	afterGuards, err := observeGraphProtection(t.Context(), h.store, f.protected)
	if err != nil || !reflect.DeepEqual(guards, afterGuards) || !reflect.DeepEqual(before, graphNativeSnapshot(t, h)) {
		t.Fatal("readonly discovery changed original facts or protected/shared objects:", err)
	}
	if err = h.store.identity(t.Context(), h.binding); err != nil {
		t.Fatal(err)
	}

	// Keep a real retained Assessment link contradictory to the selected Testee.
	// The object remains a diagnostic candidate with a blocking component hold.
	col := h.client.Database(h.name).Collection("report_query_catalog")
	if _, err = col.UpdateByID(t.Context(), f.catalogID, bson.D{{Key: "$set", Value: bson.D{{Key: "assessment_id", Value: int64(102)}}}}); err != nil {
		t.Fatal(err)
	}
	index, err = planCohort(t.Context(), newGraphStore(h.store), f.selector, f.protected, privateTestDir(t), digest(encode(f.selector)), toolSource)
	if err != nil || index.Holds["report_query_catalog:assessment_ownership_conflict"] != 1 || !index.WritePending || index.Released {
		t.Fatal("retained cross-link must block whole-graph write qualification:", err)
	}
	// Equal numeric BSON values with a different actual type are not an identity
	// alias. The native driver and actual document must reach the typed parser.
	if _, err = col.UpdateByID(t.Context(), f.catalogID, bson.D{{Key: "$set", Value: bson.D{{Key: "assessment_id", Value: int64(101)}, {Key: "testee_id", Value: int32(44)}}}}); err != nil {
		t.Fatal(err)
	}
	index, err = planCohort(t.Context(), newGraphStore(h.store), f.selector, f.protected, privateTestDir(t), digest(encode(f.selector)), toolSource)
	if err != nil || index.Holds["report_query_catalog:unassignable_testee_identity"] != 1 || index.Counts["mongo/report_query_catalog"] != 0 || !index.WritePending || index.Released {
		t.Fatal("numeric BSON alias must remain held outside writable scope:", err)
	}
	t.Logf("graph_discovery_only=true whole_cohort_write_qualified=false sql_score_shards=%v mongo_answersheet_shards=%v physical_targets=%d", pageCounts["mysql/assessment_score"], pageCounts["mongo/answersheets"], len(seen))
}
