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
	sqlOwnerPages(context.Context, string, []string, func(record) error) error
	mongoOwnerPages(context.Context, string, []string, func(record) error) error
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
func (p *cohortPlanner) sqlObject(r record, rootIDs ...string) (graphObject, error) {
	o := graphObject{Object: r.Object, Links: map[string]string{}, References: []eventevidence.StandardReference{}, RootIDs: append([]string{}, rootIDs...)}
	for _, name := range []string{"id", "org_id", "testee_id", "profile_id", "assessment_id", "answer_sheet_id", "answersheet_id", "report_id", "outcome_id", "evaluation_outcome_id", "evaluation_run_id", "enrollment_id", "plan_id", "task_id", "event_id", "retry_event_id", "committed_event_id", "opening_event_id", "source_type", "fact_type", "source_ref", "business_object_id", "scope", "status", "state", "claim_token", "lease_expires_at", "next_attempt_at", "lease_until", "manual_replay_request_id", "request_id", "resource_id", "retry_disposition", "stat_date", "clinician_id", "entry_id", "conducting_store_id"} {
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
		terminal := o.Links["status"] == "succeeded" || o.Links["status"] == "failed"
		if !terminal && (o.Links["claim_token"] != "" || o.Links["lease_expires_at"] != "") {
			p.action("settle_runtime_claim")
		}
		if o.Links["retry_event_id"] != "" {
			p.action("settle_runtime_retry_event")
		}
	case "interpretation_attention_projection":
		p.writer.index.Retained[r.Object.Name] = "successful complete event identity remains online as the report attention replay guard"
		if o.Links["status"] != "succeeded" || o.Links["event_id"] == "" || !graphDecimal(o.Links["report_id"]) || !graphDecimal(o.Links["assessment_id"]) || !graphDecimal(o.Links["testee_id"]) {
			p.holdObject(o, r.Object.Name+":incomplete_or_unsettled_projection")
		}
	case "task_opened_reminder_delivery", "task_opened_reminder_batch":
		p.writer.index.Retained[r.Object.Name] = "frozen recipients and terminal external outcome evidence remain online; no resend authority"
		state := o.Links["state"]
		settled := r.Object.Name == "task_opened_reminder_batch" && (state == "frozen" || state == "suppressed") || r.Object.Name == "task_opened_reminder_delivery" && (state == "confirmed" || state == "rejected" || state == "reviewed" || state == "suppressed")
		if !settled || o.Links["claim_token"] != "" || o.Links["lease_until"] != "" {
			p.action("settle_" + r.Object.Name + ":" + state)
			p.holdObject(o, r.Object.Name+":unsettled_external_responsibility")
		}
	case "retry_event_hold", "event_delivery_dead_letter":
		p.writer.index.Retained[r.Object.Name] = "original delivery identity, wire, retry and audit evidence retained online; settled ownership never grants deletion"
		settled := r.Object.Name == "event_delivery_dead_letter" && (o.Links["retry_disposition"] == "archived_mock" || o.Links["retry_disposition"] == "terminal") || r.Object.Name == "retry_event_hold" && o.Links["status"] == "replayed" && (o.Links["retry_disposition"] == "" || o.Links["retry_disposition"] == "terminal")
		if !settled || o.Links["claim_token"] != "" || o.Links["lease_until"] != "" || o.Links["lease_expires_at"] != "" {
			p.action("settle_" + r.Object.Name)
			p.holdObject(o, r.Object.Name+":unsettled_replay_responsibility")
		}
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
func (p *cohortPlanner) mongoObject(r record, rootIDs ...string) (graphObject, error) {
	o := graphObject{Object: r.Object, Links: map[string]string{}, References: []eventevidence.StandardReference{}, RootIDs: append([]string{}, rootIDs...)}
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
		if x := raw.Lookup("durable_acceptance"); x.Type == bson.TypeEmbeddedDocument {
			o.Links["durable_event_id"] = graphMongoString(x.Document(), "event_id")
		}
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
func singleRootSQLScan(name string) bool {
	switch name {
	case "statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact", "interpretation_attention_projection":
		return true
	}
	return false
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
	// These four actual tables have no Testee-leading index. One PK keyset
	// traversal avoids restarting the same candidate scan for each 1000 roots.
	// Only exact frozen root membership reaches the existing ownership checks.
	var roots map[string]bool
	if singleRootSQLScan(name) {
		roots = make(map[string]bool, len(ids))
		for _, id := range ids {
			roots[id] = true
		}
		where, args = "x.org_id=?", []any{org}
		if name == "interpretation_attention_projection" {
			where = "EXISTS (SELECT 1 FROM testee t WHERE t.id=x.testee_id AND t.org_id=?)"
		}
	}
	read := func(emit func(record) error) error {
		return p.reader.sqlPages(p.context, name, where, args, func(r record) error {
			if roots != nil {
				id, _ := graphSQLValue(r, "testee_id")
				if !roots[id] {
					return nil
				}
			}
			return emit(r)
		})
	}
	switch name {
	case "evaluation_outcome", "assessment_score", "assessment_task", "runtime_checkpoint", "interpretation_attention_projection", "interpretation_admission_failure", "task_opened_reminder_batch", "task_opened_reminder_delivery", "qs_rm_evaluation_request_ref", "statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact":
		batch := []record{}
		size := 0
		flush := func() error {
			var e error
			if graphStatisticTable(name) {
				e = p.statisticBatch(p.context, batch)
			} else {
				e = p.sqlOwnedBatch(p.context, batch)
			}
			batch = nil
			size = 0
			return e
		}
		if e := read(func(r record) error {
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
		return read(p.emitSQL)
	}
}

func planCohort(ctx context.Context, reader graphReader, c cohortSelector, p protected, dir, selectorSHA, source string) (graphIndex, error) {
	return leanPlanCohort(ctx, reader, c, p, dir, selectorSHA, source)
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
