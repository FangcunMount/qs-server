package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Exercise the real planner and scope writer. The same deterministic fixture is
// used before and after the storage change; it makes no connectivity claims.
func BenchmarkLeanPlannerMemory(b *testing.B) {
	p := protected{Format: 1, Kind: "mock_cohort_protected", IAM: []byte(`{"user_ids":["10001"],"profile_ids":[],"login_identity_ids":[]}`), QS: protectionScope{System: []object{key("testee", "42")}, Clack: []object{key("testee", "43")}, Other: []object{}}}
	c := cohortSelector{Format: 2, Kind: "mock_cohort_selector", Operation: "memory-benchmark", OwnerEvidenceSHA: strings.Repeat("a", 64), ProtectionSHA: digest(encode(p)), Binding: binding{SQLUUID: "fixture", SQLDatabase: "qs", MongoIdentity: strings.Repeat("b", 64), MongoDatabase: "qs", SQLVersion: 101, MongoVersion: 39}, Selection: selection{Source: "daily_simulation", OrgID: "1", From: "2025-01-01T00:00:00+08:00", Through: "2026-10-01T00:00:00+08:00", Evidence: "deterministic memory fixture"}, IdentityLinksSHA: strings.Repeat("d", 64), IdentityGroups: []cohortIdentityGroup{{UserIDs: []string{"10"}, ProfileIDs: []string{"11"}, TesteeIDs: []string{"1"}}}, TesteeIDs: []string{"1"}, RetainPlanIDs: []string{}}
	s := &graphFake{sqlRows: map[string][]record{}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
	s.sqlRows["testee"] = []record{graphSQLFixture("testee", "1", map[string]string{"org_id": "1", "profile_id": "11", "source": "daily_simulation"})}
	s.sqlRows["assessment"] = []record{graphSQLFixture("assessment", "100", map[string]string{"org_id": "1", "testee_id": "1"})}
	for i := 0; i < 1024; i++ {
		r := graphSQLFixture("runtime_checkpoint", fmt.Sprint(i+2000), map[string]string{"assessment_id": "100", "status": "succeeded"})
		r.Columns = append(r.Columns, cell{Name: "scope", Type: "VARCHAR", Raw: []byte("evaluation_run")}, cell{Name: "resource_id", Type: "VARCHAR", Raw: []byte(fmt.Sprintf("100:%d", i))})
		s.sqlRows["runtime_checkpoint"] = append(s.sqlRows["runtime_checkpoint"], r)
	}
	for i := 0; i < 2048; i++ {
		id := primitive.ObjectID{}
		binary.BigEndian.PutUint64(id[4:], uint64(i+1))
		raw, e := bson.Marshal(bson.D{{Key: "_id", Value: id}, {Key: "domain_id", Value: int64(i + 10000)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(1)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "independent_questionnaire"}}}})
		if e != nil {
			b.Fatal(e)
		}
		r, e := graphMongoObject("answersheets", raw, []byte("fixed namespace/UUID/indexes"))
		if e != nil {
			b.Fatal(e)
		}
		s.mongoRows["answersheets"] = append(s.mongoRows["answersheets"], r)
	}
	home, e := os.UserHomeDir()
	if e != nil {
		b.Fatal(e)
	}
	dir, e := os.MkdirTemp(home, ".qs-lean-memory-")
	if e != nil {
		b.Fatal(e)
	}
	b.Cleanup(func() { _ = os.RemoveAll(dir) })
	selectorSHA := digest(encode(c))
	var first string
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		output := filepath.Join(dir, fmt.Sprint(i))
		if e = os.Mkdir(output, 0700); e != nil {
			b.Fatal(e)
		}
		index, err := planCohort(context.Background(), s, c, p, output, selectorSHA, strings.Repeat("c", 40))
		if err != nil {
			b.Fatal(err)
		}
		if len(index.Holds) != 0 || index.Counts["mongo/answersheets"] != 2048 || index.Counts["mysql/runtime_checkpoint"] != 1024 {
			b.Fatal("memory fixture changed its scope or guard result")
		}
		sha := digest(encode(index))
		if first != "" && sha != first {
			b.Fatal("deterministic scope changed between iterations")
		}
		first = sha
		s.calls = nil
	}
	b.StopTimer()
	b.Logf("deterministic_index_sha256=%s", first)
}

func leanMemoryPlanner(t *testing.T, reader graphReader, c cohortSelector, dir string) *leanPlanner {
	t.Helper()
	index := &graphIndex{Counts: map[string]int64{}, Holds: map[string]int64{}, Retained: map[string]string{}, RequiredActions: map[string]int64{}}
	p := &leanPlanner{cohortPlanner: &cohortPlanner{context: t.Context(), reader: reader, selector: c, dir: dir, writer: &graphWriter{dir: dir, index: index, protected: map[string]bool{}}}, groups: map[string]cohortIdentityGroup{}, owners: map[string]map[string]leanOwner{}, parents: map[string]map[string]string{}, seenMongo: map[string]string{}}
	for _, g := range c.IdentityGroups {
		for _, root := range g.TesteeIDs {
			p.groups[root] = g
		}
	}
	for _, name := range []string{"enrollment_plan", "sheet_root", "generation_outcome", "run_generation", "event_root"} {
		p.parents[name] = map[string]string{}
	}
	return p
}

