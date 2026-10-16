package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"
	"unicode/utf8"

	eventevidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
)

type graphReader interface {
	sqlPages(context.Context, string, string, []any, func(record) error) error
	mongoPages(context.Context, string, bson.D, func(record) error) error
}
type cohortPlanner struct {
	context         context.Context
	reader          graphReader
	selector        cohortSelector
	writer          *graphWriter
	dir             string
	held            []graphHold
	holdError       error
	referenceOwners map[string]graphObject
}

func (p *cohortPlanner) action(reason string) { p.writer.index.RequiredActions[reason]++ }
func (p *cohortPlanner) reference(e *eventevidence.EventEvidenceV1, o *graphObject) {
	if e == nil {
		return
	}
	if err := e.Validate(); err != nil {
		p.holdObject(*o, o.Object.Store+"/"+o.Object.Name+":invalid_event_evidence")
		return
	}
	if e.Class == eventevidence.StandardReferenceClass && e.Reference != nil {
		if e.Reference.Scope != "org:"+p.selector.Selection.OrgID {
			p.holdObject(*o, "event_reference_scope_conflict")
			return
		}
		o.References = append(o.References, *e.Reference)
	}
}
func (p *cohortPlanner) sqlObject(r record) (graphObject, error) {
	o := graphObject{Object: r.Object, Links: map[string]string{}, References: []eventevidence.StandardReference{}}
	o.Object.Fingerprint = fingerprint(r)
	for _, name := range []string{"id", "org_id", "testee_id", "profile_id", "assessment_id", "answer_sheet_id", "outcome_id", "evaluation_outcome_id", "enrollment_id", "plan_id", "task_id", "event_id", "retry_event_id", "committed_event_id", "opening_event_id", "source_type", "source_ref", "business_object_id", "scope", "status", "state", "claim_token", "lease_expires_at", "lease_until", "manual_replay_request_id", "request_id", "resource_id", "retry_disposition", "stat_date", "clinician_id", "entry_id", "conducting_store_id"} {
		if v, ok := graphSQLValue(r, name); ok && v != "" {
			if !utf8.ValidString(v) {
				return o, errors.New("technical SQL link is not valid UTF-8")
			}
			o.Links[name] = v
		}
	}
	if org := o.Links["org_id"]; org != "" && org != p.selector.Selection.OrgID {
		p.holdObject(o, r.Object.Name+":organization_conflict")
	}
	if r.Object.Name == "testee" {
		source, _ := graphSQLValue(r, "source")
		if source != p.selector.Selection.Source {
			return o, errors.New("explicit root no longer belongs to owner-confirmed source")
		}
	}
	switch r.Object.Name {
	case "assessment_task":
		if s := o.Links["status"]; s == "opened" || s == "pending" {
			p.action("settle_task_" + s)
		}
	case "runtime_checkpoint":
		if o.Links["scope"] != "evaluation_run" {
			p.holdObject(o, "runtime_checkpoint:unknown_scope")
		}
		if o.Links["claim_token"] != "" || o.Links["lease_expires_at"] != "" {
			p.action("settle_runtime_claim")
		}
		if o.Links["retry_event_id"] != "" {
			p.action("settle_runtime_retry_event")
		}
	case "task_opened_reminder_delivery", "task_opened_reminder_batch":
		p.action("settle_" + r.Object.Name + ":" + o.Links["state"])
	case "retry_event_hold", "event_delivery_dead_letter", "qs_rm_replay_requests", "qs_rm_replay_items":
		p.action("settle_" + r.Object.Name)
	case "statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact", "statistics_store_activity_fact":
		p.action("rebuild_shared_statistics_buckets")
	}
	if raw, ok := graphSQLValue(r, "committed_event_evidence"); ok && raw != "" && raw != "null" {
		var evidence eventevidence.EventEvidenceV1
		if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
			p.holdObject(o, r.Object.Name+":event_evidence_json_invalid")
		} else {
			p.reference(&evidence, &o)
		}
	}
	return o, nil
}
func (p *cohortPlanner) emitSQL(r record) error {
	o, e := p.sqlObject(r)
	if e != nil {
		return e
	}
	return p.writer.add(o, graphSQLTables[r.Object.Name])
}
func (p *cohortPlanner) mongoObject(r record) (graphObject, error) {
	o := graphObject{Object: r.Object, Links: map[string]string{}, References: []eventevidence.StandardReference{}}
	o.Object.Fingerprint = fingerprint(r)
	raw := bson.Raw(r.BSON)
	for _, name := range []string{"domain_id", "org_id", "testee_id", "filler_id", "assessment_id", "outcome_id", "generation_id", "interpretation_run_id", "latest_run_id", "report_id", "source_id", "sort_report_id"} {
		v, e := graphMongoID(raw, name, true)
		if e != nil {
			return o, fmt.Errorf("%s/%s: %w", r.Object.Name, name, e)
		}
		if v != "" {
			o.Links[name] = v
		}
	}
	for _, name := range []string{"status", "state", "source_kind", "generated_event_id", "retry_event_id", "retry_disposition", "task_id"} {
		if v := graphMongoString(raw, name); v != "" {
			o.Links[name] = v
		}
	}
	if org := o.Links["org_id"]; org != "" && org != p.selector.Selection.OrgID {
		p.holdObject(o, r.Object.Name+":organization_conflict")
	}
	if r.Object.Name == "answersheets" {
		if o.Links["domain_id"] == "" || o.Links["testee_id"] == "" || o.Links["org_id"] == "" {
			return o, errors.New("answersheet missing actual typed ownership")
		}
		if x := raw.Lookup("admission"); x.Type == bson.TypeEmbeddedDocument {
			o.Links["purpose"] = graphMongoString(x.Document(), "purpose")
		}
		if x := raw.Lookup("start_context"); x.Type == bson.TypeEmbeddedDocument {
			id, e := graphMongoID(x.Document(), "id", true)
			if e != nil {
				return o, e
			}
			o.Links["answering_start_id"] = id
		}
		if x := raw.Lookup("attribution"); x.Type == bson.TypeEmbeddedDocument {
			for _, key := range []string{"task_id", "enrollment_id", "plan_id"} {
				v := graphMongoString(x.Document(), key)
				if v != "" {
					if !graphDecimal(v) {
						return o, errors.New("attribution identity is not a canonical decimal string")
					}
					o.Links["attribution_"+key] = v
				}
			}
		}
		if o.Links["purpose"] != "assessment" && o.Links["purpose"] != "independent_questionnaire" {
			p.holdObject(o, "answersheets:unknown_admission_purpose")
		}
	}
	switch r.Object.Name {
	case "report_generations":
		if e := requireGraphLinks(o, "domain_id", "outcome_id"); e != nil {
			return o, e
		}
	case "interpretation_runs":
		if e := requireGraphLinks(o, "domain_id", "generation_id"); e != nil {
			return o, e
		}
	case "interpret_report_artifacts":
		if e := requireGraphLinks(o, "domain_id", "org_id", "testee_id", "assessment_id", "outcome_id", "generation_id", "interpretation_run_id"); e != nil {
			return o, e
		}
	case "report_query_catalog":
		if e := requireGraphLinks(o, "org_id", "testee_id", "assessment_id", "outcome_id", "generation_id", "source_id", "sort_report_id"); e != nil {
			return o, e
		}
	}
	if r.Object.Name == "report_query_catalog" && (o.Links["source_kind"] != "artifact" || o.Links["source_id"] == "" || o.Links["source_id"] != o.Links["sort_report_id"]) {
		p.holdObject(o, "report_query_catalog:unknown_or_inconsistent_artifact_source")
	}
	if r.Object.Name == "interpretation_runs" {
		if o.Links["status"] == "running" || o.Links["status"] == "pending" || o.Links["retry_disposition"] == "automatic" {
			p.action("settle_interpretation_run")
		}
	}
	for _, path := range [][]string{{"generated_event_evidence"}, {"retry_event_evidence"}, {"durable_acceptance", "event_evidence"}} {
		v := raw.Lookup(path...)
		if v.Type == bson.TypeEmbeddedDocument {
			var evidence eventevidence.EventEvidenceV1
			if err := bson.Unmarshal(v.Document(), &evidence); err != nil {
				p.holdObject(o, r.Object.Name+":event_evidence_bson_invalid")
			} else {
				p.reference(&evidence, &o)
			}
		}
	}
	return o, nil
}
func rootBatches(ids []string, limit int, fn func([]string) error) error {
	for n := 0; n < len(ids); n += limit {
		end := n + limit
		if end > len(ids) {
			end = len(ids)
		}
		if e := fn(ids[n:end]); e != nil {
			return e
		}
	}
	return nil
}
func (p *cohortPlanner) rootSQL(name string, ids []string) error {
	slots, args := placeholders(ids)
	org := p.selector.Selection.OrgID
	where := "x.testee_id IN (" + slots + ") AND x.org_id=?"
	args = append(args, org)
	switch name {
	case "testee":
		where = "x.id IN (" + slots + ") AND x.org_id=? AND BINARY x.source=BINARY ? AND x.created_at>=? AND x.created_at<=?"
		from, _ := time.Parse(time.RFC3339, p.selector.Selection.From)
		through, _ := time.Parse(time.RFC3339, p.selector.Selection.Through)
		args = append(args, p.selector.Selection.Source, from.Format("2006-01-02 15:04:05.999999"), through.Format("2006-01-02 15:04:05.999999"))
	case "interpretation_attention_projection":
		where = "x.testee_id IN (" + slots + ") AND EXISTS (SELECT 1 FROM testee t WHERE t.id=x.testee_id AND t.org_id=?)"
	case "assessment_score", "runtime_checkpoint":
		where = "EXISTS (SELECT 1 FROM assessment a WHERE a.id=x.assessment_id AND a.testee_id IN (" + slots + ") AND a.org_id=?)"
	case "task_opened_reminder_batch", "task_opened_reminder_delivery":
		where = "x.org_id=? AND EXISTS (SELECT 1 FROM assessment_task t WHERE BINARY x.task_id=BINARY CAST(t.id AS CHAR) AND t.testee_id IN (" + slots + ") AND t.org_id=?)"
		args = append([]any{org}, args...)
	case "qs_rm_evaluation_request_ref":
		where = "x.org_id=? AND EXISTS (SELECT 1 FROM assessment a WHERE a.id=x.assessment_id AND a.testee_id IN (" + slots + ") AND a.org_id=?)"
		args = append([]any{org}, args...)
	}
	switch name {
	case "evaluation_outcome", "assessment_score", "assessment_task", "runtime_checkpoint", "interpretation_attention_projection", "interpretation_admission_failure":
		batch := []record{}
		size := 0
		flush := func() error { e := p.sqlOwnedBatch(p.context, batch); batch = nil; size = 0; return e }
		if e := p.reader.sqlPages(p.context, name, where, args, func(r record) error {
			n := len(encode(r))
			if len(batch) > 0 && size+n > 64*1024*1024 {
				if e := flush(); e != nil {
					return e
				}
			}
			batch = append(batch, r)
			size += n
			if len(batch) == 1000 {
				return flush()
			}
			return nil
		}); e != nil {
			return e
		}
		return flush()
	default:
		return p.reader.sqlPages(p.context, name, where, args, p.emitSQL)
	}
}

