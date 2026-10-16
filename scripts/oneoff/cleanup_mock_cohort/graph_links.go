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

// SQL typed parents are already frozen before the single root-collection scan.
// A retained Testee does not make a crosslinked artifact safe to ignore.
