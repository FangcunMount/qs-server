package main

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// The plan freezes identities and relationships. Only capture reads original
// values, after the maintenance boundary has been established.
type leanOwner struct {
	Root, Physical string
}

type leanPlanner struct {
	*cohortPlanner
	groups        map[string]cohortIdentityGroup
	owners        map[string]map[string]leanOwner
	parents       map[string]map[string]string
	seenMongo     map[string]string
	statisticSeen map[string]map[uint64]string
}

func (p *leanPlanner) groupRoots(roots ...string) []string {
	set := map[string]bool{}
	for _, root := range roots {
		for _, id := range p.groups[root].TesteeIDs {
			set[id] = true
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return sortedRootIDs(out)
}
func (p *leanPlanner) hold(o graphObject, reason string, roots ...string) {
	roots = append(roots, graphRoots(o)...)
	o.RootIDs = p.groupRoots(roots...)
	p.holdObject(o, reason)
}
func (p *leanPlanner) owner(name, id string) string { return p.owners[name][id].Root }
func (p *leanPlanner) remember(o graphObject, root, physicalKey string) {
	name := o.Object.Name
	id := o.Links["id"]
	if o.Object.Store == "mongo" {
		id = o.Links["domain_id"]
	}
	if name == "runtime_checkpoint" {
		id = o.Links["resource_id"]
	}
	if id == "" {
		return
	}
	if p.owners[name] == nil {
		p.owners[name] = map[string]leanOwner{}
	}
	if old, ok := p.owners[name][id]; ok && old.Physical != physicalKey {
		p.hold(o, name+":duplicate_typed_domain_identity", root, old.Root)
	}
	p.owners[name][id] = leanOwner{Root: root, Physical: physicalKey}
}
func (p *leanPlanner) parent(o graphObject, key, name, root string, required bool) {
	id := o.Links[key]
	if id == "" || id == "0" {
		if required {
			p.hold(o, o.Object.Name+":missing_"+key, root)
		}
		return
	}
	owner := p.owner(name, id)
	if owner == "" || owner != root {
		p.hold(o, o.Object.Name+":typed_"+key+"_ownership_conflict", root, owner)
	}
}
func (p *leanPlanner) emit(o graphObject, root string, rank int, physicalKey string) error {
	if root == "" || len(p.groups[root].TesteeIDs) == 0 {
		return errors.New("typed candidate has no frozen identity component")
	}
	o.RootIDs = p.groupRoots(root)
	if seen := p.statisticSeen[o.Object.Name]; seen != nil {
		id, e := statisticPhysicalID(o.Object)
		if e != nil {
			return e
		}
		if old, ok := seen[id]; ok {
			if old != root {
				p.hold(o, o.Object.Name+":conflicting_source_components", old, root)
			}
			return nil
		}
		seen[id] = root
	}
	if p.writer.protected[physicalKey] {
		p.hold(o, "target_intersects_protected_physical_identity", root)
		return nil
	}
	return p.writer.addPhysical(o, rank, physicalKey)
}
func (p *leanPlanner) processSQL(name string, r record, resolve func(graphObject) string, check func(graphObject, string)) error {
	links := map[string]string{}
	for _, key := range []string{"testee_id", "assessment_id", "outcome_id", "evaluation_outcome_id", "enrollment_id", "task_id", "source_ref", "business_object_id"} {
		if v, ok := graphSQLValue(r, key); ok {
			links[key] = v
		}
	}
	root := links["testee_id"]
	if resolve != nil {
		root = resolve(graphObject{Links: links})
	}
	o, e := p.sqlObject(r, p.groupRoots(root)...)
	if e != nil {
		return e
	}
	if len(p.groups[root].TesteeIDs) == 0 {
		return errors.New("indexed SQL relation returned a foreign frozen root")
	}
	if org := o.Links["org_id"]; org != "" && org != p.selector.Selection.OrgID {
		p.hold(o, name+":organization_conflict", root)
	}
	if check != nil {
		check(o, root)
	}
	if name == "runtime_checkpoint" {
		state, disposition := o.Links["status"], o.Links["retry_disposition"]
		// Terminal producers retain their claim token for the final CAS. Only
		// nonterminal state or a remaining retry responsibility holds the component.
		if state != "succeeded" && state != "failed" || disposition != "" && disposition != "terminal" || o.Links["next_attempt_at"] != "" {
			p.hold(o, "runtime_checkpoint:active_or_unsettled_execution", root)
		}
	}

	physicalKey := objectID(o.Object)
	switch name {
	case "testee", "clinician_relation", "assessment_entry_intake_log", "plan_enrollment", "assessment_task", "assessment", "evaluation_outcome", "runtime_checkpoint":
		p.remember(o, root, physicalKey)
	}
	p.eventOwner(o.Links["committed_event_id"], root)
	p.eventOwner(o.Links["retry_event_id"], root)
	for _, ref := range o.References {
		p.eventOwner(ref.EventID, root)
	}
	return p.emit(o, root, graphSQLTables[name], physicalKey)
}

func (p *leanPlanner) indexedSQL(name string, ids []string, query func([]string) (string, []any), resolve func(graphObject) string, check func(graphObject, string)) error {
	jobs := []leanReadJob{}
	if e := rootBatches(ids, 1000, func(batch []string) error {
		where, args := query(batch)
		jobs = append(jobs, leanReadJob{store: "mysql", name: name, where: where, args: args, ids: batch})
		return nil
	}); e != nil {
		return e
	}
	return readLeanJobs(p.context, p.reader, jobs, func(r record, _ leanReadJob) error { return p.processSQL(name, r, resolve, check) })
}
func (p *leanPlanner) byIDs(name, column string, ids []string, withOrg bool, resolve func(graphObject) string, check func(graphObject, string)) error {
	return p.indexedSQL(name, ids, func(batch []string) (string, []any) {
		slots, args := placeholders(batch)
		where := "x.`" + column + "` IN (" + slots + ")"
		if withOrg {
			where = "x.org_id=? AND " + where
			args = append([]any{p.selector.Selection.OrgID}, args...)
		}
		return where, args
	}, resolve, check)
}

func (p *leanPlanner) ids(name string) []string {
	out := make([]string, 0, len(p.owners[name]))
	for id := range p.owners[name] {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
func (p *leanPlanner) sqlChildren() error {
	roots := sortedRootIDs(p.selector.TesteeIDs)
	for _, name := range []string{"clinician_relation", "testee_store_history", "assessment_entry_intake_log", "answering_start", "plan_enrollment", "assessment_task", "assessment"} {
		if e := p.byIDs(name, "testee_id", roots, true, nil, func(o graphObject, root string) {
			if name == "assessment_task" {
				p.parent(o, "enrollment_id", "plan_enrollment", root, true)
				if p.parents["enrollment_plan"][o.Links["enrollment_id"]] != o.Links["plan_id"] {
					p.hold(o, "assessment_task:plan_enrollment_conflict", root)
				}
			}
			if name == "plan_enrollment" {
				p.parents["enrollment_plan"][o.Links["id"]] = o.Links["plan_id"]
			}
			if name == "assessment" && graphDecimal(o.Links["answer_sheet_id"]) {
				id := o.Links["answer_sheet_id"]
				if old := p.parents["sheet_root"][id]; old != "" && old != root {
					p.hold(o, "assessment:shared_answersheet", root, old)
				}
				p.parents["sheet_root"][id] = root
			}
		}); e != nil {
			return e
		}
		if e := p.writer.flush(); e != nil {
			return e
		}
	}
	assessments := p.ids("assessment")
	if e := p.byIDs("evaluation_outcome", "assessment_id", assessments, true, func(o graphObject) string { return p.owner("assessment", o.Links["assessment_id"]) }, func(o graphObject, root string) {
		p.parent(o, "assessment_id", "assessment", root, true)
		if o.Links["testee_id"] != root {
			p.hold(o, "evaluation_outcome:testee_conflict", root)
		}
	}); e != nil {
		return e
	}
	if e := p.writer.flush(); e != nil {
		return e
	}
	for _, name := range []string{"assessment_score", "runtime_checkpoint"} {
		if e := p.byIDs(name, "assessment_id", assessments, false, func(o graphObject) string { return p.owner("assessment", o.Links["assessment_id"]) }, func(o graphObject, root string) {
			p.parent(o, "assessment_id", "assessment", root, true)
			p.parent(o, "evaluation_outcome_id", "evaluation_outcome", root, false)
			if id := o.Links["testee_id"]; id != "" && id != root {
				p.hold(o, name+":testee_conflict", root)
			}
		}); e != nil {
			return e
		}
		if e := p.writer.flush(); e != nil {
			return e
		}
		if name == "runtime_checkpoint" {
			// Its duplicate-resource guard is complete, including every page.
			delete(p.owners, name)
		}
	}
	return p.byIDs("interpretation_admission_failure", "outcome_id", p.ids("evaluation_outcome"), true, func(o graphObject) string { return p.owner("evaluation_outcome", o.Links["outcome_id"]) }, func(o graphObject, root string) { p.parent(o, "assessment_id", "assessment", root, true) })
}
func (p *leanPlanner) mongoCandidate(name string, r record, root string) error {
	key := objectID(r.Object)
	if old, ok := p.seenMongo[key]; ok {
		if old != root {
			p.holdRecord(r, p.groupRoots(old, root), name+":conflicting_parent_components")
		}
		return nil
	}
	p.seenMongo[key] = root
	raw := bson.Raw(r.BSON)
	if _, e := graphMongoID(raw, "testee_id", true); e != nil {
		p.holdRecord(r, p.groupRoots(root), name+":unassignable_testee_identity")
		return nil
	}
	if testee, e := graphMongoID(raw, "testee_id", true); e == nil && testee != "" && testee != root {
		p.holdRecord(r, p.groupRoots(root, testee), name+":retained_object_links_target_parent")
		return nil
	}

	o, e := p.mongoObject(r, p.groupRoots(root)...)
	if e != nil {
		p.holdRecord(r, p.groupRoots(root), name+":invalid_typed_relationship")
		return nil
	}
	if len(p.groups[root].TesteeIDs) == 0 {
		return errors.New("mongo candidate has no frozen component")
	}
	if name == "interpretation_runs" {
		state, disposition := o.Links["status"], o.Links["retry_disposition"]
		lease, next := raw.Lookup("lease_expires_at"), raw.Lookup("next_attempt_at")
		if state != "succeeded" && state != "failed" || disposition != "" && disposition != "terminal" || lease.Type != 0 && lease.Type != bson.TypeNull || next.Type != 0 && next.Type != bson.TypeNull {
			p.hold(o, "interpretation_runs:active_or_unsettled_execution", root)
		}
	}
	if name == "report_generations" && o.Links["status"] != "generated" && o.Links["status"] != "failed" {
		p.hold(o, "report_generations:in_flight_generation", root)
	}

	if testee := o.Links["testee_id"]; testee != "" && testee != root {
		p.hold(o, name+":testee_ownership_conflict", root, testee)
	}
	switch name {
	case "answersheets":
		if expected := p.parents["sheet_root"][o.Links["domain_id"]]; expected != "" && expected != root {
			p.hold(o, "answersheets:assessment_ownership_conflict", root, expected)
		}
		for _, key := range []string{"task_id", "attribution_task_id"} {
			if id := o.Links[key]; id != "" {
				owner := p.owner("assessment_task", id)
				if owner != root {
					p.hold(o, "answersheets:task_ownership_conflict", root, owner)
				}
			}
		}
		p.parent(o, "attribution_enrollment_id", "plan_enrollment", root, false)
	case "report_generations":
		p.parent(o, "outcome_id", "evaluation_outcome", root, true)
		p.parents["generation_outcome"][o.Links["domain_id"]] = o.Links["outcome_id"]
	case "interpretation_runs":
		p.parent(o, "generation_id", "report_generations", root, true)
		p.parents["run_generation"][o.Links["domain_id"]] = o.Links["generation_id"]
	case "interpret_report_artifacts", "report_query_catalog":
		p.parent(o, "assessment_id", "assessment", root, true)
		p.parent(o, "outcome_id", "evaluation_outcome", root, true)
		p.parent(o, "generation_id", "report_generations", root, true)
		if p.parents["generation_outcome"][o.Links["generation_id"]] != o.Links["outcome_id"] {
			p.hold(o, name+":generation_outcome_conflict", root)
		}
		if name == "interpret_report_artifacts" {
			p.parent(o, "interpretation_run_id", "interpretation_runs", root, true)
			if p.parents["run_generation"][o.Links["interpretation_run_id"]] != o.Links["generation_id"] {
				p.hold(o, name+":run_generation_conflict", root)
			}
		} else {
			p.parent(o, "source_id", "interpret_report_artifacts", root, true)
		}
	}
	p.eventOwner(o.Links["generated_event_id"], root)
	p.eventOwner(o.Links["retry_event_id"], root)
	p.eventOwner(o.Links["durable_event_id"], root)
	for _, ref := range o.References {
		p.eventOwner(ref.EventID, root)
	}
	if name != "report_query_catalog" {
		p.remember(o, root, key)
	}
	return p.emit(o, root, graphMongoTables[name], key)
}
func (p *leanPlanner) mongoByIDs(name, field, parent string, ids []string) error {
	jobs := []leanReadJob{}
	if e := rootBatches(ids, 500, func(batch []string) error {
		jobs = append(jobs, leanReadJob{store: "mongo", name: name, filter: bson.D{{Key: field, Value: bson.D{{Key: "$in", Value: graphMongoIDs(batch)}}}}, ids: batch})
		return nil
	}); e != nil {
		return e
	}
	return readLeanJobs(p.context, p.reader, jobs, func(r record, job leanReadJob) error {
		raw := bson.Raw(r.BSON)
		id, e := graphMongoID(raw, field, false)
		if e != nil {
			roots := []string{}
			for _, candidate := range job.ids {
				if graphRawIDMember(raw, field, map[string]bool{candidate: true}) {
					roots = append(roots, p.owner(parent, candidate))
				}
			}
			if len(roots) == 0 {
				for _, candidate := range job.ids {
					roots = append(roots, p.owner(parent, candidate))
				}
			}
			p.holdRecord(r, p.groupRoots(roots...), name+":invalid_indexed_parent_identity")
			return nil
		}
		return p.mongoCandidate(name, r, p.owner(parent, id))
	})
}

func (p *leanPlanner) mongoChildren() error {
	// No Testee-leading AnswerSheet index exists at 101/39. Exactly one cursor
	// projects only technical IDs, including the independent-questionnaire roots.
	if e := p.reader.mongoPages(p.context, "answersheets", bson.D{}, func(r record) error {
		raw := bson.Raw(r.BSON)
		id, e := graphMongoID(raw, "testee_id", false)
		if e != nil {
			roots := []string{}
			for root := range p.groups {
				if graphRawIDMember(raw, "testee_id", map[string]bool{root: true}) {
					roots = append(roots, root)
				}
			}
			if len(roots) > 0 {
				p.holdRecord(r, p.groupRoots(roots...), "answersheets:invalid_testee_identity")
			}
			return nil
		}
		if len(p.groups[id].TesteeIDs) == 0 {
			domain, _ := graphMongoID(raw, "domain_id", true)
			if root := p.parents["sheet_root"][domain]; root != "" {
				p.holdRecord(r, p.groupRoots(root), "answersheets:foreign_testee_links_target_assessment")
			}
			return nil
		}
		return p.mongoCandidate("answersheets", r, id)
	}); e != nil {
		return e
	}
	if e := p.writer.flush(); e != nil {
		return e
	}
	for _, step := range []struct{ name, field, parent string }{{"report_generations", "outcome_id", "evaluation_outcome"}, {"interpretation_runs", "generation_id", "report_generations"}, {"interpret_report_artifacts", "generation_id", "report_generations"}, {"interpret_report_artifacts", "assessment_id", "assessment"}, {"interpret_report_artifacts", "outcome_id", "evaluation_outcome"}, {"interpret_report_artifacts", "testee_id", "testee"}, {"report_query_catalog", "assessment_id", "assessment"}, {"report_query_catalog", "testee_id", "testee"}} {
		if e := p.mongoByIDs(step.name, step.field, step.parent, p.ids(step.parent)); e != nil {
			return e
		}
		if e := p.writer.flush(); e != nil {
			return e
		}
	}
	if e := p.catalogSharedParents(); e != nil {
		return e
	}
	// No later stage revisits Mongo candidates or these parent-chain checks.
	p.seenMongo = nil
	for _, name := range []string{"enrollment_plan", "sheet_root", "generation_outcome", "run_generation"} {
		delete(p.parents, name)
	}
	return nil
}
func (p *leanPlanner) statistics() error {
	sources := []struct{ table, kind, parent string }{
		{"statistics_access_fact", "entry_intake", "assessment_entry_intake_log"}, {"statistics_access_fact", "clinician_relation", "clinician_relation"},
		{"statistics_plan_fact", "plan_enrollment", "plan_enrollment"}, {"statistics_plan_fact", "assessment_task", "assessment_task"},
		{"statistics_assessment_fact", "answersheet", "answersheets"}, {"statistics_assessment_fact", "assessment", "assessment"}, {"statistics_assessment_fact", "evaluation_outcome", "evaluation_outcome"}, {"statistics_assessment_fact", "interpret_report", "interpret_report_artifacts"}, {"statistics_assessment_fact", "interpretation_run", "interpretation_runs"},
	}
	p.statisticSeen = map[string]map[uint64]string{}
	for _, table := range []string{"statistics_access_fact", "statistics_plan_fact", "statistics_assessment_fact"} {
		p.statisticSeen[table] = map[uint64]string{}
		for _, source := range sources {
			if source.table != table {
				continue
			}

			e := p.indexedSQL(source.table, p.ids(source.parent), func(ids []string) (string, []any) {
				slots, args := placeholders(ids)
				args = append([]any{source.kind}, args...)
				return "x.source_type=? AND x.source_ref IN (" + slots + ")", args
			}, func(o graphObject) string { return p.owner(source.parent, o.Links["source_ref"]) }, func(o graphObject, root string) {
				p.checkStatisticParents(o, root)
				if o.Links["source_type"] != source.kind || o.Links["testee_id"] != "" && o.Links["testee_id"] != root {
					p.hold(o, source.table+":typed_statistic_source_ownership_conflict", root)
				}
			})
			if e != nil {
				return e
			}
			if e = p.writer.flush(); e != nil {
				return e
			}
		}
		if e := p.statisticParentIntersections(table); e != nil {
			return e
		}
		delete(p.statisticSeen, table)
	}

	for _, source := range []struct{ kind, parent string }{{"answersheet_submitted", "answersheets"}, {"assessment_completed", "assessment"}} {
		e := p.indexedSQL("statistics_store_activity_fact", p.ids(source.parent), func(ids []string) (string, []any) {
			slots, args := placeholders(ids)
			args = append([]any{p.selector.Selection.OrgID, source.kind}, args...)
			return "x.org_id=? AND x.fact_type=? AND x.business_object_id IN (" + slots + ")", args
		}, func(o graphObject) string { return p.owner(source.parent, o.Links["business_object_id"]) }, func(o graphObject, root string) {
			if o.Links["fact_type"] != source.kind || o.Links["source_ref"] != o.Links["business_object_id"] {
				p.hold(o, "statistics_store_activity_fact:typed_source_ref_conflict", root)
			}
		})
		if e != nil {
			return e
		}
		if e = p.writer.flush(); e != nil {
			return e
		}
	}
	return nil
}

func leanPlanCohort(ctx context.Context, reader graphReader, c cohortSelector, protection protected, dir, selectorSHA, source string) (graphIndex, error) {
	index := graphIndex{Format: 2, Kind: "mock_cohort_graph_index", Operation: c.Operation, SelectorSHA: selectorSHA, ProtectionSHA: c.ProtectionSHA, OwnerEvidenceSHA: c.OwnerEvidenceSHA, Source: source, Binding: c.Binding, Shards: []graphShard{}, Counts: map[string]int64{}, Retained: map[string]string{}, Holds: map[string]int64{}, HoldShards: []graphHoldShard{}, RequiredActions: map[string]int64{}, ReadOnly: true, WritePending: true}
	for name, rank := range graphSQLTables {
		if rank > 0 && name != "qs_rm_evaluation_request_ref" {
			index.Counts["mysql/"+name] = 0
		}
	}
	for name, rank := range graphMongoTables {
		if rank > 0 {
			index.Counts["mongo/"+name] = 0
		}
	}
	if e := validateSelector(c, protection); e != nil {
		return index, e
	}
	if !hashPattern.MatchString(selectorSHA) {
		return index, errors.New("exact selector hash required")
	}
	if e := privateDirectory(dir); e != nil {
		return index, e
	}
	protectedKeys := map[string]bool{}
	for _, o := range append(append(append([]object{}, protection.QS.System...), protection.QS.Clack...), protection.QS.Other...) {
		protectedKeys[objectID(o)] = true
	}
	core := &cohortPlanner{context: ctx, reader: reader, selector: c, writer: &graphWriter{dir: dir, index: &index, protected: protectedKeys}, dir: dir}
	p := &leanPlanner{cohortPlanner: core, groups: map[string]cohortIdentityGroup{}, owners: map[string]map[string]leanOwner{}, parents: map[string]map[string]string{}, seenMongo: map[string]string{}}
	for _, name := range []string{"enrollment_plan", "sheet_root", "generation_outcome", "run_generation", "event_root"} {
		p.parents[name] = map[string]string{}
	}
	for _, g := range c.IdentityGroups {
		for _, root := range g.TesteeIDs {
			p.groups[root] = g
		}
	}
	var guardedIAM iamScope
	if e := strictDecode(protection.IAM, &guardedIAM); e != nil {
		return index, e
	}
	protectedUsers := map[string]bool{}
	protectedProfiles := map[string]bool{}
	for _, id := range guardedIAM.Users {
		protectedUsers[id] = true
	}
	for _, id := range guardedIAM.Profiles {
		protectedProfiles[id] = true
	}
	for _, g := range c.IdentityGroups {
		intersects := false
		for _, id := range g.UserIDs {
			intersects = intersects || protectedUsers[id]
		}
		for _, id := range g.ProfileIDs {
			intersects = intersects || protectedProfiles[id]
		}
		if intersects {
			p.hold(graphObject{Object: keyRoot(g.TesteeIDs[0]), Links: map[string]string{}}, "identity_component_intersects_protected_iam", g.TesteeIDs...)
		}
	}
	from, _ := time.Parse(time.RFC3339, c.Selection.From)
	through, _ := time.Parse(time.RFC3339, c.Selection.Through)
	e := rootBatches(sortedRootIDs(c.TesteeIDs), 1000, func(ids []string) error {
		slots, args := placeholders(ids)
		args = append(args, c.Selection.OrgID, c.Selection.Source, from.Format("2006-01-02 15:04:05.999999"), through.Format("2006-01-02 15:04:05.999999"))
		return reader.sqlPages(ctx, "testee", "x.id IN ("+slots+") AND x.org_id=? AND BINARY x.source=BINARY ? AND x.created_at>=? AND x.created_at<=?", args, func(r record) error {
			id, _ := graphSQLValue(r, "id")
			o, e := p.sqlObject(r, p.groupRoots(id)...)
			if e != nil {
				return e
			}
			if len(p.groups[id].TesteeIDs) == 0 {
				return errors.New("root query returned undeclared identity")
			}
			profile, _ := graphSQLValue(r, "profile_id")
			found := false
			for _, v := range p.groups[id].ProfileIDs {
				found = found || v == profile
			}
			if !found {
				p.hold(o, "testee:profile_identity_component_conflict", id)
			}
			physicalKey := objectID(o.Object)
			p.remember(o, id, physicalKey)
			return p.emit(o, id, graphSQLTables["testee"], physicalKey)
		})
	})
	if e != nil {
		return index, e
	}
	if len(p.owners["testee"]) != len(c.TesteeIDs) {
		return index, errors.New("one or more frozen roots failed source/org/window verification")
	}
	if e = p.writer.flush(); e != nil {
		return index, e
	}
	for _, step := range []func() error{p.sharedIdentityIntersections, p.sqlChildren, p.mongoChildren, p.statistics, p.retainedResponsibilities} {
		if e = step(); e != nil {
			return index, e
		}
		if e = p.writer.flush(); e != nil {
			return index, e
		}
	}
	for _, name := range []string{"assessment_plan", "assessment_entry", "assessment_entry_resolve_log", "actor_stores", "operators", "clinician", "questionnaires", "assessment_models", "assessment_norms", "interpretation_report_templates"} {
		index.Retained[name] = "shared/formal definition retained; association does not grant deletion"
	}
	for _, id := range c.RetainPlanIDs {
		index.Retained["assessment_plan/"+id] = "explicit shared plan retained"
	}
	for _, name := range []string{"statistics_org_snapshot", "statistics_access_daily", "statistics_assessment_daily", "statistics_plan_activity_daily", "statistics_plan_fulfillment_daily", "statistics_store_activity_daily"} {
		index.Retained[name] = "shared buckets retained; rebuild only affected dates"
	}
	index.RequiredActions["targeted_cache_invalidation_before_reopen"] = 1
	index.RequiredActions["generator_and_prepared_input_retirement"] = 1
	if e = p.flushHolds(); e != nil {
		return index, e
	}
	index.DiscoveryComplete = true
	return index, nil
}

func (p *leanPlanner) retainedResponsibilities() error {
	summary, ok := p.reader.(interface {
		retainedCounts(context.Context, string) (map[string]int64, error)
	})
	if !ok {
		return errors.New("lean reader requires bounded retained-state aggregation")
	}
	counts, e := summary.retainedCounts(p.context, p.selector.Selection.OrgID)
	if e != nil {
		return e
	}
	for key, n := range counts {
		p.writer.index.Counts[key] = n
	}
	for _, name := range []string{"interpretation_attention_projection", "task_opened_reminder_batch", "task_opened_reminder_delivery", "qs_rm_evaluation_request_ref", "event_delivery_dead_letter", "retry_event_hold", "qs_rm_replay_items", "qs_rm_replay_requests", "mysql/rm_outbox", "mongo/rm_outbox", "ai_bridge_requests"} {
		p.writer.index.Retained[name] = "original identity, successful replay guard and settled audit remain online; state counts are explicitly all-org or exact-target"
	}
	// AI was proven outside this cohort. A new exact Testee intersection is
	// retained and holds its component; this tool never gains an AI purge scope.
	if e = rootBatches(sortedRootIDs(p.selector.TesteeIDs), 1000, func(ids []string) error {
		slots, args := placeholders(ids)
		return p.reader.sqlPages(p.context, "ai_bridge_requests", "x.testee_id IN ("+slots+")", args, func(r record) error {
			root, _ := graphSQLValue(r, "testee_id")
			p.holdRecord(r, p.groupRoots(root), "ai_bridge_requests:requires_separate_shared_reference_resolution")
			p.writer.index.Counts["retained/exact_target/mysql/ai_bridge_requests"]++
			return nil
		})
	}); e != nil {
		return e
	}
	// A durable evaluation reference is audit identity, not a deletion target.
	if e = rootBatches(p.ids("assessment"), 1000, func(ids []string) error {
		slots, args := placeholders(ids)
		return p.reader.sqlPages(p.context, "qs_rm_evaluation_request_ref", "x.assessment_id IN ("+slots+")", args, func(r record) error {
			id, _ := graphSQLValue(r, "assessment_id")
			root := p.owner("assessment", id)
			event, _ := graphSQLValue(r, "event_id")
			p.writer.index.Counts["retained/exact_target/mysql/qs_rm_evaluation_request_ref"]++
			if root != "" && event != "" {
				p.eventOwner(event, root)
			}
			return nil
		})
	}); e != nil {
		return e
	}
	// Indexed Report and Task predicates inspect only the applicable technical
	// identities; settled rows never enter scope shards or original archives.
	for _, step := range []struct{ name, column, parent string }{{"interpretation_attention_projection", "report_id", "interpret_report_artifacts"}, {"task_opened_reminder_batch", "task_id", "assessment_task"}, {"task_opened_reminder_delivery", "task_id", "assessment_task"}} {
		if e = rootBatches(p.ids(step.parent), 1000, func(ids []string) error {
			slots, args := placeholders(ids)
			where := "x.`" + step.column + "` IN (" + slots + ")"
			if step.name != "interpretation_attention_projection" {
				where = "x.org_id=? AND " + where
				args = append([]any{p.selector.Selection.OrgID}, args...)
			}
			return p.reader.sqlPages(p.context, step.name, where, args, func(r record) error {
				id, _ := graphSQLValue(r, step.column)
				root := p.owner(step.parent, id)
				o, e := p.sqlObject(r, p.groupRoots(root)...)
				if e != nil {
					return e
				}
				p.writer.index.Counts["retained/exact_target/mysql/"+step.name]++
				if o.Links["testee_id"] != "" && o.Links["testee_id"] != root {
					p.hold(o, step.name+":retained_owner_conflict", root, o.Links["testee_id"])
				}
				return nil
			})
		}); e != nil {
			return e
		}
	}
	for _, step := range []struct{ name, where string }{{"event_delivery_dead_letter", "x.org_id=? AND x.retry_disposition NOT IN ('archived_mock','terminal')"}, {"retry_event_hold", "x.org_id=? AND (x.status<>'replayed' OR COALESCE(x.retry_disposition,'') NOT IN ('','terminal') OR x.claim_token IS NOT NULL OR x.next_attempt_at IS NOT NULL)"}} {
		if e = p.reader.sqlPages(p.context, step.name, step.where, []any{p.selector.Selection.OrgID}, func(r record) error {
			testee, _ := graphSQLValue(r, "testee_id")
			assessment, _ := graphSQLValue(r, "assessment_id")
			root := testee
			if p.groups[root].TesteeIDs == nil {
				root = p.owner("assessment", assessment)
			}
			if root == "" {
				p.writer.index.Counts["retained/unassigned_unsettled/mysql/"+step.name]++
				return nil
			}
			o, e := p.sqlObject(r, p.groupRoots(root)...)
			if e != nil {
				return e
			}
			p.hold(o, step.name+":unsettled_replay_responsibility", root, testee, p.owner("assessment", assessment))
			return nil
		}); e != nil {
			return e
		}
	}
	// Published does not prove downstream FIN. Those original wires are retained;
	// the independently frozen broker/manual-replay settlement remains required.
	if e = p.reader.sqlPages(p.context, "rm_outbox", "x.scope=? AND (x.state<>'published' OR x.claim_token IS NOT NULL OR x.lease_until IS NOT NULL)", []any{"org:" + p.selector.Selection.OrgID}, func(r record) error {
		id, _ := graphSQLValue(r, "message_id")
		root := p.parents["event_root"][id]
		if root == "" {
			p.writer.index.Counts["retained/unassigned_unsettled/mysql/rm_outbox"]++
			return nil
		}
		o, e := p.sqlObject(r, p.groupRoots(root)...)
		if e != nil {
			return e
		}
		p.hold(o, "mysql/rm_outbox:unsettled_publish_responsibility", root)
		return nil
	}); e != nil {
		return e
	}
	filter := bson.D{{Key: "scope", Value: "org:" + p.selector.Selection.OrgID}, {Key: "$or", Value: bson.A{bson.D{{Key: "state", Value: bson.D{{Key: "$ne", Value: "published"}}}}, bson.D{{Key: "claim_token", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$nin", Value: bson.A{nil, ""}}}}}, bson.D{{Key: "lease_until", Value: bson.D{{Key: "$ne", Value: nil}}}}}}}
	if e = p.reader.mongoPages(p.context, "rm_outbox", filter, func(r record) error {
		raw := bson.Raw(r.BSON)
		root := p.parents["event_root"][graphMongoString(raw, "message_id")]
		if root == "" {
			p.writer.index.Counts["retained/unassigned_unsettled/mongo/rm_outbox"]++
			return nil
		}
		p.holdRecord(r, p.groupRoots(root), "mongo/rm_outbox:unsettled_publish_responsibility")
		return nil
	}); e != nil {
		return e
	}
	p.writer.index.RequiredActions["frozen_broker_and_manual_replay_settlement"] = 1
	return nil
}
func (p *leanPlanner) eventOwner(event, root string) {
	if event != "" && root != "" {
		if old := p.parents["event_root"][event]; old != "" && old != root {
			p.hold(graphObject{Object: keyRoot(root), Links: map[string]string{}}, "shared_message_identity", root, old)
			return
		}
		p.parents["event_root"][event] = root
	}
}

func observeLeanProtection(ctx context.Context, reader graphReader, p protected) ([]object, error) {
	guards := sortedObjects(append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...))
	out := []object{}
	for _, expected := range guards {
		count := 0
		emit := func(r record) error {
			count++
			if !reflect.DeepEqual(r.Object.Key, expected.Key) || r.Object.Store != expected.Store || r.Object.Name != expected.Name {
				return errors.New("protected technical identity changed")
			}
			out = append(out, r.Object)
			return nil
		}
		var e error
		if expected.Store == "mysql" {
			where, args, err := keyWhere(expected)
			if err != nil {
				return nil, err
			}
			e = reader.sqlPages(ctx, expected.Name, where, args, emit)
		} else {
			filter, err := mongoKey(expected)
			if err != nil {
				return nil, err
			}
			e = reader.mongoPages(ctx, expected.Name, filter, emit)
		}
		if e != nil {
			return nil, e
		}
		if count != 1 {
			return nil, errors.New("protected physical identity is missing or ambiguous")
		}
	}
	return out, nil
}

func (p *leanPlanner) sharedIdentityIntersections() error {
	profiles := []string{}
	users := []string{}
	profileRoot := map[string]string{}
	userRoot := map[string]string{}
	for _, g := range p.selector.IdentityGroups {
		for _, id := range g.ProfileIDs {
			profiles = append(profiles, id)
			profileRoot[id] = g.TesteeIDs[0]
		}
		for _, id := range g.UserIDs {
			users = append(users, id)
			userRoot[id] = g.TesteeIDs[0]
		}
	}
	for _, step := range []struct {
		name, column string
		ids          []string
		roots        map[string]string
	}{{"testee", "profile_id", profiles, profileRoot}, {"operators", "user_id", users, userRoot}} {
		if e := rootBatches(step.ids, 1000, func(ids []string) error {
			slots, args := placeholders(ids)
			return p.reader.sqlPages(p.context, step.name, "x.`"+step.column+"` IN ("+slots+")", args, func(r record) error {
				id, _ := graphSQLValue(r, step.column)
				root := step.roots[id]
				if root == "" {
					return errors.New("shared identity query returned an unbound component")
				}
				rowID, _ := graphSQLValue(r, "id")
				if step.name == "operators" || p.groups[rowID].TesteeIDs == nil || !sameRootGroup(p.groups[root], p.groups[rowID]) {
					p.holdRecord(r, p.groupRoots(root), step.name+":identity_shared_outside_frozen_cohort")
					p.writer.index.Counts["retained/shared_identity/"+step.name]++
				}
				return nil
			})
		}); e != nil {
			return e
		}
	}
	return nil
}
func sameRootGroup(a, b cohortIdentityGroup) bool {
	return len(a.TesteeIDs) > 0 && len(b.TesteeIDs) > 0 && a.TesteeIDs[0] == b.TesteeIDs[0]
}

// Catalog has indexed Testee and Assessment ownership, but no leading index for
// Generation/Outcome. One directed technical query checks only foreign owners
// that refer to frozen parents; it never repeats an unindexed scan per ID page.
func (p *leanPlanner) catalogSharedParents() error {
	generations, outcomes := p.ids("report_generations"), p.ids("evaluation_outcome")
	if len(generations) == 0 && len(outcomes) == 0 {
		return nil
	}
	parents := bson.A{}
	if len(generations) > 0 {
		parents = append(parents, bson.D{{Key: "generation_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(generations)}}}})
	}
	if len(outcomes) > 0 {
		parents = append(parents, bson.D{{Key: "outcome_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(outcomes)}}}})
	}
	filter := bson.D{{Key: "$or", Value: parents}, {Key: "$nor", Value: bson.A{bson.D{{Key: "testee_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(p.selector.TesteeIDs)}}}}, bson.D{{Key: "assessment_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(p.ids("assessment"))}}}}}}}
	encoded, e := bson.Marshal(filter)
	if e != nil {
		return e
	}
	if len(encoded) > 15*1024*1024 {
		return errors.New("fixed catalog shared-parent query exceeds bounded command; choose a smaller complete component subset")
	}
	return p.reader.mongoPages(p.context, "report_query_catalog", filter, func(r record) error {
		raw := bson.Raw(r.BSON)
		testee, _ := graphMongoID(raw, "testee_id", true)
		assessment, _ := graphMongoID(raw, "assessment_id", true)
		if len(p.groups[testee].TesteeIDs) > 0 || p.owner("assessment", assessment) != "" {
			return nil
		}
		roots := []string{}
		for _, field := range []struct{ key, parent string }{{"generation_id", "report_generations"}, {"outcome_id", "evaluation_outcome"}} {
			id, e := graphMongoID(raw, field.key, true)
			if e == nil {
				roots = append(roots, p.owner(field.parent, id))
				continue
			}
			hints := []string{}
			for candidate, entry := range p.owners[field.parent] {
				if graphRawIDMember(raw, field.key, map[string]bool{candidate: true}) {
					hints = append(hints, entry.Root)
				}
			}
			if len(hints) == 0 {
				return errors.New("catalog shared-parent identity cannot be assigned to a frozen component")
			}
			roots = append(roots, hints...)
		}
		roots = p.groupRoots(roots...)

		if len(roots) > 0 {
			p.holdRecord(r, roots, "report_query_catalog:retained_object_links_target_parent")
		}
		return nil
	})
}

func statisticPhysicalID(o object) (uint64, error) {
	if o.Store != "mysql" || len(o.Key) != 1 || o.Key[0].Name != "id" || o.Key[0].Type != "uint64" {
		return 0, errors.New("statistics scope requires its actual unsigned physical ID")
	}
	id, e := strconv.ParseUint(string(o.Key[0].Raw), 10, 64)
	if e != nil || id == 0 || strconv.FormatUint(id, 10) != string(o.Key[0].Raw) {
		return 0, errors.New("statistics physical ID is not canonical")
	}
	return id, nil
}
func statisticParentDomains(table string) []struct{ column, parent string } {
	switch table {
	case "statistics_assessment_fact":
		return []struct{ column, parent string }{{"answersheet_id", "answersheets"}, {"assessment_id", "assessment"}, {"outcome_id", "evaluation_outcome"}, {"report_id", "interpret_report_artifacts"}, {"enrollment_id", "plan_enrollment"}, {"task_id", "assessment_task"}}
	case "statistics_plan_fact":
		return []struct{ column, parent string }{{"enrollment_id", "plan_enrollment"}, {"task_id", "assessment_task"}}
	}
	return nil
}
func (p *leanPlanner) checkStatisticParents(o graphObject, root string) {
	for _, domain := range statisticParentDomains(o.Object.Name) {
		p.parent(o, domain.column, domain.parent, root, false)
	}
}
func (p *leanPlanner) statisticParentIntersections(table string) error {
	parts := []string{}
	args := []any{}
	// One directed server-side query per namespace, including the unindexed
	// denormalized columns. It never restarts their candidate scan per ID batch.
	domains := append([]struct{ column, parent string }{{"testee_id", "testee"}}, statisticParentDomains(table)...)
	for _, domain := range domains {
		ids := p.ids(domain.parent)
		if len(ids) == 0 {
			continue
		}
		parts = append(parts, "x.`"+domain.column+"` IN (SELECT fixed_ids.id FROM JSON_TABLE(?, '$[*]' COLUMNS(id BIGINT UNSIGNED PATH '$' ERROR ON ERROR)) fixed_ids)")
		args = append(args, string(encode(ids)))
	}
	if len(parts) == 0 {
		return nil
	}
	single, ok := p.reader.(interface {
		sqlOneCursor(context.Context, string, string, []any, func(record) error) error
	})
	if !ok {
		return errors.New("statistics intersections require a bounded technical single-cursor reader")
	}
	return single.sqlOneCursor(p.context, table, "("+strings.Join(parts, " OR ")+")", args, func(r record) error {
		id, e := statisticPhysicalID(r.Object)
		if e != nil {
			return e
		}
		if _, seen := p.statisticSeen[table][id]; seen {
			return nil
		}
		roots := []string{}
		for _, domain := range domains {
			value, _ := graphSQLValue(r, domain.column)
			if root := p.owner(domain.parent, value); root != "" {
				roots = append(roots, root)
			}
		}
		roots = p.groupRoots(roots...)
		if len(roots) == 0 {
			return nil
		}
		o, e := p.sqlObject(r, roots...)
		if e != nil {
			return e
		}
		p.hold(o, table+":retained_row_links_frozen_typed_parent", roots...)
		p.writer.index.Counts["retained/foreign_or_mixed/mysql/"+table]++
		// A foreign source row never acquires deletion authority from one matching
		// numeric column. The named parent domains determine only its held groups.
		return nil
	})
}