func planCohort(ctx context.Context, reader graphReader, c cohortSelector, p protected, dir, selectorSHA, source string) (graphIndex, error) {
	index := graphIndex{Format: 1, Kind: "mock_cohort_graph_index", Operation: c.Operation, SelectorSHA: selectorSHA, ProtectionSHA: c.ProtectionSHA, OwnerEvidenceSHA: c.OwnerEvidenceSHA, Source: source, Binding: c.Binding, Shards: []graphShard{}, Counts: map[string]int64{}, Retained: map[string]string{}, Holds: map[string]int64{}, HoldShards: []graphHoldShard{}, RequiredActions: map[string]int64{}, ReadOnly: true, WritePending: true}
	if e := validateSelector(c, p); e != nil {
		return index, e
	}
	if !hashPattern.MatchString(selectorSHA) {
		return index, errors.New("exact selector hash required")
	}
	if e := privateDirectory(dir); e != nil {
		return index, e
	}
	protectedKeys := map[string]bool{}
	for _, o := range append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...) {
		protectedKeys[objectID(o)] = true
	}
	writer := &graphWriter{dir: dir, index: &index, protected: protectedKeys}
	planner := &cohortPlanner{context: ctx, reader: reader, selector: c, writer: writer, dir: dir, referenceOwners: map[string]graphObject{}}
	// Exact roots are checked before any dependent discovery. Filtering a drifted
	// root out is an error, never an apparently successful smaller cohort.
	ids := sortedRootIDs(c.TesteeIDs)
	if e := rootBatches(ids, 1000, func(batch []string) error { return planner.rootSQL("testee", batch) }); e != nil {
		return index, e
	}
	if index.Counts["mysql/testee"] != int64(len(ids)) {
		return index, errors.New("one or more frozen roots failed source/org/window verification")
	}
	if e := writer.flush(); e != nil {
		return index, e
	}
	// Child discovery is rooted in this exact Testee subset. Shared Plan and Entry
	// definitions and their unassignable resolve logs are deliberately retained.
	tables := []string{"clinician_relation", "testee_store_history", "assessment_entry_intake_log", "answering_start", "plan_enrollment", "assessment_task", "assessment", "evaluation_outcome", "assessment_score", "runtime_checkpoint", "interpretation_attention_projection", "interpretation_admission_failure", "task_opened_reminder_batch", "task_opened_reminder_delivery", "qs_rm_evaluation_request_ref", "statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact"}
	for _, name := range tables {
		index.Counts["mysql/"+name] = 0
		if e := rootBatches(ids, 1000, func(batch []string) error { return planner.rootSQL(name, batch) }); e != nil {
			return index, e
		}
		if e := writer.flush(); e != nil {
			return index, e
		}
	}
	index.Counts["mongo/answersheets"] = 0
	index.Counts["mongo/interpret_report_artifacts"] = 0
	index.Counts["mongo/report_query_catalog"] = 0
	for _, name := range []string{"answersheets", "interpret_report_artifacts", "report_query_catalog"} {
		if e := planner.rootMongo(ctx, name, ids); e != nil {
			return index, e
		}
		if e := writer.flush(); e != nil {
			return index, e
		}
	}

	index.Counts["mongo/report_generations"] = 0
	if e := graphIDs(dir, &index, "mysql", "evaluation_outcome", "id", 500, func(batch []string) error {
		return planner.generationPage(ctx, batch)
	}); e != nil {
		return index, e
	}
	if e := writer.flush(); e != nil {
		return index, e
	}
	index.Counts["mongo/interpretation_runs"] = 0
	if e := graphIDs(dir, &index, "mongo", "report_generations", "domain_id", 500, func(batch []string) error {
		return planner.runPage(ctx, batch)
	}); e != nil {
		return index, e
	}
	if e := writer.flush(); e != nil {
		return index, e
	}
	// Fixed shared objects are not converted into mock ownership by association.
	for _, name := range []string{"assessment_plan", "assessment_entry", "assessment_entry_resolve_log", "actor_stores", "operators", "clinician", "questionnaires", "assessment_models", "assessment_norms", "interpretation_report_templates"} {
		index.Retained[name] = "shared/formal definition or non-exclusive ownership; no deletion authority"
	}
	for _, id := range c.RetainPlanIDs {
		index.Retained["assessment_plan/"+id] = "explicit shared plan; only exact cohort enrollment/task children are candidates"
	}
	for _, name := range []string{"statistics_org_snapshot", "statistics_access_daily", "statistics_assessment_daily", "statistics_plan_activity_daily", "statistics_plan_fulfillment_daily", "statistics_store_activity_daily"} {
		index.Retained[name] = "shared aggregate buckets; exact affected buckets must be rebuilt from retained facts"
	}
	// These traversals are completed separately below rather than silently
	// presenting the business rows alone as the complete reliable-message graph.
	if e := planner.discoverResponsibilities(ctx); e != nil {
		return index, e
	}
	if e := writer.flush(); e != nil {
		return index, e
	}
	if e := planner.discoverStoreFacts(ctx); e != nil {
		return index, e
	}
	if e := writer.flush(); e != nil {
		return index, e
	}
	index.RequiredActions["targeted_cache_invalidation_before_reopen"] = 1
	index.RequiredActions["generator_and_prepared_input_retirement"] = 1
	if e := planner.flushHolds(); e != nil {
		return index, e
	}
	index.DiscoveryComplete = true
	return index, nil
}

func (p *cohortPlanner) discoverStoreFacts(ctx context.Context) error {
	p.writer.index.Counts["mysql/statistics_store_activity_fact"] = 0
	for _, source := range []struct{ store, name, link, fact string }{{"mongo", "answersheets", "domain_id", "answersheet_submitted"}, {"mysql", "assessment", "id", "assessment_completed"}} {
		if e := graphIDs(p.dir, p.writer.index, source.store, source.name, source.link, 1000, func(ids []string) error {
			slots, args := placeholders(ids)
			args = append([]any{p.selector.Selection.OrgID, source.fact}, args...)
			return p.reader.sqlPages(ctx, "statistics_store_activity_fact", "x.org_id=? AND BINARY x.source_type=BINARY ? AND x.business_object_id IN ("+slots+")", args, p.emitSQL)
		}); e != nil {
			return e
		}
		if e := p.writer.flush(); e != nil {
			return e
		}
	}
	return nil
}

func readOnlyGraphResult(outPath string, index graphIndex) error {
	if !index.Released {
		return errors.New("unreleased planner cannot publish complete index")
	}
	if filepath.Base(outPath) != "graph-index.json" {
		return errors.New("fixed graph-index filename required")
	}
	return writeNew(outPath, encode(index))
}
