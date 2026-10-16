package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This reader is only for typed discovery and reverse-reference checks. Its
// projected records must never be used as original-value archives/fingerprints.
type leanGraphReader struct{ store *graphStore }

func newLeanReader(s *graphStore) *leanGraphReader { return &leanGraphReader{store: s} }

func (r *leanGraphReader) sqlProjection(ctx context.Context, name string) (string, error) {
	columns, ok := map[string][]string{
		"testee":                              {"id", "org_id", "profile_id", "source", "created_at"},
		"clinician_relation":                  {"id", "org_id", "testee_id", "clinician_id", "source_type", "source_id"},
		"testee_store_history":                {"id", "org_id", "testee_id"},
		"assessment_entry_intake_log":         {"id", "org_id", "testee_id", "clinician_id", "entry_id"},
		"answering_start":                     {"id", "org_id", "testee_id", "started_by_user_id"},
		"plan_enrollment":                     {"id", "org_id", "testee_id", "plan_id", "status"},
		"assessment_task":                     {"id", "org_id", "testee_id", "plan_id", "enrollment_id", "assessment_id", "status"},
		"assessment":                          {"id", "org_id", "testee_id", "answer_sheet_id", "status"},
		"evaluation_outcome":                  {"id", "org_id", "testee_id", "assessment_id", "evaluation_run_id", "committed_event_id", "committed_event_evidence"},
		"assessment_score":                    {"id", "testee_id", "assessment_id", "evaluation_outcome_id"},
		"runtime_checkpoint":                  {"id", "scope", "resource_id", "assessment_id", "event_type", "status", "attempt_no", "claim_token", "lease_expires_at", "next_attempt_at", "retry_event_id", "retry_disposition"},
		"interpretation_attention_projection": {"event_id", "report_id", "assessment_id", "testee_id", "status"},
		"interpretation_admission_failure":    {"id", "org_id", "testee_id", "assessment_id", "outcome_id", "generation_id", "event_id", "kind", "code", "decision"},
		"qs_rm_evaluation_request_ref":        {"event_id", "assessment_id", "org_id"},
		"task_opened_reminder_batch":          {"id", "org_id", "task_id", "opening_event_id", "state", "recipient_count"},
		"task_opened_reminder_delivery":       {"id", "org_id", "task_id", "opening_event_id", "user_id", "login_identity_id", "state", "claim_token", "lease_until"},
		"event_delivery_dead_letter":          {"id", "org_id", "message_id", "event_id", "retry_disposition", "replay_request_id"},
		"retry_event_hold":                    {"id", "org_id", "message_id", "event_id", "status", "retry_disposition", "claim_token", "claim_expires_at", "next_attempt_at", "manual_replay_request_id"},
		"qs_rm_replay_requests":               {"org_id", "request_id", "store_name"},
		"qs_rm_replay_items":                  {"org_id", "request_id", "ordinal", "event_id", "authorized", "reason"},
		"rm_outbox":                           {"id", "producer", "message_id", "destination", "event_type", "schema_version", "scope", "state", "claim_token", "lease_until", "manual_replay_request_id"},
		"ai_bridge_requests":                  {"request_id", "organization_id", "testee_id", "status", "session_id"},
		"statistics_access_fact":              {"id", "org_id", "testee_id", "source_type", "source_ref", "stat_date", "clinician_id", "entry_id"},
		"statistics_assessment_fact":          {"id", "org_id", "testee_id", "filler_id", "source_type", "source_ref", "stat_date", "answersheet_id", "assessment_id", "outcome_id", "report_id", "plan_id", "enrollment_id", "task_id"},
		"statistics_plan_fact":                {"id", "org_id", "testee_id", "source_type", "source_ref", "stat_date", "plan_id", "enrollment_id", "task_id"},
		"statistics_store_activity_fact":      {"id", "org_id", "source_type", "source_ref", "stat_date", "fact_type", "business_object_id", "conducting_store_id"},
		"operators":                           {"id", "org_id", "user_id"},
	}[name]
	if r == nil || r.store == nil {
		return "", errors.New("SQL namespace outside fixed lean technical catalog")
	}
	if !ok {
		if !readableSQL[name] {
			return "", errors.New("SQL namespace outside fixed lean technical catalog")
		}
		shape, err := r.store.shape(ctx, name)
		if err != nil {
			return "", err
		}
		for _, key := range shape.keys {
			columns = append(columns, key.Name)
		}
	}
	selected := make([]string, 0, len(columns)+12)
	for _, column := range columns {
		selected = append(selected, "x.`"+column+"`")
	}
	if name == "ai_bridge_requests" {
		selected = append(selected, "x.organization_id AS `org_id`")
	}
	if name == "event_delivery_dead_letter" || name == "retry_event_hold" {
		// Only the fixed domain-wire identity fields leave MySQL, never payload_json.
		// Physical org_id stays separate; JSON_TYPE preserves string/number/null.
		for _, field := range []struct{ path, alias string }{
			{"$.data.testee_id", "testee_id"}, {"$.data.assessment_id", "assessment_id"}, {"$.data.org_id", "wire_org_id"},
			{"$.id", "wire_event_id"}, {"$.eventType", "wire_event_type"}, {"$.aggregateType", "wire_aggregate_type"}, {"$.aggregateId", "wire_aggregate_id"},
		} {
			value := "JSON_EXTRACT(x.payload_json,'" + field.path + "')"
			selected = append(selected, "JSON_UNQUOTE("+value+") AS `"+field.alias+"`", "JSON_TYPE("+value+") AS `"+field.alias+"_json_type`")
		}
	}
	return strings.Join(selected, ","), nil
}

