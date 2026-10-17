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
	if name != "testee" || !strings.Contains(where, "created_at") {
		return r.graphFake.sqlPages(ctx, name, where, args, emit)
	}
	r.calls = append(r.calls, "mysql/"+name+":"+where)
	for _, arg := range args[:len(args)-4] {
		id, ok := arg.(string)
		if ok && r.roots[id] {
			n, _ := strconv.Atoi(id)
			if e := emit(graphSQLFixture("testee", id, map[string]string{"org_id": "1", "profile_id": strconv.Itoa(n + 20000), "source": "daily_simulation"})); e != nil {
				return e
			}
		}
	}
	return nil
}

func TestGraphStatisticsUseTypedSourceIndexWithoutOrgScan(t *testing.T) {
	fake, c, protection, dir := graphFixture(t)
	fake.sqlRows = map[string][]record{}
	fake.mongoRows = nil
	c.TesteeIDs = []string{}
	c.IdentityGroups = []cohortIdentityGroup{}
	reader := &graphScanReader{graphFake: fake, roots: map[string]bool{}}
	for n := 1; n <= 2001; n++ {
		id := strconv.Itoa(n)
		c.TesteeIDs = append(c.TesteeIDs, id)
		reader.roots[id] = true
		c.IdentityGroups = append(c.IdentityGroups, cohortIdentityGroup{UserIDs: []string{strconv.Itoa(n + 10000)}, ProfileIDs: []string{strconv.Itoa(n + 20000)}, TesteeIDs: []string{id}})
	}
	// All synthetic roots have their own real projected Profile identity.
	protection.QS.System = []object{key("testee", "90001")}
	protection.QS.Clack = []object{key("testee", "90002")}
	c.ProtectionSHA = digest(encode(protection))
	fake.sqlRows["clinician_relation"] = []record{graphSQLFixture("clinician_relation", "11", map[string]string{"org_id": "1", "testee_id": "1"}), graphSQLFixture("clinician_relation", "12", map[string]string{"org_id": "1", "testee_id": "2001"})}
	fake.sqlRows["statistics_access_fact"] = []record{graphSQLFixture("statistics_access_fact", "21", map[string]string{"org_id": "1", "testee_id": "1", "source_type": "clinician_relation", "source_ref": "11"}), graphSQLFixture("statistics_access_fact", "22", map[string]string{"org_id": "1", "testee_id": "2001", "source_type": "clinician_relation", "source_ref": "12"}), graphSQLFixture("statistics_access_fact", "23", map[string]string{"org_id": "1", "source_type": "clinician_relation", "source_ref": "999999"})}
	index, e := planCohort(t.Context(), reader, c, protection, dir, digest(encode(c)), strings.Repeat("c", 40))
	if e != nil {
		t.Fatal(e)
	}
	if index.Counts["mysql/statistics_access_fact"] != 2 {
		t.Fatal("typed source lookup lost exact facts", index.Counts)
	}
	indexed, directed := 0, map[string]int{}
	for _, call := range reader.calls {
		if !strings.HasPrefix(call, "mysql/statistics_") {
			continue
		}
		if strings.Contains(call, "x.source_type=? AND x.source_ref IN (") {
			indexed++
			continue
		}
		if !strings.Contains(call, " IN (SELECT fixed_ids.id FROM JSON_TABLE(") || strings.Contains(call, "EXISTS (") || strings.Contains(call, "x.testee_id IN (") {
			t.Fatal("statistics restarted an org/Testee batch scan", call)
		}
		namespace := strings.SplitN(call, ":", 2)[0]
		directed[namespace]++
	}
	if indexed != 1 || len(directed) != 3 {
		t.Fatal("indexed source lookup plus one directed query per namespace required", indexed, directed)
	}
	for _, count := range directed {
		if count != 1 {
			t.Fatal("explicit parent scan repeated with a root batch", directed)
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
