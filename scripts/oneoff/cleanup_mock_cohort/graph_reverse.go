package main

import (
	"context"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/bson"
	"math"
	"sort"
	"strconv"
)

type graphFrozenReferences struct {
	ids      map[string]map[string]bool
	physical map[string]string
}

func graphParentScope(scope string) bool {
	switch scope {
	case "mysql/assessment_entry_intake_log", "mysql/clinician_relation",
		"mysql/plan_enrollment", "mysql/assessment_task", "mysql/answering_start",
		"mysql/assessment", "mysql/evaluation_outcome", "mongo/answersheets",
		"mongo/report_generations", "mongo/interpretation_runs", "mongo/interpret_report_artifacts":
		return true
	}
	return false
}

func frozenGraphReferences(dir string, index graphIndex, roots []string, physical bool) (graphFrozenReferences, error) {
	f := graphFrozenReferences{ids: map[string]map[string]bool{}, physical: map[string]string{}}
	e := forSelectedScope(dir, index, roots, func(page []graphObject, _ int) error {
		for _, o := range page {
			if physical {
				f.physical[objectID(o.Object)] = "planned_identity"
			}
			if o.Object.Name == "runtime_checkpoint" && o.Links["scope"] == "evaluation_run" && o.Links["resource_id"] != "" {
				if f.ids["runtime/evaluation_run"] == nil {
					f.ids["runtime/evaluation_run"] = map[string]bool{}
				}
				f.ids["runtime/evaluation_run"][o.Links["resource_id"]] = true
			}
			scope := o.Object.Store + "/" + o.Object.Name
			id := o.Links["id"]
			if o.Object.Store == "mongo" {
				id = o.Links["domain_id"]
			}
			// Only parent domains participate in residual relationship queries. Score
			// and fact PKs already have exact archive verification; accumulating their
			// millions of IDs again does not add a relationship check.
			if graphParentScope(scope) && graphDecimal(id) {
				if f.ids[scope] == nil {
					f.ids[scope] = map[string]bool{}
				}
				f.ids[scope][id] = true
			}
		}
		return nil
	})
	if e != nil {
		return f, e
	}
	return f, nil
}
func sortedGraphIDs(ids map[string]bool) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Final clearance checks the current relationship graph using the frozen typed
// parents, not only the original physical PKs. New children and foreign-domain
// crosslinks cannot disappear merely because their former SQL parent was deleted.
func graphNoRemainingReferences(ctx context.Context, r graphReader, g group, index graphIndex, scopeDir string) error {
	if store, ok := r.(*graphStore); ok {
		r = newLeanReader(store)
	}
	return graphRelationshipCheck(ctx, r, g, index, scopeDir, false)
}
func graphRelationshipCheck(ctx context.Context, r graphReader, g group, index graphIndex, scopeDir string, allowOriginal bool) error {
	f, e := frozenGraphReferences(scopeDir, index, g.QS.Graph.RootIDs, allowOriginal)
	if e != nil {
		return e
	}
	return graphRelationshipReferences(ctx, r, g, f, allowOriginal)
}

