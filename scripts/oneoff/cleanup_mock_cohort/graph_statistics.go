package main

import (
	"context"
	"strings"
)

// A denormalized Testee column alone is not source ownership. Statistics keep
// their actual source domain; equal decimals in another source_type never join.
func (p *cohortPlanner) statisticBatch(ctx context.Context, records []record) error {
	sources := map[string]struct{ store, name string }{"entry_intake": {"mysql", "assessment_entry_intake_log"}, "clinician_relation": {"mysql", "clinician_relation"}, "plan_enrollment": {"mysql", "plan_enrollment"}, "assessment_task": {"mysql", "assessment_task"}, "answersheet": {"mongo", "answersheets"}, "assessment": {"mysql", "assessment"}, "evaluation_outcome": {"mysql", "evaluation_outcome"}, "interpret_report": {"mongo", "interpret_report_artifacts"}, "interpretation_run": {"mongo", "interpretation_runs"}}
	objects := make([]graphObject, len(records))
	ids := map[string][]string{}
	seen := map[string]map[string]bool{}
	for i, r := range records {
		o, e := p.sqlObject(r)
		if e != nil {
			return e
		}
		objects[i] = o
		kind, id := o.Links["source_type"], o.Links["source_ref"]
		_, known := sources[kind]
		allowed := r.Object.Name == "statistics_access_fact" && (kind == "entry_intake" || kind == "clinician_relation") || r.Object.Name == "statistics_plan_fact" && (kind == "plan_enrollment" || kind == "assessment_task") || r.Object.Name == "statistics_assessment_fact" && (kind == "answersheet" || kind == "assessment" || kind == "evaluation_outcome" || kind == "interpret_report" || kind == "interpretation_run")
		if !known || !allowed || !graphDecimal(id) {
			p.holdObject(o, r.Object.Name+":unknown_typed_statistic_source")
			continue
		}
		if seen[kind] == nil {
			seen[kind] = map[string]bool{}
		}
		if !seen[kind][id] {
			seen[kind][id] = true
			ids[kind] = append(ids[kind], id)
		}
	}
	owners := map[string]map[string]map[string]string{}
	for kind, list := range ids {
		source := sources[kind]
		all := map[string]map[string]string{}
		limit := 1000
		if source.store == "mongo" {
			limit = 500
		}
		if e := rootBatches(list, limit, func(batch []string) error {
			var values map[string]map[string]string
			var e error
			if source.store == "mysql" {
				values, e = p.sqlLinkOwners(ctx, source.name, batch)
			} else {
				values, e = p.mongoLinkOwners(ctx, source.name, batch)
			}
			for id, owner := range values {
				all[id] = owner
			}
			return e
		}); e != nil {
			return e
		}
		owners[kind] = all
	}
	// Run has no direct Testee identity: Run -> Generation -> Outcome is required.
	if len(owners["interpretation_run"]) > 0 {
		generationIDs := []string{}
		for _, run := range owners["interpretation_run"] {
			generationIDs = append(generationIDs, run["generation_id"])
		}
		generations := map[string]map[string]string{}
		if e := rootBatches(generationIDs, 500, func(batch []string) error {
			rows, e := p.mongoLinkOwners(ctx, "report_generations", batch)
			for id, row := range rows {
				generations[id] = row
			}
			return e
		}); e != nil {
			return e
		}
		outcomeIDs := []string{}
		for _, gen := range generations {
			outcomeIDs = append(outcomeIDs, gen["outcome_id"])
		}
		outcomes := map[string]map[string]string{}
		if e := rootBatches(outcomeIDs, 1000, func(batch []string) error {
			rows, e := p.sqlLinkOwners(ctx, "evaluation_outcome", batch)
			for id, row := range rows {
				outcomes[id] = row
			}
			return e
		}); e != nil {
			return e
		}
		for _, run := range owners["interpretation_run"] {
			outcome := outcomes[generations[run["generation_id"]]["outcome_id"]]
			for _, key := range []string{"testee_id", "org_id", "assessment_id"} {
				run[key] = outcome[key]
			}
			run["outcome_id"] = outcome["id"]
		}
	}
	for _, o := range objects {
		kind, id := o.Links["source_type"], o.Links["source_ref"]
		owner, exists := owners[kind][id]
		conflict := !exists || owner["testee_id"] != o.Links["testee_id"] || owner["org_id"] != o.Links["org_id"]
		if !conflict {
			for _, key := range []string{"assessment_id", "outcome_id", "plan_id", "enrollment_id", "task_id"} {
				value := o.Links[key]
				if value != "" && value != "0" && owner[key] != "" && value != owner[key] {
					conflict = true
				}
			}
			expected := map[string]string{"answersheet": "answersheet_id", "assessment": "assessment_id", "evaluation_outcome": "outcome_id", "interpret_report": "report_id", "plan_enrollment": "enrollment_id", "assessment_task": "task_id"}[kind]
			if expected != "" && o.Links[expected] != "" && o.Links[expected] != "0" && o.Links[expected] != id {
				conflict = true
			}
		}
		if conflict {
			p.holdObject(o, o.Object.Name+":typed_statistic_source_ownership_conflict")
		}
		if e := p.writer.add(o, graphSQLTables[o.Object.Name]); e != nil {
			return e
		}
	}
	return nil
}
func graphStatisticTable(name string) bool {
	return strings.HasPrefix(name, "statistics_") && name != "statistics_store_activity_fact"
}
