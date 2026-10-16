package main

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLeanPlanDoesNotDependOnBusinessBodyOrPreimageFingerprint(t *testing.T) {
	reader, c, protection, dir := graphFixture(t)
	body := cell{Name: "payload_json", Type: "LONGTEXT", Raw: []byte(strings.Repeat("original business result", 10000))}
	reader.sqlRows["evaluation_outcome"][0].Columns = append(reader.sqlRows["evaluation_outcome"][0].Columns, body)
	first, e := planCohort(t.Context(), reader, c, protection, dir, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	reader.sqlRows["evaluation_outcome"][0].Columns[len(reader.sqlRows["evaluation_outcome"][0].Columns)-1].Raw = []byte("changed body with identical typed ownership")
	next := privateTestDir(t)
	second, e := planCohort(t.Context(), reader, c, protection, next, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("business values changed identity-only plan")
	}
	for _, shard := range first.Shards {
		a, e := os.ReadFile(filepath.Join(dir, shard.File))
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(filepath.Join(next, shard.File))
		if e != nil {
			t.Fatal(e)
		}
		if string(a) != string(b) || strings.Contains(string(a), "original business result") {
			t.Fatal("body or body fingerprint entered plan")
		}
		page, e := readGraphPage(dir, shard)
		if e != nil {
			t.Fatal(e)
		}
		for _, o := range page.Objects {
			if o.Object.Fingerprint != "" {
				t.Fatal("plan froze a full-value preimage")
			}
		}
	}
}
func TestLeanIdentityGroupsCoverEveryRootAndForbidSplittingSharedIdentities(t *testing.T) {
	_, c, p, _ := graphFixture(t)
	for _, kind := range []string{"missing_links", "missing_root", "duplicate_user", "duplicate_profile", "foreign_root"} {
		t.Run(kind, func(t *testing.T) {
			changed := c
			changed.IdentityGroups = append([]cohortIdentityGroup{}, c.IdentityGroups...)
			switch kind {
			case "missing_links":
				changed.IdentityLinksSHA = ""
			case "missing_root":
				changed.TesteeIDs = []string{"1", "2"}
			case "duplicate_user":
				changed.TesteeIDs = []string{"1", "2"}
				changed.IdentityGroups = append(changed.IdentityGroups, cohortIdentityGroup{UserIDs: []string{"10"}, ProfileIDs: []string{"12"}, TesteeIDs: []string{"2"}})
			case "duplicate_profile":
				changed.TesteeIDs = []string{"1", "2"}
				changed.IdentityGroups = append(changed.IdentityGroups, cohortIdentityGroup{UserIDs: []string{"20"}, ProfileIDs: []string{"11"}, TesteeIDs: []string{"2"}})
			case "foreign_root":
				changed.IdentityGroups[0].TesteeIDs = []string{"2"}
			}
			if validateSelector(changed, p) == nil {
				t.Fatal("unbound or split identity component accepted")
			}
		})
	}
}
func TestLeanConflictHoldsEntireUserComponentAndPreservesUnrelatedGroup(t *testing.T) {
	reader, c, p, dir := graphFixture(t)
	c.TesteeIDs = []string{"1", "2", "3"}
	c.IdentityGroups = []cohortIdentityGroup{{UserIDs: []string{"10"}, ProfileIDs: []string{"11", "12"}, TesteeIDs: []string{"1", "2"}}, {UserIDs: []string{"30"}, ProfileIDs: []string{"13"}, TesteeIDs: []string{"3"}}}
	reader.sqlRows["testee"] = append(reader.sqlRows["testee"], graphSQLFixture("testee", "2", map[string]string{"org_id": "1", "source": "daily_simulation", "profile_id": "999"}), graphSQLFixture("testee", "3", map[string]string{"org_id": "1", "source": "daily_simulation", "profile_id": "13"}))
	index, e := planCohort(t.Context(), reader, c, p, dir, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	if validateGraphHolds(dir, index, map[string]bool{"1": true}) == nil || validateGraphHolds(dir, index, map[string]bool{"2": true}) == nil {
		t.Fatal("profile conflict did not hold complete User component")
	}
	if e = validateGraphHolds(dir, index, map[string]bool{"3": true}); e != nil {
		t.Fatal("unrelated User component blocked", e)
	}
}
func TestLeanSettledResponsibilitiesAreCountedAndNeverArchived(t *testing.T) {
	reader, c, p, dir := graphFixture(t)
	dd := graphSQLFixture("event_delivery_dead_letter", "90", map[string]string{"org_id": "1", "testee_id": "1"})
	dd.Columns = append(dd.Columns, cell{Name: "retry_disposition", Type: "VARCHAR", Raw: []byte("archived_mock")}, cell{Name: "payload_json", Type: "LONGTEXT", Raw: []byte("large retained original wire")})
	reader.sqlRows["event_delivery_dead_letter"] = []record{dd}
	index, e := planCohort(t.Context(), reader, c, p, dir, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	if index.Retained["event_delivery_dead_letter"] == "" {
		t.Fatal("retained audit responsibility missing")
	}
	for _, s := range index.Shards {
		if s.Name == "event_delivery_dead_letter" || s.Name == "rm_outbox" || s.Name == "task_opened_reminder_delivery" {
			t.Fatal("settled retained wire entered per-object scope archive")
		}
	}
}

func TestLeanCatalogForeignOwnerCannotHideFrozenGeneration(t *testing.T) {
	reader, c, p, dir := graphFixture(t)
	foreign := graphMongoFixture(t, "report_query_catalog", bson.D{{Key: "testee_id", Value: int64(999)}, {Key: "org_id", Value: int64(1)}, {Key: "assessment_id", Value: int64(999)}, {Key: "generation_id", Value: int64(800)}, {Key: "outcome_id", Value: int64(200)}})
	reader.mongoRows["report_query_catalog"] = append(reader.mongoRows["report_query_catalog"], foreign)
	index, e := planCohort(t.Context(), reader, c, p, dir, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	if index.Holds["report_query_catalog:retained_object_links_target_parent"] != 1 || validateGraphHolds(dir, index, map[string]bool{"1": true}) == nil {
		t.Fatal("foreign catalog owner hid a frozen generation/outcome reference")
	}
	if index.Counts["mongo/report_query_catalog"] != 1 {
		t.Fatal("foreign shared object was promoted into a target")
	}
}
func TestLeanProtectedIAMAndNewAIIntersectionHoldOnlyTheirComponent(t *testing.T) {
	for _, kind := range []string{"protected_user", "new_ai_request"} {
		t.Run(kind, func(t *testing.T) {
			reader, c, p, dir := graphFixture(t)
			if kind == "protected_user" {
				c.IdentityGroups[0].UserIDs = []string{"10001"}
			} else {
				reader.sqlRows["ai_bridge_requests"] = []record{graphSQLFixture("ai_bridge_requests", "90", map[string]string{"org_id": "1", "testee_id": "1"})}
			}
			index, e := planCohort(t.Context(), reader, c, p, dir, digest(encode(c)), strings.Repeat("c", 40))
			if e != nil {
				t.Fatal(e)
			}
			if validateGraphHolds(dir, index, map[string]bool{"1": true}) == nil {
				t.Fatal("protected/shared identity acquired cleanup authority")
			}
			if e = validateGraphHolds(dir, index, map[string]bool{"99": true}); e != nil {
				t.Fatal("unrelated identity component was blocked", e)
			}
		})
	}
}

func TestLeanExecutionResponsibilitiesHoldWhileMockSchedulingCanRetire(t *testing.T) {
	for _, kind := range []string{"runtime_claim", "runtime_retry", "run_lease", "run_pending", "generation_inflight", "task_pending"} {
		t.Run(kind, func(t *testing.T) {
			reader, c, p, dir := graphFixture(t)
			switch kind {
			case "runtime_claim", "runtime_retry":
				r := graphSQLFixture("runtime_checkpoint", "91", map[string]string{"assessment_id": "100", "status": "succeeded"})
				r.Columns = append(r.Columns, cell{Name: "scope", Type: "VARCHAR", Raw: []byte("evaluation_run")})
				if kind == "runtime_claim" {
					r.Columns = append(r.Columns, cell{Name: "claim_token", Type: "VARCHAR", Raw: []byte("actual-live-claim")})
				} else {
					r.Columns = append(r.Columns, cell{Name: "retry_disposition", Type: "VARCHAR", Raw: []byte("automatic")})
				}
				reader.sqlRows["runtime_checkpoint"] = []record{r}
			case "task_pending":
				for i := range reader.sqlRows["assessment_task"][0].Columns {
					if reader.sqlRows["assessment_task"][0].Columns[i].Name == "status" {
						reader.sqlRows["assessment_task"][0].Columns[i].Raw = []byte("pending")
					}
				}
			default:
				name := "interpretation_runs"
				if kind == "generation_inflight" {
					name = "report_generations"
				}
				var doc bson.D
				if e := bson.Unmarshal(reader.mongoRows[name][0].BSON, &doc); e != nil {
					t.Fatal(e)
				}
				if kind == "run_lease" {
					doc = append(doc, bson.E{Key: "lease_expires_at", Value: primitive.NewDateTimeFromTime(time.Now().Add(time.Minute))})
				} else {
					for i := range doc {
						if doc[i].Key == "status" {
							doc[i].Value = "pending"
						}
					}
				}
				raw, e := bson.Marshal(doc)
				if e != nil {
					t.Fatal(e)
				}
				reader.mongoRows[name][0].BSON = raw
			}
			index, e := planCohort(t.Context(), reader, c, p, dir, digest(encode(c)), strings.Repeat("c", 40))
			if e != nil {
				t.Fatal(e)
			}
			held := validateGraphHolds(dir, index, map[string]bool{"1": true}) != nil
			if kind == "task_pending" {
				if held || index.RequiredActions["settle_task_pending"] != 1 {
					t.Fatal("internal mock scheduling was incorrectly treated as external unfinished work", index.Holds)
				}
			} else if !held {
				t.Fatal("active execution/retry responsibility did not hold its User component", kind)
			}
		})
	}
}

func TestLeanStatisticsExplicitParentIntersectionsHoldWithoutExpandingTargets(t *testing.T) {
	for _, spec := range []struct{ table, column, target, otherDomain string }{
		{"statistics_assessment_fact", "assessment_id", "100", "200"},
		{"statistics_assessment_fact", "answersheet_id", "20", "100"},
		{"statistics_assessment_fact", "outcome_id", "200", "700"},
		{"statistics_assessment_fact", "report_id", "700", "400"},
		{"statistics_plan_fact", "enrollment_id", "600", "400"},
		{"statistics_plan_fact", "task_id", "400", "600"},
	} {
		for _, matches := range []bool{true, false} {
			label := spec.column + "/other_domain_same_number"
			if matches {
				label = spec.column + "/frozen_parent"
			}
			t.Run(label, func(t *testing.T) {
				reader, c, protection, dir := graphFixture(t)
				value := spec.otherDomain
				if matches {
					value = spec.target
				}
				reader.sqlRows[spec.table] = []record{graphSQLFixture(spec.table, "910", map[string]string{"org_id": "1", "testee_id": "999", "source_type": "assessment", "source_ref": "999999", spec.column: value})}
				index, e := planCohort(t.Context(), reader, c, protection, dir, digest(encode(c)), strings.Repeat("c", 40))
				if e != nil {
					t.Fatal(e)
				}
				held := validateGraphHolds(dir, index, map[string]bool{"1": true}) != nil
				if held != matches {
					t.Fatal("typed explicit parent ownership escaped or crossed a different ID domain", index.Holds)
				}
				if index.Counts["mysql/"+spec.table] != 0 {
					t.Fatal("foreign/mixed statistic gained physical deletion authority")
				}
				if matches && index.Counts["retained/foreign_or_mixed/mysql/"+spec.table] != 1 {
					t.Fatal("foreign physical reference was not counted once", index.Counts)
				}
			})
		}
	}
}

func TestLeanStatisticsSourceRowsDeduplicateAndCheckAllExplicitParents(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "foreign_outcome"}[mismatch], func(t *testing.T) {
			reader, c, protection, dir := graphFixture(t)
			outcome := "200"
			if mismatch {
				outcome = "999999"
			}
			reader.sqlRows["statistics_assessment_fact"] = []record{graphSQLFixture("statistics_assessment_fact", "911", map[string]string{"org_id": "1", "testee_id": "1", "source_type": "assessment", "source_ref": "100", "assessment_id": "100", "answersheet_id": "21", "outcome_id": outcome, "report_id": "700", "enrollment_id": "600", "task_id": "400"})}
			index, e := planCohort(t.Context(), reader, c, protection, dir, digest(encode(c)), strings.Repeat("c", 40))
			if e != nil {
				t.Fatal(e)
			}
			if index.Counts["mysql/statistics_assessment_fact"] != 1 {
				t.Fatal("source and explicit-parent discovery duplicated a physical statistic", index.Counts)
			}
			if held := validateGraphHolds(dir, index, map[string]bool{"1": true}) != nil; held != mismatch {
				t.Fatal("source ownership bypassed conflicting explicit parent", index.Holds)
			}
			seen := map[string]bool{}
			for _, shard := range index.Shards {
				page, e := readGraphPage(dir, shard)
				if e != nil {
					t.Fatal(e)
				}
				for _, o := range page.Objects {
					id := objectID(o.Object)
					if seen[id] {
						t.Fatal("duplicate physical scope key")
					}
					seen[id] = true
				}
			}
		})
	}
}

func TestLeanStatisticsCrossComponentParentHoldsBothCompleteGroups(t *testing.T) {
	reader, c, protection, dir := graphFixture(t)
	c.TesteeIDs = []string{"1", "2", "3"}
	c.IdentityGroups = []cohortIdentityGroup{{UserIDs: []string{"10"}, ProfileIDs: []string{"11", "12"}, TesteeIDs: []string{"1", "3"}}, {UserIDs: []string{"20"}, ProfileIDs: []string{"22"}, TesteeIDs: []string{"2"}}}
	reader.sqlRows["testee"] = append(reader.sqlRows["testee"], graphSQLFixture("testee", "2", map[string]string{"org_id": "1", "profile_id": "22", "source": "daily_simulation"}), graphSQLFixture("testee", "3", map[string]string{"org_id": "1", "profile_id": "12", "source": "daily_simulation"}))
	reader.sqlRows["statistics_assessment_fact"] = []record{graphSQLFixture("statistics_assessment_fact", "912", map[string]string{"org_id": "1", "testee_id": "1", "source_type": "assessment", "source_ref": "100", "assessment_id": "101"})}
	index, e := planCohort(t.Context(), reader, c, protection, dir, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	for _, root := range c.TesteeIDs {
		if validateGraphHolds(dir, index, map[string]bool{root: true}) == nil {
			t.Fatal("mixed statistic did not hold both complete User components", root, index.Holds)
		}
	}
	if index.Counts["mysql/statistics_assessment_fact"] != 1 {
		t.Fatal("mixed statistic duplicated its scope")
	}
}