func (r *leanGraphReader) sqlPages(ctx context.Context, name, where string, args []any, emit func(record) error) error {
	columns, e := r.sqlProjection(ctx, name)
	if e != nil {
		return e
	}
	return r.store.sqlPagesSelected(ctx, name, where, args, columns, emit)
}

func leanStatisticsCursorTable(name string) bool {
	return name == "statistics_access_fact" || name == "statistics_assessment_fact" || name == "statistics_plan_fact"
}

// Statistics source batches and directed parent intersections send their fixed
// predicates once, rather than repeating selection/sorting for each keyset page.
// The callback only checks in-memory identities/holds; it must not issue another
// query on this open cursor's connection.
func (r *leanGraphReader) sqlOneCursor(ctx context.Context, name, where string, args []any, emit func(record) error) (result error) {
	if !leanStatisticsCursorTable(name) {
		return errors.New("single-cursor query outside fixed statistics namespaces")
	}
	columns, e := r.sqlProjection(ctx, name)
	if e != nil {
		return e
	}
	shape, e := r.store.shape(ctx, name)
	if e != nil {
		return e
	}
	order := make([]string, 0, len(shape.keys))
	for _, key := range shape.keys {
		order = append(order, "x.`"+key.Name+"`")
	}
	rows, e := r.store.live.readSQL().QueryContext(ctx, "SELECT "+columns+" FROM `"+name+"` x WHERE "+where+" ORDER BY "+strings.Join(order, ","), args...)
	if e != nil {
		return e
	}
	defer func() { result = errors.Join(result, rows.Err(), rows.Close()) }()
	names, e := rows.Columns()
	if e != nil {
		return e
	}
	types, e := rows.ColumnTypes()
	if e != nil {
		return e
	}
	page := make([]record, 0, 1000)
	pageBytes := 0
	flush := func() error {
		for _, row := range page {
			if e := ctx.Err(); e != nil {
				return e
			}
			if e := emit(row); e != nil {
				return e
			}
		}
		clear(page)
		page = page[:0]
		pageBytes = 0
		return nil
	}
	for rows.Next() {
		row, e := scanGraphSQLRow(rows, names, types, shape, name)
		if e != nil {
			return e
		}
		n := len(encode(row))
		if len(page) == 1000 || pageBytes+n > 64*1024*1024 {
			if e = flush(); e != nil {
				return e
			}
		}
		page = append(page, row)
		pageBytes += n
	}
	if e = errors.Join(rows.Err(), ctx.Err()); e != nil {
		return e
	}
	return flush()
}

