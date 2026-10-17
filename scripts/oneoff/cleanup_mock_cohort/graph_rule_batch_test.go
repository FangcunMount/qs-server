package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// Model the fixed emitted query's scalar $in and string $regex semantics. The
// old graphFake intentionally supplies an OR superset and cannot test filtering.
type ruleBatchFilteringReader struct{ *graphFake }

func (r ruleBatchFilteringReader) mongoPages(ctx context.Context, name string, filter bson.D, emit func(record) error) error {
	if name != "answersheets" || len(filter) == 0 || filter[0].Key != "$or" {
		return r.graphFake.mongoPages(ctx, name, filter, emit)
	}
	r.calls = append(r.calls, "mongo/"+name)
	for _, row := range r.mongoRows[name] {
		matched := false
		for _, raw := range filter[0].Value.(bson.A) {
			clause := raw.(bson.D)
			value := bson.Raw(row.BSON).Lookup(clause[0].Key)
			condition := clause[0].Value.(bson.D)[0]
			if condition.Key == "$regex" && value.Type == bson.TypeString {
				matched = matched || regexp.MustCompile(condition.Value.(string)).MatchString(value.StringValue())
			}
			if condition.Key == "$in" {
				for _, id := range condition.Value.(bson.A) {
					n := id.(int64)
					matched = matched || value.Type == bson.TypeInt64 && value.Int64() == n || value.Type == bson.TypeInt32 && int64(value.Int32()) == n || value.Type == bson.TypeDouble && value.Double() == float64(n)
				}
			}
		}
		if matched {
			if e := emit(row); e != nil {
				return e
			}
		}
	}
	return nil
}

func TestRuleBatchPlanPreservesOriginalTargetsIndependentSheetsAndHolds(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		reader, selector, protection, firstDir := graphFixture(t)
		independent := graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(9991)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(1)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "independent_questionnaire"}}}})
		reader.mongoRows["answersheets"] = append(reader.mongoRows["answersheets"], independent)
		if conflict {
			reader.mongoRows["answersheets"] = append(reader.mongoRows["answersheets"], graphMongoFixture(t, "answersheets", bson.D{{Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: "+0001"}}))
		}
		first, e := planCohort(t.Context(), reader, selector, protection, firstDir, digest(encode(selector)), strings.Repeat("c", 40))
		if e != nil {
			t.Fatal(e)
		}
		secondDir := privateTestDir(t)
		second, e := planCohort(graphRuleBatchContext(t.Context(), true), ruleBatchFilteringReader{reader}, selector, protection, secondDir, digest(encode(selector)), strings.Repeat("c", 40))
		if e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(first, second) || first.Counts["mongo/answersheets"] != 3 {
			t.Fatal("explicit rule batch changed original target closure or independent AnswerSheet")
		}
		if conflict && len(second.Holds) == 0 {
			t.Fatal("malformed Testee alias lost its component hold")
		}
		for _, ref := range first.Shards {
			a, e := os.ReadFile(filepath.Join(firstDir, ref.File))
			if e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(filepath.Join(secondDir, ref.File))
			if e != nil || !bytes.Equal(a, b) {
				t.Fatal("rule batch changed original typed shard bytes", e)
			}
		}
		if reader.mongoRootScans["answersheets"] != 1 {
			t.Fatal("explicit rule batch still requested an unfiltered AnswerSheet root cursor")
		}
	}
}

func TestRuleBatchAnswerSheetFilterRetainsAliasesAndAssessmentLinkedForeignOwner(t *testing.T) {
	filter, e := graphRuleBatchAnswersheets(graphRuleBatchContext(t.Context(), true), []string{"123"}, map[string]string{"456": "123"})
	if e != nil {
		t.Fatal(e)
	}
	clauses := filter[0].Value.(bson.A)
	seen := map[string]string{}
	for _, raw := range clauses {
		clause := raw.(bson.D)
		condition := clause[0].Value.(bson.D)
		if condition[0].Key == "$regex" {
			seen[clause[0].Key] = condition[0].Value.(string)
		}
	}
	for field, values := range map[string][]string{"testee_id": {"123", "+123", "000123", "+000123"}, "domain_id": {"456", "000456"}} {
		pattern := regexp.MustCompile(seen[field])
		for _, value := range values {
			if !pattern.MatchString(value) {
				t.Fatal("original decimal alias or frozen assessment AnswerSheet omitted")
			}
		}
		for _, foreign := range []string{"-123", "123x", "1123", " 123"} {
			if pattern.MatchString(foreign) {
				t.Fatal("foreign identity expanded fixed candidate query")
			}
		}
	}
}

