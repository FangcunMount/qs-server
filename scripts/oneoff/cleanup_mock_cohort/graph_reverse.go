package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"

	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
)

type graphFrozenReferences struct {
	ids        map[string]map[string]bool
	references map[string]eventevidence.StandardReference
	physical   map[string]string
	retained   map[string]string
}

func frozenGraphReferences(dir string, index graphIndex, roots []string, physical bool) (graphFrozenReferences, error) {
	f := graphFrozenReferences{ids: map[string]map[string]bool{}, references: map[string]eventevidence.StandardReference{}, physical: map[string]string{}, retained: map[string]string{}}
	e := forSelectedScope(dir, index, roots, func(page []graphObject, _ int) error {
		for _, o := range page {
			if physical {
				f.physical[objectID(o.Object)] = o.Object.Fingerprint
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
			if graphDecimal(id) {
				if f.ids[scope] == nil {
					f.ids[scope] = map[string]bool{}
				}
				f.ids[scope][id] = true
			}
			for _, ref := range o.References {
				f.references[referenceKey(ref.Producer, ref.EventID, ref.Destination)] = ref
			}
		}
		return nil
	})
	if e != nil {
		return f, e
	}
	e = forRetainedScope(dir, index, roots, func(page []graphObject, _ int) error {
		for _, o := range page {
			f.retained[objectID(o.Object)] = o.Object.Fingerprint
		}
		return nil
	})
	return f, e
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
	return graphRelationshipCheck(ctx, r, g, index, scopeDir, false)
}
func graphRelationshipCheck(ctx context.Context, r graphReader, g group, index graphIndex, scopeDir string, allowOriginal bool) error {
	f, e := frozenGraphReferences(scopeDir, index, g.QS.Graph.RootIDs, allowOriginal)
	if e != nil {
		return e
	}
	roots := graphRootSet(g.QS.Graph.RootIDs)
	var iam iamScope
	if e = json.Unmarshal(g.IAM, &iam); e != nil {
		return e
	}
	users := graphRootSet(iam.Users)
	remains := func(row record) error {
		if f.retained[objectID(row.Object)] == fingerprint(row) || allowOriginal && f.physical[objectID(row.Object)] == fingerprint(row) {
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
	for _, table := range []string{"assessment_score", "runtime_checkpoint", "evaluation_outcome", "qs_rm_evaluation_request_ref"} {
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
	for _, table := range []string{"task_opened_reminder_batch", "task_opened_reminder_delivery"} {
		if e = checkSQL(table, "task_id", f.ids["mysql/assessment_task"], true); e != nil {
			return e
		}
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
	// Final validation uses the same single PK scan for these unindexed tables.
	// Testee, IAM filler and source domains are tested together, not by repeating
	// a full candidate scan for every frozen parent page or source type.
	for _, table := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact", "interpretation_attention_projection"} {
		if e = r.sqlPages(ctx, table, "1=1", nil, func(row record) error {
			testee, _ := graphSQLValue(row, "testee_id")
			matched := roots[testee]
			if table == "interpretation_attention_projection" {
				assessment, _ := graphSQLValue(row, "assessment_id")
				report, _ := graphSQLValue(row, "report_id")
				matched = matched || f.ids["mysql/assessment"][assessment] || f.ids["mongo/interpret_report_artifacts"][report]
			} else {
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
	for _, source := range []struct{ kind, scope string }{{"answersheet_submitted", "mongo/answersheets"}, {"assessment_completed", "mysql/assessment"}} {
		if e = rootBatches(sortedGraphIDs(f.ids[source.scope]), 1000, func(batch []string) error {
			slots, args := placeholders(batch)
			args = append([]any{source.kind}, args...)
			return r.sqlPages(ctx, "statistics_store_activity_fact", "BINARY x.source_type=BINARY ? AND x.business_object_id IN ("+slots+")", args, remains)
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
	if e = checkSQL("task_opened_reminder_delivery", "user_id", users, true); e != nil {
		return e
	}
	if e = checkSQL("task_opened_reminder_delivery", "login_identity_id", graphRootSet(iam.Identities), true); e != nil {
		return e
	}
	if e = checkSQL("answering_start", "started_by_user_id", users, false); e != nil {
		return e
	}
	// These three namespaces have no usable Testee index in production. Exactly
	// one bounded cursor scan per collection avoids an IN-scan for every root page.
	for _, name := range []string{"answersheets", "interpret_report_artifacts", "report_query_catalog"} {
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
	for _, check := range []struct{ name, key, scope string }{{"report_generations", "outcome_id", "mysql/evaluation_outcome"}, {"interpretation_runs", "generation_id", "mongo/report_generations"}} {
		if e = rootBatches(sortedGraphIDs(f.ids[check.scope]), 500, func(batch []string) error {
			values := bson.A{}
			for _, id := range batch {
				n, _ := strconv.ParseInt(id, 10, 64)
				values = append(values, n)
			}
			return r.mongoPages(ctx, check.name, bson.D{{Key: check.key, Value: bson.D{{Key: "$in", Value: values}}}}, remains)
		}); e != nil {
			return e
		}
	}
	for _, name := range []string{"event_delivery_dead_letter", "retry_event_hold"} {
		if e = r.sqlPages(ctx, name, "x.org_id=?", []any{g.QS.Selector.OrgID}, func(row record) error {
			c, err := deliveryPayload(row)
			if err != nil {
				return err
			}
			if roots[c.testeeID] || f.ids["mysql/assessment"][c.assessmentID] {
				return remains(row)
			}
			return nil
		}); e != nil {
			return e
		}
	}
	for _, name := range []string{"qs_rm_replay_requests", "qs_rm_replay_items"} {
		if e = r.sqlPages(ctx, name, "x.org_id=?", []any{g.QS.Selector.OrgID}, remains); e != nil {
			return e
		}
	}
	if e = checkSQL("ai_bridge_requests", "testee_id", roots, false); e != nil {
		return e
	}
	if e = rootBatches(sortedGraphIDs(f.ids["mysql/assessment"]), 1000, func(batch []string) error {
		slots, args := placeholders(batch)
		return r.sqlPages(ctx, "ai_bridge_requests", "EXISTS (SELECT 1 FROM ai_bridge_request_assessments ra WHERE ra.request_id=x.request_id AND ra.assessment_id IN ("+slots+"))", args, remains)
	}); e != nil {
		return e
	}
	refs := []eventevidence.StandardReference{}
	for _, ref := range f.references {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return referenceKey(refs[i].Producer, refs[i].EventID, refs[i].Destination) < referenceKey(refs[j].Producer, refs[j].EventID, refs[j].Destination)
	})
	for start := 0; start < len(refs); start += 500 {
		end := start + 500
		if end > len(refs) {
			end = len(refs)
		}
		parts := []string{}
		args := []any{}
		filters := bson.A{}
		for _, ref := range refs[start:end] {
			parts = append(parts, "(BINARY x.producer=BINARY ? AND BINARY x.message_id=BINARY ? AND BINARY x.destination=BINARY ?)")
			args = append(args, ref.Producer, ref.EventID, ref.Destination)
			filters = append(filters, bson.D{{Key: "producer", Value: ref.Producer}, {Key: "message_id", Value: ref.EventID}, {Key: "destination", Value: ref.Destination}})
		}
		if e = r.sqlPages(ctx, "rm_outbox", strings.Join(parts, " OR "), args, remains); e != nil {
			return e
		}
		if e = r.mongoPages(ctx, "rm_outbox", bson.D{{Key: "$or", Value: filters}}, remains); e != nil {
			return e
		}
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