// Clearance merges only frozen typed parent IDs; original materials stay separate.
func graphRelationshipReferences(ctx context.Context, r graphReader, g group, f graphFrozenReferences, allowOriginal bool) error {
	var e error
	roots := graphRootSet(g.QS.Graph.RootIDs)
	var iam iamScope
	if e = json.Unmarshal(g.IAM, &iam); e != nil {
		return e
	}
	users := graphRootSet(iam.Users)
	remains := func(row record) error {
		if allowOriginal && f.physical[objectID(row.Object)] != "" {
			return nil
		}
		return errors.New("new or foreign reverse reference to the exact target graph remains")
	}
	checkSQL := func(table, column string, ids map[string]bool, binary bool) error {
		return rootBatches(sortedGraphIDs(ids), 1000, func(batch []string) error {
			slots, args := placeholders(batch)
			col := "x.`" + column + "`"
			if binary {
				col = "BINARY " + col
			}
			return r.sqlPages(ctx, table, col+" IN ("+slots+")", args, remains)
		})
	}
	for _, table := range []string{"clinician_relation", "testee_store_history", "assessment_entry_intake_log", "answering_start", "plan_enrollment", "assessment_task", "assessment", "evaluation_outcome", "interpretation_admission_failure"} {
		if e = checkSQL(table, "testee_id", roots, false); e != nil {
			return e
		}
	}
	if e = checkSQL("testee", "id", roots, false); e != nil {
		return e
	}
	for _, table := range []string{"assessment_score", "runtime_checkpoint", "evaluation_outcome"} {
		if e = checkSQL(table, "assessment_id", f.ids["mysql/assessment"], false); e != nil {
			return e
		}
	}
	for _, check := range []struct {
		table, column, scope string
		binary               bool
	}{
		{"assessment_task", "assessment_id", "mysql/assessment", false},
		{"assessment_score", "evaluation_outcome_id", "mysql/evaluation_outcome", false},
		{"interpretation_admission_failure", "outcome_id", "mysql/evaluation_outcome", false},
		{"interpretation_admission_failure", "generation_id", "mongo/report_generations", false},
		{"interpretation_admission_failure", "assessment_id", "mysql/assessment", false},
		{"evaluation_outcome", "evaluation_run_id", "runtime/evaluation_run", true},
	} {
		if e = checkSQL(check.table, check.column, f.ids[check.scope], check.binary); e != nil {
			return e
		}
	}
	if e = checkSQL("assessment_task", "enrollment_id", f.ids["mysql/plan_enrollment"], false); e != nil {
		return e
	}
	if e = checkSQL("assessment", "answer_sheet_id", f.ids["mongo/answersheets"], false); e != nil {
		return e
	}
	// The same source_ref decimal means different domains for these fixed types.
	sources := []struct{ source, scope string }{{"entry_intake", "mysql/assessment_entry_intake_log"}, {"clinician_relation", "mysql/clinician_relation"}, {"plan_enrollment", "mysql/plan_enrollment"}, {"assessment_task", "mysql/assessment_task"}, {"answersheet", "mongo/answersheets"}, {"assessment", "mysql/assessment"}, {"evaluation_outcome", "mysql/evaluation_outcome"}, {"interpret_report", "mongo/interpret_report_artifacts"}, {"interpretation_run", "mongo/interpretation_runs"}}
	typedSources := map[string]map[string]bool{}
	for _, source := range sources {
		typedSources[source.source] = f.ids[source.scope]
	}
	// Source domains use their actual composite index. The fallback predicates
	// catch malformed/foreign source links through exact Testee or filler ownership
	// on the server, returning only matches instead of shipping every fact row.
	for _, table := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact"} {
		for kind, ids := range typedSources {
			if e = rootBatches(sortedGraphIDs(ids), 1000, func(batch []string) error {
				slots, args := placeholders(batch)
				args = append([]any{kind, kind}, args...)
				return r.sqlPages(ctx, table, "x.source_type=? AND BINARY x.source_type=BINARY ? AND x.source_ref IN ("+slots+")", args, remains)
			}); e != nil {
				return e
			}
		}
		rootJSON, marshalErr := json.Marshal(sortedGraphIDs(roots))
		if marshalErr != nil {
			return marshalErr
		}
		where := "x.testee_id IN (SELECT ids.id FROM JSON_TABLE(?, '$[*]' COLUMNS(id BIGINT PATH '$')) AS ids)"
		args := []any{string(rootJSON)}
		if table == "statistics_assessment_fact" && len(users) > 0 {
			userJSON, marshalErr := json.Marshal(sortedGraphIDs(users))
			if marshalErr != nil {
				return marshalErr
			}
			where += " OR x.filler_id IN (SELECT ids.id FROM JSON_TABLE(?, '$[*]' COLUMNS(id BIGINT PATH '$')) AS ids)"
			args = append(args, string(userJSON))
		}
		if e = r.sqlPages(ctx, table, where, args, func(row record) error {
			testee, _ := graphSQLValue(row, "testee_id")
			matched := roots[testee]
			{
				kind, _ := graphSQLValue(row, "source_type")
				ref, _ := graphSQLValue(row, "source_ref")
				matched = matched || typedSources[kind][ref]
				if table == "statistics_assessment_fact" {
					filler, _ := graphSQLValue(row, "filler_id")
					matched = matched || users[filler]
				}
			}
			if matched {
				return remains(row)
			}
			return nil
		}); e != nil {
			return e
		}
	}
	// A fact's explicit parent columns remain meaningful even if its Testee or
	// source_ref belongs to another component. Such rows are exceptions, never
	// implicit deletion targets. Query those typed columns without transferring
	// unrelated fact rows or confusing equal IDs from different parent domains.
	for _, link := range []struct{ table, column, scope string }{
		{"statistics_assessment_fact", "answersheet_id", "mongo/answersheets"},
		{"statistics_assessment_fact", "assessment_id", "mysql/assessment"},
		{"statistics_assessment_fact", "outcome_id", "mysql/evaluation_outcome"},
		{"statistics_assessment_fact", "report_id", "mongo/interpret_report_artifacts"},
		{"statistics_assessment_fact", "enrollment_id", "mysql/plan_enrollment"},
		{"statistics_assessment_fact", "task_id", "mysql/assessment_task"},
		{"statistics_plan_fact", "enrollment_id", "mysql/plan_enrollment"},
		{"statistics_plan_fact", "task_id", "mysql/assessment_task"},
	} {
		if e = rootBatches(sortedGraphIDs(f.ids[link.scope]), 64000, func(batch []string) error {
			ids, err := json.Marshal(batch)
			if err != nil {
				return err
			}
			where := "x.`" + link.column + "` IN (SELECT ids.id FROM JSON_TABLE(?, '$[*]' COLUMNS(id BIGINT PATH '$')) AS ids)"
			return r.sqlPages(ctx, link.table, where, []any{string(ids)}, remains)
		}); e != nil {
			return e
		}
	}
	for _, source := range []struct{ kind, scope string }{{"answersheet_submitted", "mongo/answersheets"}, {"assessment_completed", "mysql/assessment"}} {
		if e = rootBatches(sortedGraphIDs(f.ids[source.scope]), 1000, func(batch []string) error {
			slots, args := placeholders(batch)
			args = append([]any{source.kind}, args...)
			return r.sqlPages(ctx, "statistics_store_activity_fact", "x.fact_type=? AND x.business_object_id IN ("+slots+")", args, remains)
		}); e != nil {
			return e
		}
	}
	if e = checkSQL("testee", "profile_id", graphRootSet(iam.Profiles), false); e != nil {
		return e
	}
	if e = checkSQL("operators", "user_id", users, false); e != nil {
		return e
	}
	if e = checkSQL("answering_start", "started_by_user_id", users, false); e != nil {
		return e
	}
	// This runs after exact target deletion. It reads only technical fields from
	// the remaining documents, including malformed numeric aliases and cross-domain
	// links. Answers and report bodies are never requested by the lean reader.
	for _, name := range []string{"answersheets", "interpret_report_artifacts", "report_query_catalog", "report_generations", "interpretation_runs"} {
		if e = r.mongoPages(ctx, name, bson.D{}, func(row record) error {
			raw := bson.Raw(row.BSON)
			candidate := graphRawIDMember(raw, "testee_id", roots) || name == "answersheets" && graphRawIDMember(raw, "filler_id", users) || graphRawIDMember(raw, "domain_id", f.ids["mongo/"+name])
			for _, link := range []struct{ key, scope string }{{"assessment_id", "mysql/assessment"}, {"outcome_id", "mysql/evaluation_outcome"}, {"generation_id", "mongo/report_generations"}, {"interpretation_run_id", "mongo/interpretation_runs"}, {"latest_run_id", "mongo/interpretation_runs"}, {"report_id", "mongo/interpret_report_artifacts"}, {"sort_report_id", "mongo/interpret_report_artifacts"}, {"task_id", "mysql/assessment_task"}} {
				candidate = candidate || graphRawIDMember(raw, link.key, f.ids[link.scope])
			}
			if name == "report_query_catalog" && graphMongoString(raw, "source_kind") == "artifact" {
				candidate = candidate || graphRawIDMember(raw, "source_id", f.ids["mongo/interpret_report_artifacts"])
			}
			if v := raw.Lookup("attribution"); v.Type == bson.TypeEmbeddedDocument {
				candidate = candidate || graphRawIDMember(v.Document(), "task_id", f.ids["mysql/assessment_task"]) || graphRawIDMember(v.Document(), "enrollment_id", f.ids["mysql/plan_enrollment"])
			}
			if v := raw.Lookup("start_context"); v.Type == bson.TypeEmbeddedDocument {
				candidate = candidate || graphRawIDMember(v.Document(), "id", f.ids["mysql/answering_start"])
			}
			if candidate {
				return remains(row)
			}
			return nil
		}); e != nil {
			return e
		}
	}
	// Message wire, dedup, attention and external delivery evidence are retained
	// online. Their settled state is checked by the rule plan; they are not deleted
	// merely because their historical subject is being retired.
	if e = checkSQL("ai_bridge_requests", "testee_id", roots, false); e != nil {
		return e
	}
	if e = rootBatches(sortedGraphIDs(f.ids["mysql/assessment"]), 1000, func(batch []string) error {
		slots, args := placeholders(batch)
		return r.sqlPages(ctx, "ai_bridge_requests", "EXISTS (SELECT 1 FROM ai_bridge_request_assessments ra WHERE ra.request_id=x.request_id AND ra.assessment_id IN ("+slots+"))", args, remains)
	}); e != nil {
		return e
	}
	return nil
}
func graphRawIDMember(raw bson.Raw, key string, ids map[string]bool) bool {
	v := raw.Lookup(key)
	switch v.Type {
	case bson.TypeInt64:
		return ids[strconv.FormatInt(v.Int64(), 10)]
	case bson.TypeInt32:
		return ids[strconv.FormatInt(int64(v.Int32()), 10)]
	case bson.TypeString:
		value := v.StringValue()
		if ids[value] {
			return true
		}
		n, e := strconv.ParseInt(value, 10, 64)
		return e == nil && n > 0 && ids[strconv.FormatInt(n, 10)]
	case bson.TypeDouble:
		value := v.Double()
		return value > 0 && value < math.Exp2(63) && math.Trunc(value) == value && ids[strconv.FormatInt(int64(value), 10)]
	}
	return false
}