func TestRuleBatchActualReceiptsRemainPartialUntilGlobalClearance(t *testing.T) {
	for _, action := range []string{"apply", "verify", "capture"} {
		for _, fast := range []bool{false, true} {
			var out bytes.Buffer
			c := config{action: action, ruleBatch: fast, groupSHA: strings.Repeat("a", 64)}
			if e := graphEmit(&out, c, group{Operation: "fixed-rule-subset"}, strings.Repeat("b", 64), false, true); e != nil {
				t.Fatal(e)
			}
			var r actionReceipt
			if e := json.Unmarshal(out.Bytes(), &r); e != nil {
				t.Fatal(e)
			}
			partial := fast && action != "capture"
			if !r.Complete || !r.Released || r.Pending != partial || r.Whole == partial {
				t.Fatal("batch receipt overstated global clearance or changed default evidence")
			}
		}
	}
}

func TestRuleBatchFlagsDoNotEnterRestoreOrPrimitivePlan(t *testing.T) {
	for _, args := range [][]string{{"--rule-batch", "--action=restore"}, {"--rule-batch", "--action=plan"}, {"--rule-batch", "--action=verify", "--expect-original"}} {
		if _, e := flags(args); e == nil {
			t.Fatal("batch option entered incompatible authority path")
		}
	}
	for _, action := range []string{"plan-cohort", "capture", "apply", "verify"} {
		if _, e := flags([]string{"--rule-batch", "--action=" + action}); e != nil {
			t.Fatal(e)
		}
	}
}

func TestRuleBatchReuseRejectsEarlierUnreleasedDifferentOrExpiredObservation(t *testing.T) {
	_, selector, _, _ := graphFixture(t)
	now := time.Now().UTC()
	window := graphBinding{RootIDs: []string{"1"}, WindowStarted: now.Add(-time.Minute), WindowExpires: now.Add(time.Minute)}
	good := graphIndex{Source: toolSource, ReadOnly: true, DiscoveryComplete: true, Released: true, RuleBatch: &graphRuleBatchObservation{Started: now.Add(-30 * time.Second), Released: now.Add(-10 * time.Second)}}
	for _, change := range []string{"none", "missing", "early", "inverted", "future", "expired", "source", "unreleased", "partial_roots"} {
		t.Run(change, func(t *testing.T) {
			index, g, selected := good, group{QS: qsScope{Graph: &window}}, selector
			copyObservation := *good.RuleBatch
			index.RuleBatch = &copyObservation
			switch change {
			case "missing":
				index.RuleBatch = nil
			case "early":
				index.RuleBatch.Started = window.WindowStarted.Add(-time.Second)
			case "inverted":
				index.RuleBatch.Released = index.RuleBatch.Started.Add(-time.Second)
			case "future":
				index.RuleBatch.Released = now.Add(time.Second)
			case "expired":
				index.RuleBatch.Released = window.WindowExpires
			case "source":
				index.Source = "foreign-source"
			case "unreleased":
				index.Released = false
			case "partial_roots":
				selected.TesteeIDs = []string{"1", "2"}
			}
			e := graphRuleBatchReuse(index, selected, g, now)
			if (e == nil) != (change == "none") {
				t.Fatal("same-window released exact-selection authority was misclassified", e)
			}
		})
	}
}

func TestRuleBatchReuseKeepsHeldSiblingOutAndAdmitsWholeEligibleComponent(t *testing.T) {
	reader, selector, protection, scope := graphFixture(t)
	selector.TesteeIDs = []string{"1", "2"}
	selector.IdentityGroups = append(selector.IdentityGroups, cohortIdentityGroup{UserIDs: []string{"20"}, ProfileIDs: []string{"22"}, TesteeIDs: []string{"2"}})
	reader.sqlRows["testee"] = append(reader.sqlRows["testee"], graphSQLFixture("testee", "2", map[string]string{"org_id": "1", "source": "daily_simulation", "profile_id": "999"}))
	index, e := planCohort(graphRuleBatchContext(t.Context(), true), ruleBatchFilteringReader{reader}, selector, protection, scope, digest(encode(selector)), toolSource)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	index.ReadOnly, index.DiscoveryComplete, index.Released = true, true, true
	index.RuleBatch = &graphRuleBatchObservation{Started: now.Add(-time.Second), Released: now}
	window := graphBinding{RootIDs: []string{"1"}, WindowStarted: now.Add(-time.Minute), WindowExpires: now.Add(time.Minute)}
	g := group{QS: qsScope{Graph: &window}}
	if e = validateGraphHolds(scope, index, graphRootSet(window.RootIDs)); e != nil {
		t.Fatal("eligible sibling was blocked", e)
	}
	if e = graphRuleBatchReuse(index, selector, g, now); e != nil {
		t.Fatal("complete eligible subset cannot reuse original observation", e)
	}
	window.RootIDs = []string{"2"}
	if e = validateGraphHolds(scope, index, graphRootSet(window.RootIDs)); e == nil {
		t.Fatal("held sibling became eligible for capture")
	}
	selector.IdentityGroups = []cohortIdentityGroup{{UserIDs: []string{"10"}, ProfileIDs: []string{"11", "22"}, TesteeIDs: []string{"1", "2"}}}
	window.RootIDs = []string{"1"}
	if e = graphRuleBatchReuse(index, selector, g, now); e == nil {
		t.Fatal("subset split a complete identity component")
	}
}
