package main

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
)

func uniqueLinks(objects []graphObject, key string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, o := range objects {
		v := o.Links[key]
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
func (p *cohortPlanner) sqlLinkOwners(ctx context.Context, name string, ids []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > 1000 {
		return out, errors.New("ownership lookup exceeds fixed SQL batch")
	}
	e := p.reader.sqlOwnerPages(ctx, name, ids, func(r record) error {
		links := map[string]string{}
		for _, key := range []string{"id", "testee_id", "org_id", "assessment_id", "outcome_id", "plan_id", "enrollment_id"} {
			if value, ok := graphSQLValue(r, key); ok {
				links[key] = value
			}
		}
		id := links["id"]
		if !graphDecimal(id) {
			return errors.New("actual ownership row has no typed domain ID")
		}
		out[id] = links
		return nil
	})
	return out, e
}
func (p *cohortPlanner) mongoRootBatch(ctx context.Context, records []record) error {
	if len(records) == 0 {
		return nil
	}
	if len(records) > 500 {
		return errors.New("mongo ownership batch exceeds fixed 500")
	}
	objects := make([]graphObject, len(records))
	for i, r := range records {
		o, e := p.mongoObject(r)
		if e != nil {
			return e
		}
		objects[i] = o
	}
	assessments, e := p.sqlLinkOwners(ctx, "assessment", uniqueLinks(objects, "assessment_id"))
	if e != nil {
		return e
	}
	outcomes, e := p.sqlLinkOwners(ctx, "evaluation_outcome", uniqueLinks(objects, "outcome_id"))
	if e != nil {
		return e
	}
	tasks := []string{}
	seen := map[string]bool{}
	for _, o := range objects {
		for _, key := range []string{"task_id", "attribution_task_id"} {
			v := o.Links[key]
			if v != "" && !seen[v] {
				if !graphDecimal(v) {
					return errors.New("task link must be canonical decimal string")
				}
				seen[v] = true
				tasks = append(tasks, v)
			}
		}
	}
	taskOwners, e := p.sqlLinkOwners(ctx, "assessment_task", tasks)
	if e != nil {
		return e
	}
	generations, e := p.mongoLinkOwners(ctx, "report_generations", uniqueLinks(objects, "generation_id"))
	if e != nil {
		return e
	}
	runs, e := p.mongoLinkOwners(ctx, "interpretation_runs", uniqueLinks(objects, "interpretation_run_id"))
	if e != nil {
		return e
	}
	artifacts, e := p.mongoLinkOwners(ctx, "interpret_report_artifacts", uniqueLinks(objects, "source_id"))
	if e != nil {
		return e
	}
	for _, o := range objects {
		if id := o.Links["assessment_id"]; id != "" {
			a, ok := assessments[id]
			if !ok {
				p.holdObject(o, o.Object.Name+":missing_assessment_ownership")
			} else if a["testee_id"] != o.Links["testee_id"] || a["org_id"] != o.Links["org_id"] {
				p.holdObject(o, o.Object.Name+":assessment_ownership_conflict")
			}
		}
		if id := o.Links["outcome_id"]; id != "" {
			a, ok := outcomes[id]
			if !ok {
				p.holdObject(o, o.Object.Name+":missing_outcome_ownership")
			} else if a["assessment_id"] != o.Links["assessment_id"] || a["testee_id"] != o.Links["testee_id"] || a["org_id"] != o.Links["org_id"] {
				p.holdObject(o, o.Object.Name+":outcome_ownership_conflict")
			}
		}
		if id := o.Links["generation_id"]; id != "" {
			g, ok := generations[id]
			if !ok || g["outcome_id"] != o.Links["outcome_id"] {
				p.holdObject(o, o.Object.Name+":generation_ownership_conflict")
			}
		}
		if id := o.Links["interpretation_run_id"]; id != "" {
			r, ok := runs[id]
			if !ok || r["generation_id"] != o.Links["generation_id"] {
				p.holdObject(o, o.Object.Name+":run_ownership_conflict")
			}
		}
		if o.Object.Name == "report_query_catalog" {
			r, ok := artifacts[o.Links["source_id"]]
			if !ok || r["org_id"] != o.Links["org_id"] || r["testee_id"] != o.Links["testee_id"] || r["assessment_id"] != o.Links["assessment_id"] || r["outcome_id"] != o.Links["outcome_id"] || r["generation_id"] != o.Links["generation_id"] {
				p.holdObject(o, "report_query_catalog:artifact_ownership_conflict")
			}
		}
		for _, key := range []string{"task_id", "attribution_task_id"} {
			if id := o.Links[key]; id != "" {
				t, ok := taskOwners[id]
				if !ok {
					p.holdObject(o, "answersheets:missing_task_ownership")
				} else if t["testee_id"] != o.Links["testee_id"] || t["org_id"] != o.Links["org_id"] || o.Links["attribution_enrollment_id"] != "" && t["enrollment_id"] != o.Links["attribution_enrollment_id"] || o.Links["attribution_plan_id"] != "" && t["plan_id"] != o.Links["attribution_plan_id"] {
					p.holdObject(o, "answersheets:task_ownership_conflict")
				}
			}
		}
		if e := p.writer.add(o, graphMongoTables[o.Object.Name]); e != nil {
			return e
		}
	}
	return nil
}
func (p *cohortPlanner) rootMongo(ctx context.Context, name string, ids []string) error {
	if e := p.loadSQLReverseRoots(); e != nil {
		return e
	}
	roots := map[string]bool{}
	for _, id := range ids {
		roots[id] = true
	}
	batch := []record{}
	size := 0
	flush := func() error { e := p.mongoRootBatch(ctx, batch); batch = nil; size = 0; return e }
	// The production AnswerSheet schema has no Testee index. Walk each of these
	// collections once in _id order with cursor batches <=500, and retain only
	// the small typed root set in RAM. Repeating an unindexed $in per root batch
	// would multiply whole-collection work. Raw retained documents are discarded.
	if e := p.reader.mongoPages(ctx, name, bson.D{}, func(r record) error {
		raw := bson.Raw(r.BSON)
		testee, e := graphMongoID(raw, "testee_id", false)
		if e != nil {
			hints := []string{}
			v := raw.Lookup("testee_id")
			candidate := ""
			if v.Type == bson.TypeInt32 && v.Int32() > 0 {
				candidate = fmt.Sprint(v.Int32())
			}
			if v.Type == bson.TypeString {
				candidate = v.StringValue()
			}
			if roots[candidate] {
				hints = append(hints, candidate)
			}
			p.holdRecord(r, hints, name+":unassignable_testee_identity")
			return nil
		}
		if !roots[testee] {
			if linked := p.foreignSQLParentRoots(raw); len(linked) > 0 {
				p.holdRecord(r, linked, name+":retained_object_links_target_parent")
			}
			return nil
		}
		n := len(encode(r))
		if len(batch) > 0 && size+n > 64*1024*1024 {
			if e := flush(); e != nil {
				return e
			}
		}
		size += n
		batch = append(batch, r)
		if len(batch) == 500 {
			return flush()
		}
		return nil
	}); e != nil {
		return e
	}
	return flush()
}

func requireGraphLinks(o graphObject, names ...string) error {
	for _, key := range names {
		if !graphDecimal(o.Links[key]) {
			return fmt.Errorf("%s missing typed %s identity", o.Object.Name, key)
		}
	}
	return nil
}

func (p *cohortPlanner) sqlOwnedBatch(ctx context.Context, records []record) error {
	if len(records) == 0 {
		return nil
	}
	// Resolve the narrow parent IDs before lifecycle/evidence classification,
	// so a pending child hold is already bound to its exact typed component.
	objects := make([]graphObject, len(records))
	for i, r := range records {
		objects[i].Links = map[string]string{}
		for _, name := range []string{"assessment_id", "enrollment_id", "task_id"} {
			if value, ok := graphSQLValue(r, name); ok {
				objects[i].Links[name] = value
			}
		}
	}
	assessments, e := p.sqlLinkOwners(ctx, "assessment", uniqueLinks(objects, "assessment_id"))
	if e != nil {
		return e
	}
	enrollments, e := p.sqlLinkOwners(ctx, "plan_enrollment", uniqueLinks(objects, "enrollment_id"))
	if e != nil {
		return e
	}
	tasks, e := p.sqlLinkOwners(ctx, "assessment_task", uniqueLinks(objects, "task_id"))
	if e != nil {
		return e
	}
	for i, links := range objects {
		roots := []string{}
		if owner, ok := assessments[links.Links["assessment_id"]]; ok {
			roots = append(roots, owner["testee_id"])
		}
		if owner, ok := tasks[links.Links["task_id"]]; ok {
			roots = append(roots, owner["testee_id"])
		}
		o, e := p.sqlObject(records[i], roots...)
		if e != nil {
			return e
		}
		if owner, ok := assessments[o.Links["assessment_id"]]; ok && o.Links["testee_id"] == "" {
			o.RootIDs = []string{owner["testee_id"]}
		}
		if id := o.Links["task_id"]; id != "" {
			t, ok := tasks[id]
			if !ok {
				p.holdObject(o, o.Object.Name+":missing_task_ownership")
			} else {
				o.RootIDs = []string{t["testee_id"]}
				if o.Links["org_id"] != "" && t["org_id"] != o.Links["org_id"] {
					p.holdObject(o, o.Object.Name+":task_ownership_conflict")
				}
			}
		}

		if id := o.Links["assessment_id"]; id != "" && id != "0" {
			a, ok := assessments[id]
			if !ok {
				p.holdObject(o, o.Object.Name+":missing_assessment_ownership")
			} else if o.Links["testee_id"] != "" && a["testee_id"] != o.Links["testee_id"] || o.Links["org_id"] != "" && a["org_id"] != o.Links["org_id"] {
				p.holdObject(o, o.Object.Name+":assessment_ownership_conflict")
			}
		}
		if id := o.Links["enrollment_id"]; id != "" && id != "0" {
			e, ok := enrollments[id]
			if !ok {
				p.holdObject(o, o.Object.Name+":missing_enrollment_ownership")
			} else if e["org_id"] != o.Links["org_id"] || e["testee_id"] != o.Links["testee_id"] || e["plan_id"] != o.Links["plan_id"] {
				p.holdObject(o, o.Object.Name+":enrollment_ownership_conflict")
			}
		}
		if e := p.writer.add(o, graphSQLTables[o.Object.Name]); e != nil {
			return e
		}
	}
	return nil
}

func (p *cohortPlanner) mongoLinkOwners(ctx context.Context, name string, ids []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > 500 {
		return out, errors.New("mongo link ownership batch exceeds fixed 500")
	}
	e := p.reader.mongoOwnerPages(ctx, name, ids, func(r record) error {
		raw := bson.Raw(r.BSON)
		links := map[string]string{}
		for _, name := range []string{"domain_id", "org_id", "testee_id", "assessment_id", "outcome_id", "generation_id", "interpretation_run_id", "report_id", "latest_run_id"} {
			v, e := graphMongoID(raw, name, true)
			if e != nil {
				return e
			}
			if v != "" {
				links[name] = v
			}
		}
		id := links["domain_id"]
		if id == "" {
			return errors.New("mongo ownership document missing original domain identity")
		}
		if _, exists := out[id]; exists {
			return errors.New("ambiguous duplicate logical Mongo domain ID")
		}
		out[id] = links
		return nil
	})
	return out, e
}

func (p *cohortPlanner) deliveryCorrelations(ctx context.Context, candidates []deliveryCandidate, roots map[string]bool, assessments map[string]string) error {
	eligible := []deliveryCandidate{}
	ids := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, c := range candidates {
		if !c.recognized || c.orgID != p.selector.Selection.OrgID {
			continue
		}
		owned := roots[c.testeeID]
		if owner, ok := assessments[c.assessmentID]; ok {
			owned = roots[owner]
		}
		if !owned {
			continue
		}
		eligible = append(eligible, c)
		for key, id := range c.links {
			if seen[key] == nil {
				seen[key] = map[string]bool{}
			}
			if !seen[key][id] {
				ids[key] = append(ids[key], id)
				seen[key][id] = true
			}
		}
	}
	mongoOwners := map[string]map[string]map[string]string{}
	for _, source := range []struct{ key, collection string }{{"run_id", "interpretation_runs"}, {"generation_id", "report_generations"}, {"report_id", "interpret_report_artifacts"}, {"answersheet_id", "answersheets"}} {
		all := map[string]map[string]string{}
		if e := rootBatches(ids[source.key], 500, func(batch []string) error {
			owners, e := p.mongoLinkOwners(ctx, source.collection, batch)
			if e != nil {
				return e
			}
			for id, owner := range owners {
				all[id] = owner
			}
			return nil
		}); e != nil {
			return e
		}
		mongoOwners[source.key] = all
	}
	sqlOwners := map[string]map[string]map[string]string{}
	for _, source := range []struct{ key, table string }{{"outcome_id", "evaluation_outcome"}, {"task_id", "assessment_task"}, {"enrollment_id", "plan_enrollment"}} {
		owners, e := p.sqlLinkOwners(ctx, source.table, ids[source.key])
		if e != nil {
			return e
		}
		sqlOwners[source.key] = owners
	}
	for _, c := range eligible {
		for key, id := range c.links {
			switch key {
			case "outcome_id", "task_id", "enrollment_id":
				owner, ok := sqlOwners[key][id]
				if !ok || owner["org_id"] != c.orgID || c.testeeID != "" && owner["testee_id"] != c.testeeID || key == "outcome_id" && c.assessmentID != "" && owner["assessment_id"] != c.assessmentID {
					p.holdRecord(c.r, []string{c.testeeID}, c.r.Object.Name+":"+key+"_ownership_conflict")
				}
			case "generation_id":
				owner, ok := mongoOwners[key][id]
				if !ok || c.links["outcome_id"] != "" && owner["outcome_id"] != c.links["outcome_id"] {
					p.holdRecord(c.r, []string{c.testeeID}, c.r.Object.Name+":generation_reference_conflict")
				}
			case "run_id":
				owner, ok := mongoOwners[key][id]
				if !ok || c.links["generation_id"] != "" && owner["generation_id"] != c.links["generation_id"] {
					p.holdRecord(c.r, []string{c.testeeID}, c.r.Object.Name+":run_reference_conflict")
				}
			case "report_id", "answersheet_id":
				owner, ok := mongoOwners[key][id]
				if !ok || owner["org_id"] != c.orgID || c.testeeID != "" && owner["testee_id"] != c.testeeID || key == "report_id" && (c.assessmentID != "" && owner["assessment_id"] != c.assessmentID || c.links["outcome_id"] != "" && owner["outcome_id"] != c.links["outcome_id"] || c.links["generation_id"] != "" && owner["generation_id"] != c.links["generation_id"]) {
					p.holdRecord(c.r, []string{c.testeeID}, c.r.Object.Name+":"+key+"_reference_conflict")
				}
			case "plan_id":
				if t, ok := sqlOwners["task_id"][c.links["task_id"]]; ok && t["plan_id"] != id {
					p.holdRecord(c.r, []string{c.testeeID}, c.r.Object.Name+":plan_reference_conflict")
				}
				if e, ok := sqlOwners["enrollment_id"][c.links["enrollment_id"]]; ok && e["plan_id"] != id {
					p.holdRecord(c.r, []string{c.testeeID}, c.r.Object.Name+":plan_reference_conflict")
				}
			}
		}
	}
	return nil
}

func (p *cohortPlanner) generationPage(ctx context.Context, outcomeIDs []string) error {
	owners, e := p.sqlLinkOwners(ctx, "evaluation_outcome", outcomeIDs)
	if e != nil {
		return e
	}
	return p.reader.mongoPages(ctx, "report_generations", bson.D{{Key: "outcome_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(outcomeIDs)}}}}, func(r record) error {
		id, e := graphMongoID(bson.Raw(r.BSON), "outcome_id", false)
		if e != nil {
			return e
		}
		owner, ok := owners[id]
		roots := []string{}
		if ok {
			roots = append(roots, owner["testee_id"])
		}
		o, e := p.mongoObject(r, roots...)
		if e != nil {
			return e
		}
		if !ok {
			p.holdObject(o, "report_generations:missing_outcome_ownership")
		} else {
			o.RootIDs = []string{owner["testee_id"]}
		}
		return p.writer.add(o, graphMongoTables[r.Object.Name])
	})
}
func (p *cohortPlanner) runPage(ctx context.Context, generationIDs []string) error {
	parents, e := p.mongoLinkOwners(ctx, "report_generations", generationIDs)
	if e != nil {
		return e
	}
	outcomeIDs := []string{}
	seen := map[string]bool{}
	for _, parent := range parents {
		id := parent["outcome_id"]
		if id != "" && !seen[id] {
			outcomeIDs = append(outcomeIDs, id)
			seen[id] = true
		}
	}
	owners, e := p.sqlLinkOwners(ctx, "evaluation_outcome", outcomeIDs)
	if e != nil {
		return e
	}
	return p.reader.mongoPages(ctx, "interpretation_runs", bson.D{{Key: "generation_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(generationIDs)}}}}, func(r record) error {
		id, e := graphMongoID(bson.Raw(r.BSON), "generation_id", false)
		if e != nil {
			return e
		}
		parent, ok := parents[id]
		owner, hasOwner := owners[parent["outcome_id"]]
		roots := []string{}
		if ok && hasOwner {
			roots = append(roots, owner["testee_id"])
		}
		o, e := p.mongoObject(r, roots...)
		if e != nil {
			return e
		}
		if !ok || !hasOwner {
			p.holdObject(o, "interpretation_runs:missing_generation_ownership")
		} else {
			o.RootIDs = []string{owner["testee_id"]}
		}
		return p.writer.add(o, graphMongoTables[r.Object.Name])
	})
}

// SQL typed parents are already frozen before the single root-collection scan.
// A retained Testee does not make a crosslinked artifact safe to ignore.
func (p *cohortPlanner) loadSQLReverseRoots() error {
	if p.reverseSQLRoots != nil {
		return nil
	}
	p.reverseSQLRoots = map[string]map[string][]string{}
	for _, ref := range p.writer.index.Shards {
		if ref.Store != "mysql" || (ref.Name != "assessment" && ref.Name != "evaluation_outcome" && ref.Name != "assessment_task" && ref.Name != "plan_enrollment") {
			continue
		}
		page, e := readGraphPage(p.dir, ref)
		if e != nil {
			return e
		}
		if p.reverseSQLRoots[ref.Name] == nil {
			p.reverseSQLRoots[ref.Name] = map[string][]string{}
		}
		for _, o := range page.Objects {
			p.reverseSQLRoots[ref.Name][o.Links["id"]] = graphRoots(o)
		}
	}
	return nil
}
func (p *cohortPlanner) foreignSQLParentRoots(raw bson.Raw) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(v bson.RawValue, domain string) {
		id := ""
		switch v.Type {
		case bson.TypeInt64:
			id = fmt.Sprint(v.Int64())
		case bson.TypeInt32:
			id = fmt.Sprint(v.Int32())
		case bson.TypeString:
			id = v.StringValue()
		}
		for _, root := range p.reverseSQLRoots[domain][id] {
			if !seen[root] {
				seen[root] = true
				out = append(out, root)
			}
		}
	}
	for _, v := range []struct{ key, domain string }{{"assessment_id", "assessment"}, {"outcome_id", "evaluation_outcome"}, {"task_id", "assessment_task"}} {
		add(raw.Lookup(v.key), v.domain)
	}
	if v := raw.Lookup("attribution"); v.Type == bson.TypeEmbeddedDocument {
		add(v.Document().Lookup("task_id"), "assessment_task")
		add(v.Document().Lookup("enrollment_id"), "plan_enrollment")
	}
	return out
}
