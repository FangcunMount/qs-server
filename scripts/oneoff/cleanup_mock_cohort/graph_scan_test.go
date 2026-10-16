package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

type graphScanReader struct {
	*graphFake
	roots map[string]bool
}

func (r *graphScanReader) sqlPages(ctx context.Context, name, where string, args []any, emit func(record) error) error {
	if name != "testee" {
		return r.graphFake.sqlPages(ctx, name, where, args, emit)
	}
	r.calls = append(r.calls, "mysql/"+name+":"+where)
	for _, arg := range args[:len(args)-4] {
		id, ok := arg.(string)
		if ok && r.roots[id] {
			if e := emit(graphSQLFixture("testee", id, map[string]string{"org_id": "1", "source": "daily_simulation"})); e != nil {
				return e
			}
		}
	}
	return nil
}

func TestGraphUnindexedSQLScanVisitsEachTableOnceAndKeepsExactRoots(t *testing.T) {
	fake, c, protection, dir := graphFixture(t)
	fake.sqlRows = map[string][]record{}
	fake.mongoRows = nil
	c.TesteeIDs = []string{}
	reader := &graphScanReader{graphFake: fake, roots: map[string]bool{}}
	for n := 1; n <= 2001; n++ {
		id := strconv.Itoa(n)
		c.TesteeIDs = append(c.TesteeIDs, id)
		reader.roots[id] = true
	}
	// The shared fake protection uses roots 42/43; retain a different pair here.
	protection.QS.System = []object{key("testee", "90001")}
	protection.QS.Clack = []object{key("testee", "90002")}
	c.ProtectionSHA = digest(encode(protection))
	for _, name := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact", "interpretation_attention_projection"} {
		fake.sqlRows[name] = []record{
			graphSQLFixture(name, "11", map[string]string{"org_id": "1", "testee_id": "1"}),
			graphSQLFixture(name, "12", map[string]string{"org_id": "1", "testee_id": "2001"}),
			graphSQLFixture(name, "13", map[string]string{"org_id": "1", "testee_id": "999999"}),
		}
	}
	index, err := planCohort(t.Context(), reader, c, protection, dir, digest(encode(c)), strings.Repeat("c", 40))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact", "interpretation_attention_projection"} {
		calls := 0
		for _, call := range reader.calls {
			if strings.HasPrefix(call, "mysql/"+name+":") {
				calls++
				if strings.Contains(call, "testee_id IN") {
					t.Fatal("unindexed table still repeats a root-IN candidate scan")
				}
			}
		}
		if calls != 1 || index.Counts["mysql/"+name] != 2 {
			t.Fatalf("%s scans=%d selected=%d, want one scan and two exact roots", name, calls, index.Counts["mysql/"+name])
		}
	}
}

func TestGraphSingleSQLScanPageFailureStopsAndUnownedNullRemainsOutsideScope(t *testing.T) {
	for _, failure := range []bool{true, false} {
		fake, c, _, dir := graphFixture(t)
		if failure {
			fake.fail = "statistics_access_fact"
		} else {
			fake.sqlRows["statistics_access_fact"] = []record{graphSQLFixture("statistics_access_fact", "1", map[string]string{"org_id": "1", "testee_id": "01"}), graphSQLFixture("statistics_access_fact", "2", map[string]string{"org_id": "1"})}
		}
		index := graphIndex{Counts: map[string]int64{}, Holds: map[string]int64{}, RequiredActions: map[string]int64{}}
		planner := cohortPlanner{context: t.Context(), reader: fake, selector: c, writer: &graphWriter{dir: dir, index: &index, protected: map[string]bool{}}, dir: dir}
		err := planner.rootSQL("statistics_access_fact", c.TesteeIDs)
		if failure && err == nil || !failure && err != nil || index.Counts["mysql/statistics_access_fact"] != 0 {
			t.Fatal(errors.New("page failures must stop; nonmember/nullable same-org facts must remain outside scope"))
		}
	}
}