func TestLeanMemoryOwnerCollisionPreservesLastOwnerAndWholeComponentHold(t *testing.T) {
	reader, c, _, dir := graphFixture(t)
	c.IdentityGroups = []cohortIdentityGroup{{TesteeIDs: []string{"1", "3"}}, {TesteeIDs: []string{"2"}}}
	p := leanMemoryPlanner(t, reader, c, dir)
	o := graphObject{Object: key("assessment", "100"), Links: map[string]string{"id": "100"}}
	p.remember(o, "1", objectID(o.Object))
	p.remember(o, "2", objectID(o.Object))
	if len(p.held) != 0 || p.owner("assessment", "100") != "2" {
		t.Fatal("identical physical identity changed its established overwrite behavior")
	}
	o.Object = key("assessment", "101")
	p.remember(o, "1", objectID(o.Object))
	if p.owner("assessment", "100") != "1" || len(p.held) != 1 || !reflect.DeepEqual(p.held[0].RootIDs, []string{"1", "2", "3"}) || p.held[0].PhysicalSHA != objectID(o.Object) {
		t.Fatal("distinct physical duplicate lost its prior owner or complete component hold")
	}
	o.Links["id"] = "20"
	p.remember(o, "2", objectID(o.Object))
	if !reflect.DeepEqual(p.ids("assessment"), []string{"100", "20"}) || p.owner("assessment", "missing") != "" {
		t.Fatal("typed owner enumeration or missing identity behavior changed")
	}
	for _, tc := range []struct{ store, name, field, id string }{{"mongo", "answersheets", "domain_id", "30"}, {"mysql", "runtime_checkpoint", "resource_id", "100:attempt-2"}} {
		item := graphObject{Object: key(tc.name, "900"), Links: map[string]string{"id": "900", tc.field: tc.id}}
		item.Object.Store = tc.store
		p.remember(item, "1", objectID(item.Object))
		if p.owner(tc.name, tc.id) != "1" || p.owner(tc.name, "900") != "" {
			t.Fatal("Mongo domain ID or opaque runtime resource acquired a different ID domain")
		}
	}
}

func TestLeanMemoryMongoDedupCoversRejectedAndCatalogCandidates(t *testing.T) {
	reader, c, _, dir := graphFixture(t)
	c.IdentityGroups = append(c.IdentityGroups, cohortIdentityGroup{TesteeIDs: []string{"2"}})
	p := leanMemoryPlanner(t, reader, c, dir)
	r := reader.mongoRows["answersheets"][0]
	for _, root := range []string{"1", "1", "2"} {
		if e := p.mongoCandidate("answersheets", r, root); e != nil {
			t.Fatal(e)
		}
	}
	if p.writer.index.Counts["mongo/answersheets"] != 1 || p.writer.index.Holds["answersheets:conflicting_parent_components"] != 1 || p.owner("answersheets", "20") != "1" {
		t.Fatal("repeated parent paths changed scope, ownership, or conflict holds")
	}
	bad := graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(500)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: float64(1)}})
	for i := 0; i < 2; i++ {
		if e := p.mongoCandidate("answersheets", bad, "1"); e != nil {
			t.Fatal(e)
		}
	}
	if p.writer.index.Holds["answersheets:unassignable_testee_identity"] != 1 || p.writer.index.Counts["mongo/answersheets"] != 1 || p.seenMongo[objectID(bad.Object)] != "1" {
		t.Fatal("rejected physical candidate lost stage-local duplicate protection")
	}
	catalog := reader.mongoRows["report_query_catalog"][0]
	for i := 0; i < 2; i++ {
		if e := p.mongoCandidate("report_query_catalog", catalog, "1"); e != nil {
			t.Fatal(e)
		}
	}
	if p.writer.index.Counts["mongo/report_query_catalog"] != 1 || len(p.owners["report_query_catalog"]) != 0 || p.seenMongo[objectID(catalog.Object)] != "1" {
		t.Fatal("catalog duplicate protection was incorrectly replaced by domain ownership")
	}
}

type leanMemoryInterruptedReader struct {
	*graphFake
	failSQL, failMongo string
}

func (r *leanMemoryInterruptedReader) sqlPages(ctx context.Context, name, where string, args []any, emit func(record) error) error {
	if e := r.graphFake.sqlPages(ctx, name, where, args, emit); e != nil {
		return e
	}
	if name == r.failSQL {
		return errors.New("fixed read interrupted after records")
	}
	return nil
}
func (r *leanMemoryInterruptedReader) mongoPages(ctx context.Context, name string, filter bson.D, emit func(record) error) error {
	if e := r.graphFake.mongoPages(ctx, name, filter, emit); e != nil {
		return e
	}
	if name == r.failMongo && len(filter) > 0 && filter[0].Key == "$or" {
		return errors.New("fixed catalog read interrupted after records")
	}
	return nil
}

