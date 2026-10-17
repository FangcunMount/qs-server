package main

import (
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestDirectedReferencesKeepNumericAliasesNestedParentsAndWrongDomains(t *testing.T) {
	g := group{IAM: []byte(`{"user_ids":["900"],"profile_ids":[],"login_identity_ids":[]}`), QS: qsScope{Graph: &graphBinding{RootIDs: []string{"44"}}}}
	f := graphFrozenReferences{ids: map[string]map[string]bool{"mysql/assessment": {"101": true, "9007199254740994": true}, "mysql/plan_enrollment": {"501": true}, "mysql/answering_start": {"71": true}, "mongo/interpret_report_artifacts": {"601": true}}}
	cases := []struct {
		label   string
		doc     bson.D
		blocked bool
	}{
		{"int64", bson.D{{Key: "assessment_id", Value: int64(101)}}, true},
		{"int32", bson.D{{Key: "assessment_id", Value: int32(101)}}, true},
		{"double", bson.D{{Key: "assessment_id", Value: float64(101)}}, true},
		{"double_above_2pow53", bson.D{{Key: "assessment_id", Value: float64(9007199254740994)}}, true},
		{"rounded_different_domain_value", bson.D{{Key: "assessment_id", Value: float64(9007199254740993)}}, false},
		{"canonical_string", bson.D{{Key: "assessment_id", Value: "101"}}, true},
		{"plus_zero_string", bson.D{{Key: "assessment_id", Value: "+000101"}}, true},
		{"nested_start", bson.D{{Key: "start_context", Value: bson.D{{Key: "id", Value: "00071"}}}}, true},
		{"nested_enrollment", bson.D{{Key: "attribution", Value: bson.D{{Key: "enrollment_id", Value: int64(501)}}}}, true},
		{"shared_filler", bson.D{{Key: "filler_id", Value: int64(900)}}, true},
		{"wrong_domain", bson.D{{Key: "report_id", Value: int64(101)}}, false},
		{"fractional", bson.D{{Key: "assessment_id", Value: 101.5}}, false},
	}
	for _, v := range cases {
		t.Run(v.label, func(t *testing.T) {
			r := &graphFake{sqlRows: map[string][]record{}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
			r.mongoRows["answersheets"] = []record{graphMongoFixture(t, "answersheets", append(bson.D{{Key: "domain_id", Value: int64(999)}}, v.doc...))}
			err := graphRelationshipReferences(graphFrozenRuleDeleteContext(t.Context(), true), r, g, f, false)
			if (err != nil) != v.blocked {
				t.Fatal("directed callback changed typed reference semantics", err)
			}
			for name, count := range r.mongoRootScans {
				if count != 0 {
					t.Fatal("explicit mode issued an unfiltered Mongo traversal", name)
				}
			}
		})
	}
}

func TestDirectedReferences6000RootsActualNonuniformParentsStayWithinWireBudgets(t *testing.T) {
	roots, ids := map[string]bool{}, map[string]bool{}
	for i := range 6000 {
		roots[fmt.Sprint(600000000000000000+int64(i))] = true
	}
	for i := range 21405 {
		ids[fmt.Sprint(700000000000000000+int64(i))] = true
	}
	f := graphFrozenReferences{ids: map[string]map[string]bool{}}
	for _, scope := range []string{"mysql/assessment", "mysql/evaluation_outcome", "mysql/assessment_task", "mysql/plan_enrollment", "mysql/answering_start", "mongo/answersheets", "mongo/report_generations", "mongo/interpretation_runs", "mongo/interpret_report_artifacts", "mongo/report_query_catalog"} {
		f.ids[scope] = ids
	}
	tasks, enrollments := map[string]bool{}, map[string]bool{}
	for i := range 27376 {
		tasks[fmt.Sprint(800000000000000000+int64(i))] = true
	}
	for i := range 3422 {
		enrollments[fmt.Sprint(900000000000000000+int64(i))] = true
	}
	f.ids["mysql/assessment_task"], f.ids["mysql/plan_enrollment"], f.ids["mysql/answering_start"] = tasks, enrollments, nil
	for _, name := range []string{"answersheets", "interpret_report_artifacts", "report_query_catalog", "report_generations", "interpretation_runs"} {
		filter, err := graphFrozenMongoReferenceFilter(name, roots, roots, f)
		if err != nil {
			t.Fatal(name, err)
		}
		raw, err := bson.Marshal(filter)
		t.Logf("synthetic actual shape Mongo %s BSON bytes=%d", name, len(raw))
		if err != nil || len(raw) > graphMongoCommandBytes || len(filter) == 0 {
			t.Fatal("unbounded directed Mongo query", name, len(raw), err)
		}
	}
	for _, table := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact"} {
		sources := map[string]map[string]bool{"assessment": ids, "answersheet": ids, "evaluation_outcome": ids, "interpret_report": ids, "interpretation_run": ids, "assessment_task": tasks, "plan_enrollment": enrollments, "clinician_relation": ids, "entry_intake": ids}
		where, args, err := graphFrozenStatisticsPredicate(table, roots, roots, sources, f)
		t.Logf("synthetic actual shape SQL %s conservative wire bytes=%d", table, len(encode(args))+len(where)+8192)
		if err != nil || len(encode(args))+len(where)+8192 >= 64<<20 || strings.Count(where, "?") != len(args) {
			t.Fatal("directed SQL packet/parameters changed", table, err)
		}
	}
}

func TestDirectedReferenceClosedCatalogAndTypedDomains(t *testing.T) {
	roots := map[string]bool{"44": true}
	for _, name := range []string{"rm_outbox", "foreign"} {
		if _, err := graphFrozenMongoReferenceFilter(name, roots, nil, graphFrozenReferences{}); err == nil {
			t.Fatal("foreign namespace accepted")
		}
	}
	if _, err := graphFrozenMongoReferenceFilter("answersheets", map[string]bool{"044": true}, nil, graphFrozenReferences{}); err == nil {
		t.Fatal("noncanonical authority expanded")
	}
	if _, _, err := graphFrozenStatisticsPredicate("assessment", roots, nil, nil, graphFrozenReferences{}); err == nil {
		t.Fatal("foreign SQL table accepted")
	}
	f := graphFrozenReferences{ids: map[string]map[string]bool{"mongo/answersheets": {"700": true}, "mysql/assessment": {"101": true}, "mongo/interpret_report_artifacts": {"501": true}}}
	where, args, err := graphFrozenStatisticsPredicate("statistics_assessment_fact", roots, nil, map[string]map[string]bool{"answersheet": {"700": true}}, f)
	if err != nil || !strings.Contains(where, "x.`answersheet_id`") || !strings.Contains(where, "x.`assessment_id`") || !strings.Contains(where, "x.`report_id`") || !strings.Contains(where, "BINARY x.source_type=BINARY ?") || strings.Count(where, "?") != len(args) {
		t.Fatal("typed SQL parent/source condition omitted", where, err)
	}
}