func (r *leanGraphReader) mongoPages(ctx context.Context, name string, filter bson.D, emit func(record) error) error {
	if r == nil || r.store == nil {
		return errors.New("lean Mongo store unavailable")
	}
	var columns []string
	switch name {
	case "answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts", "report_query_catalog":
		columns = []string{"_id", "domain_id", "org_id", "testee_id", "filler_id", "assessment_id", "outcome_id", "generation_id", "interpretation_run_id", "latest_run_id", "report_id", "source_kind", "source_id", "sort_report_id", "task_id", "status", "state", "retry_disposition", "lease_expires_at", "next_attempt_at", "generated_event_id", "retry_event_id", "admission.purpose", "start_context.id", "attribution.task_id", "attribution.enrollment_id", "attribution.plan_id", "generated_event_evidence", "retry_event_evidence", "durable_acceptance.event_id", "durable_acceptance.event_evidence"}
	case "rm_outbox":
		columns = []string{"_id", "producer", "message_id", "destination", "event_type", "schema_version", "scope", "state", "claim_token", "lease_until", "manual_replay_request_id"}
	case "qs_rm_replay_requests":
		columns = []string{"_id", "org_id", "request_id", "store_name", "items.event_id", "items.authorized", "items.ordinal"}
	default:
		return errors.New("mongo namespace outside fixed lean technical catalog")
	}
	projection := make(bson.D, 0, len(columns))
	for _, column := range columns {
		projection = append(projection, bson.E{Key: column, Value: 1})
	}
	if name == "rm_outbox" || name == "qs_rm_replay_requests" {
		return r.retainedMongoPages(ctx, name, filter, projection, emit)
	}
	return r.store.mongoPagesSelected(ctx, name, filter, projection, emit)
}

func (r *leanGraphReader) sqlOwnerPages(ctx context.Context, name string, ids []string, emit func(record) error) error {
	if r == nil || r.store == nil {
		return errors.New("lean SQL owner store unavailable")
	}
	return r.store.sqlOwnerPages(ctx, name, ids, emit)
}
func (r *leanGraphReader) mongoOwnerPages(ctx context.Context, name string, ids []string, emit func(record) error) error {
	if r == nil || r.store == nil {
		return errors.New("lean Mongo owner store unavailable")
	}
	return r.store.mongoOwnerPages(ctx, name, ids, emit)
}