func TestLeanMemoryReleasesStageMapsOnlyAfterCompleteGuardedReads(t *testing.T) {
	for _, stage := range []string{"complete", "runtime_read_error", "runtime_flush_error", "catalog_read_error"} {
		t.Run(stage, func(t *testing.T) {
			base, c, _, dir := graphFixture(t)
			reader := &leanMemoryInterruptedReader{graphFake: base}
			p := leanMemoryPlanner(t, reader, c, dir)
			if e := p.processSQL("testee", base.sqlRows["testee"][0], func(graphObject) string { return "1" }, nil); e != nil {
				t.Fatal(e)
			}
			if e := p.writer.flush(); e != nil {
				t.Fatal(e)
			}
			for _, id := range []string{"91", "92"} {
				r := graphSQLFixture("runtime_checkpoint", id, map[string]string{"assessment_id": "100", "status": "succeeded"})
				r.Columns = append(r.Columns, cell{Name: "scope", Type: "VARCHAR", Raw: []byte("evaluation_run")}, cell{Name: "resource_id", Type: "VARCHAR", Raw: []byte("100:2")})
				base.sqlRows["runtime_checkpoint"] = append(base.sqlRows["runtime_checkpoint"], r)
			}
			if stage == "runtime_read_error" {
				reader.failSQL = "runtime_checkpoint"
			}
			if stage == "runtime_flush_error" {
				// The fixture has three SQL children before runtime. Its next
				// exclusive scope file must not be overwritten to complete a stage.
				if e := writeNew(filepath.Join(dir, "scope-00000005.json"), []byte("existing original")); e != nil {
					t.Fatal(e)
				}
			}
			err := p.sqlChildren()
			if strings.HasPrefix(stage, "runtime_") {
				if err == nil || p.owner("runtime_checkpoint", "100:2") != "1" || p.writer.index.Holds["runtime_checkpoint:duplicate_typed_domain_identity"] != 1 {
					t.Fatal("incomplete runtime phase lost its duplicate guard state", err)
				}
				return
			}
			if err != nil || p.owners["runtime_checkpoint"] != nil || p.writer.index.Holds["runtime_checkpoint:duplicate_typed_domain_identity"] != 1 || p.writer.index.Counts["mysql/runtime_checkpoint"] != 2 {
				t.Fatal("complete runtime phase changed its holds/counts or retained dead ownership", err)
			}
			p.parents["event_root"]["fixed-audit-event"] = "1"
			if stage == "catalog_read_error" {
				reader.failMongo = "report_query_catalog"
			}
			err = p.mongoChildren()
			if stage == "catalog_read_error" {
				if err == nil || len(p.seenMongo) == 0 || len(p.parents["generation_outcome"]) == 0 || len(p.parents["run_generation"]) == 0 {
					t.Fatal("incomplete Mongo phase discarded duplicate or parent guards", err)
				}
				return
			}
			if err != nil || p.seenMongo != nil {
				t.Fatal("complete Mongo phase retained its dead duplicate map", err)
			}
			for _, name := range []string{"enrollment_plan", "sheet_root", "generation_outcome", "run_generation"} {
				if p.parents[name] != nil {
					t.Fatal("completed parent-chain state was retained", name)
				}
			}
			if p.parents["event_root"]["fixed-audit-event"] != "1" || p.owner("interpret_report_artifacts", "700") != "1" {
				t.Fatal("later statistics or audit consumers lost their live identity maps")
			}
			if e := p.statistics(); e != nil {
				t.Fatal(e)
			}
			if e := p.retainedResponsibilities(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestLeanMemoryCachedWriterPreservesProtectionAndScopeBytes(t *testing.T) {
	o := graphObject{Object: key("assessment", "100"), Links: map[string]string{"id": "100", "testee_id": "1"}, RootIDs: []string{"1"}}
	var first []byte
	for _, cached := range []bool{false, true} {
		w := &graphWriter{dir: privateTestDir(t), index: &graphIndex{Counts: map[string]int64{}}, protected: map[string]bool{}}
		add := func(o graphObject) error { return w.add(o, 1) }
		if cached {
			add = func(o graphObject) error { return w.addPhysical(o, 1, objectID(o.Object)) }
		}
		w.protected[objectID(o.Object)] = true
		if e := add(o); e == nil || len(w.current) != 0 {
			t.Fatal("protected physical identity reached scope writer")
		}
		delete(w.protected, objectID(o.Object))
		bad := o
		bad.Object.Fingerprint = strings.Repeat("a", 64)
		if e := add(bad); e == nil || len(w.current) != 0 {
			t.Fatal("preimage fingerprint reached identity-only writer")
		}
		if e := add(o); e != nil {
			t.Fatal(e)
		}
		if e := w.flush(); e != nil {
			t.Fatal(e)
		}
		raw, e := os.ReadFile(filepath.Join(w.dir, w.index.Shards[0].File))
		if e != nil {
			t.Fatal(e)
		}
		if first != nil && !bytes.Equal(first, raw) {
			t.Fatal("cached identity changed persisted scope bytes")
		}
		first = raw
	}
}