// Counts are diagnostics for one whole org, not target/deletion authority. No
// settled wire or business body is streamed merely to report its state count.
func (r *leanGraphReader) retainedCounts(ctx context.Context, org string) (map[string]int64, error) {
	if err := canonicalIDs([]string{org}, false); err != nil {
		return nil, err
	}
	if r == nil || r.store == nil || r.store.live == nil {
		return nil, errors.New("lean retained-count store unavailable")
	}
	counts := map[string]int64{}
	for _, fixed := range []struct{ name, state, predicate string }{
		{"interpretation_attention_projection", "COALESCE(x.status,'null')", "1=1"},
		{"task_opened_reminder_batch", "COALESCE(x.state,'null')", "x.org_id=?"},
		{"task_opened_reminder_delivery", "COALESCE(x.state,'null')", "x.org_id=?"},
		{"event_delivery_dead_letter", "COALESCE(x.retry_disposition,'null')", "x.org_id=?"},
		{"retry_event_hold", "CONCAT(COALESCE(x.status,'null'),'/',COALESCE(x.retry_disposition,'null'))", "x.org_id=?"},
		{"rm_outbox", "COALESCE(x.state,'null')", "x.scope=?"},
		{"qs_rm_replay_requests", "'all'", "x.org_id=?"},
		{"qs_rm_replay_items", "'all'", "x.org_id=?"},
	} {
		if _, err := r.store.shape(ctx, fixed.name); err != nil {
			return nil, err
		}
		arg := org
		if fixed.name == "rm_outbox" {
			arg = "org:" + org
		}
		args, scope := []any{arg}, "all_org"
		if fixed.name == "interpretation_attention_projection" {
			// No org column exists, and a retained guard may outlive its Testee.
			args, scope = nil, "all_store"
		}
		rows, err := r.store.live.readSQL().QueryContext(ctx, "SELECT "+fixed.state+",COUNT(*) FROM `"+fixed.name+"` x WHERE "+fixed.predicate+" GROUP BY "+fixed.state, args...)
		if err != nil {
			return nil, err
		}
		prefix := "retained/" + scope + "/mysql/" + fixed.name
		counts[prefix+"/total"] = 0
		n := 0
		for rows.Next() {
			var state string
			var count int64
			if err = rows.Scan(&state, &count); err != nil {
				return nil, errors.Join(err, rows.Close())
			}
			n++
			if n > 256 || len(state) > 128 || strings.ContainsAny(state, "\x00\r\n") || count < 0 {
				return nil, errors.Join(errors.New("retained SQL state aggregation exceeds fixed shape"), rows.Close())
			}
			counts[prefix+"/state/"+state] = count
			counts[prefix+"/total"] += count
		}
		if err = errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, err
		}
	}
	orgID, _ := strconv.ParseInt(org, 10, 64) // canonicalIDs already bounds int64.
	for _, name := range []string{"rm_outbox", "qs_rm_replay_requests"} {
		if _, err := r.store.mongoDefinition(ctx, name); err != nil {
			return nil, err
		}
		match := bson.D{{Key: "org_id", Value: orgID}}
		group := bson.D{{Key: "_id", Value: "all"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}
		if name == "rm_outbox" {
			match = bson.D{{Key: "scope", Value: "org:" + org}}
			group[0].Value = "$state"
		}
		cur, err := r.store.live.mongo.Database(r.store.live.mongoDB).Collection(name).Aggregate(ctx, bson.A{bson.D{{Key: "$match", Value: match}}, bson.D{{Key: "$group", Value: group}}}, options.Aggregate().SetBatchSize(32).SetAllowDiskUse(false))
		if err != nil {
			return nil, err
		}
		prefix := "retained/all_org/mongo/" + name
		counts[prefix+"/total"] = 0
		n := 0
		for cur.Next(ctx) {
			var row struct {
				State string `bson:"_id"`
				Count int64  `bson:"count"`
			}
			if err = cur.Decode(&row); err != nil {
				return nil, errors.Join(err, cur.Close(ctx))
			}
			n++
			if n > 256 || row.State == "" || len(row.State) > 64 || strings.ContainsAny(row.State, "\x00\r\n") || row.Count < 0 {
				return nil, errors.Join(errors.New("retained Mongo state aggregation exceeds fixed shape"), cur.Close(ctx))
			}
			counts[prefix+"/state/"+row.State] = row.Count
			counts[prefix+"/total"] += row.Count
		}
		if err = errors.Join(cur.Err(), cur.Close(ctx)); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

// RM uses a composite BSON _id. This identity is retained and read-only: it is
// never admitted to the raw mutation key codec or a target/archive scope.
func (r *leanGraphReader) retainedMongoPages(ctx context.Context, name string, filter, projection bson.D, emit func(record) error) error {
	definition, e := r.store.mongoDefinition(ctx, name)
	if e != nil {
		return e
	}
	cur, e := r.store.live.mongo.Database(r.store.live.mongoDB).Collection(name).Find(ctx, filter, options.Find().SetProjection(projection).SetBatchSize(500).SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}))
	if e != nil {
		return e
	}
	for cur.Next(ctx) {
		raw := append(bson.Raw{}, cur.Current...)
		id, e := raw.LookupErr("_id")
		if e != nil {
			return errors.Join(e, cur.Close(ctx))
		}
		if e = id.Validate(); e != nil {
			return errors.Join(e, cur.Close(ctx))
		}
		if id.Type == bson.TypeNull || id.Type == bson.TypeUndefined {
			return errors.Join(errors.New("retained Mongo identity is null"), cur.Close(ctx))
		}
		o := object{Store: "mongo", Name: name, Key: []cell{{Name: "_id", Type: fmt.Sprintf("bson:%02x", byte(id.Type)), Raw: append([]byte{}, id.Value...)}}}
		if e = validateObject(o); e != nil {
			return errors.Join(e, cur.Close(ctx))
		}
		if e = emit(record{Object: o, BSON: raw, Definition: definition}); e != nil {
			return errors.Join(e, cur.Close(ctx))
		}
	}
	return errors.Join(cur.Err(), cur.Close(ctx), ctx.Err())
}
