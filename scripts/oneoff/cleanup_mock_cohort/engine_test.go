package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type memoryStore struct {
	rows          map[string]record
	definitions   map[string][]byte
	writes        int
	ackUnknown    bool
	identityError error
}

func (s *memoryStore) identity(context.Context, binding) error { return s.identityError }
func (s *memoryStore) read(_ context.Context, o object) (record, bool, error) {
	r, ok := s.rows[objectID(o)]
	if !ok {
		return record{Object: o, Definition: s.definitions[o.Store+"/"+o.Name]}, false, nil
	}
	return r, true, nil
}
func (s *memoryStore) mutate(_ context.Context, rows []record, restore bool) error {
	s.writes++
	for _, r := range rows {
		if restore {
			s.rows[objectID(r.Object)] = r
		} else {
			delete(s.rows, objectID(r.Object))
		}
	}
	if s.ackUnknown {
		return errors.New("server committed, client did not receive ACK")
	}
	return nil
}

func key(table, id string) object {
	return object{Store: "mysql", Name: table, Key: []cell{{Name: "id", Type: "uint64", Raw: []byte(id)}}}
}
func privateTestDir(t *testing.T) string {
	t.Helper()
	home, e := os.UserHomeDir()
	if e != nil {
		t.Fatal(e)
	}
	dir, e := os.MkdirTemp(home, ".qs-mock-cohort-test-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func fixture(t *testing.T) (*memoryStore, group, protected, material, string) {
	t.Helper()
	dir := privateTestDir(t)
	s := &memoryStore{rows: map[string]record{}, definitions: map[string][]byte{}}
	p := protected{Format: 1, Kind: "mock_cohort_protected", IAM: []byte(`{"user_ids":["10001"],"profile_ids":[],"login_identity_ids":[]}`), QS: protectionScope{System: []object{key("testee", "42")}, Clack: []object{key("testee", "43")}, Other: []object{}}}
	g := group{Format: 1, Kind: "mock_cohort", Operation: "test-cohort", OwnerSHA: strings.Repeat("a", 64), ProtectedSHA: strings.Repeat("b", 64), IAM: []byte(`{"user_ids":[],"profile_ids":[],"login_identity_ids":[]}`), QS: qsScope{Binding: binding{SQLVersion: 101, MongoVersion: 39}, Selector: selection{Source: "mock", OrgID: "1", Evidence: "owner-declared exclusive mock component", From: "2025-01-01T00:00:00+08:00", Through: "2026-10-01T00:00:00+08:00"}, Objects: []object{key("assessment_score", "42")}}}
	for _, o := range append(append([]object{}, g.QS.Objects...), append(p.QS.System, p.QS.Clack...)...) {
		r := record{Object: o, Definition: []byte("fixed schema/session"), Columns: []cell{{Name: "id", Type: "BIGINT", Raw: append([]byte{}, o.Key[0].Raw...)}, {Name: "payload", Type: "JSON", Raw: []byte("null")}, {Name: "empty", Type: "VARBINARY", Raw: []byte{}}, {Name: "missing", Type: "TEXT", Null: true}}}
		s.rows[objectID(o)] = r
		s.definitions[o.Store+"/"+o.Name] = r.Definition
	}
	planned, e := plan(t.Context(), s, g, p)
	if e != nil {
		t.Fatal(e)
	}
	m, e := capture(t.Context(), s, planned, p, dir, strings.Repeat("c", 64), strings.Repeat("d", 40))
	if e != nil {
		t.Fatal(e)
	}
	return s, planned, p, m, dir
}

func TestPlanPhysicalDomainsAndExplicitExclusions(t *testing.T) {
	s, g, p, _, _ := fixture(t)
	if e := validateGroup(g, p); e != nil {
		t.Fatalf("equal Testee/Score numeric keys must be independent: %v", e)
	}
	g.QS.Objects = append(g.QS.Objects, key("assessment", "123"))
	planned, e := plan(t.Context(), s, g, p)
	if e != nil {
		t.Fatal(e)
	}
	if len(planned.QS.Objects) != 1 || len(planned.QS.Exclusions) != 1 || s.writes != 0 {
		t.Fatal("unknown root must be excluded without expanding Testee or writing")
	}
	g.QS.Objects = []object{p.QS.System[0]}
	if e = validateGroup(g, p); e == nil {
		t.Fatal("protected physical identity must reject before reads/writes")
	}
}

func TestArchivePreservesSQLNullJSONNullEmptyAndBinary(t *testing.T) {
	_, _, _, m, dir := fixture(t)
	r, e := archived(dir, m.Records[0])
	if e != nil {
		t.Fatal(e)
	}
	if !r.Columns[3].Null || r.Columns[3].Raw != nil || r.Columns[2].Null || r.Columns[2].Raw == nil || len(r.Columns[2].Raw) != 0 || !bytes.Equal(r.Columns[1].Raw, []byte("null")) {
		t.Fatal("SQL NULL/JSON null/empty bytes collapsed")
	}
	original := []byte{0xff, 0, 0xfe}
	r.Columns[2].Raw = original
	raw := encode(r)
	var decoded record
	if e = strictDecode(raw, &decoded); e != nil || !bytes.Equal(decoded.Columns[2].Raw, original) {
		t.Fatal("non-UTF8 bytes must survive without normalization")
	}
}

func TestUnknownCommitNeverReplaysUntilReadOnlyReconciled(t *testing.T) {
	s, g, _, m, dir := fixture(t)
	a := approval{Expires: time.Now().Add(time.Minute)}
	s.ackUnknown = true
	if e := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("e", 64), false); e == nil || s.writes != 1 {
		t.Fatal("commit ACK unknown must remain failed")
	}
	if e := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("f", 64), false); e == nil || s.writes != 1 {
		t.Fatal("unknown chunk must not replay")
	}
	if _, e := os.Stat(filepath.Join(dir, "approval-"+strings.Repeat("f", 64)+".consumed")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("unknown hold must precede new approval consumption")
	}
	if e := verify(t.Context(), s, m, dir, false); e != nil || s.writes != 1 {
		t.Fatalf("new read-only verification must resolve actual absent outcome: %v", e)
	}
	s.ackUnknown = false
	if e := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("f", 64), false); e != nil || s.writes != 1 {
		t.Fatal("resolved already-absent chunk must not issue another delete")
	}
	if e := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("1", 64), true); e != nil || s.writes != 2 {
		t.Fatalf("typed exact restoration must work: %v", e)
	}
	if e := verify(t.Context(), s, m, dir, true); e != nil {
		t.Fatal(e)
	}
}

func TestProtectionDriftStopsBeforeClaim(t *testing.T) {
	s, g, p, m, dir := fixture(t)
	r := s.rows[objectID(p.QS.System[0])]
	r.Columns[1].Raw = []byte(`{"real":"changed"}`)
	s.rows[objectID(p.QS.System[0])] = r
	if e := mutate(t.Context(), s, g, m, dir, approval{Expires: time.Now().Add(time.Minute)}, strings.Repeat("e", 64), false); e == nil || s.writes != 0 {
		t.Fatal("protection drift must stop all components")
	}
	if _, e := os.Stat(filepath.Join(dir, "approval-"+strings.Repeat("e", 64)+".consumed")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("protection failure must not consume authorization")
	}
}

func TestRestoreConflictingCurrentFactNeverOverwrites(t *testing.T) {
	s, g, _, m, dir := fixture(t)
	r := s.rows[objectID(g.QS.Objects[0])]
	r.Columns[1].Raw = []byte(`{"new":"business fact"}`)
	s.rows[objectID(g.QS.Objects[0])] = r
	if e := mutate(t.Context(), s, g, m, dir, approval{Expires: time.Now().Add(time.Minute)}, strings.Repeat("e", 64), true); e == nil || s.writes != 0 {
		t.Fatal("current fact must not be overwritten")
	}
}

func TestChunksRespectBothStoreCountsAndByteBudget(t *testing.T) {
	for _, store := range []string{"mysql", "mongo"} {
		m := material{}
		for i := range 2001 {
			m.Records = append(m.Records, recordRef{Object: object{Store: store, Name: "same"}, Bytes: 1024, SHA: fmt.Sprintf("%064d", i)})
		}
		for _, b := range batches(m) {
			limit := 1000
			if store == "mongo" {
				limit = 500
			}
			if len(b) > limit {
				t.Fatal("physical chunk count exceeded")
			}
		}
	}
	m := material{Records: []recordRef{{Object: object{Store: "mysql", Name: "same"}, Bytes: 32 * 1024 * 1024}, {Object: object{Store: "mysql", Name: "same"}, Bytes: 32 * 1024 * 1024}, {Object: object{Store: "mysql", Name: "same"}, Bytes: 1}}}
	if len(batches(m)) != 2 {
		t.Fatal("bytes must split otherwise legal count")
	}
}

func TestBSONPhysicalIdentityTypesRemainDistinct(t *testing.T) {
	ids := []any{primitive.NewObjectID(), "7", int32(7), int64(7)}
	seen := map[string]bool{}
	for _, id := range ids {
		typ, raw, e := bson.MarshalValue(id)
		if e != nil {
			t.Fatal(e)
		}
		o := object{Store: "mongo", Name: "report_query_catalog", Key: []cell{{Name: "_id", Type: fmt.Sprintf("bson:%02x", byte(typ)), Raw: raw}}}
		if e = validateObject(o); e != nil {
			t.Fatal(e)
		}
		if _, e = mongoKey(o); e != nil {
			t.Fatal(e)
		}
		if seen[objectID(o)] {
			t.Fatal("BSON type identity collapsed")
		}
		seen[objectID(o)] = true
	}
}

func TestStrictNestedDuplicateAndUnknownFields(t *testing.T) {
	for _, raw := range []string{`{"format_version":1,"format_version":1}`, `{"qs":{"selector":{"source":"x","source":"y"}}}`, `{"unknown":true}`} {
		var g group
		if e := strictDecode([]byte(raw), &g); e == nil {
			t.Fatal("ambiguous protocol accepted")
		}
	}
}

func TestApprovalBindsSourceExactHashesAndAbsoluteWindow(t *testing.T) {
	_, g, _, m, _ := fixture(t)
	now := time.Now()
	a := approval{Format: 1, Kind: "mock_cohort_action_approval", Action: "apply", Operation: m.Operation, GroupSHA: m.GroupSHA, ArchiveSHA: strings.Repeat("a", 64), ProtectedSHA: m.ProtectedSHA, ToolSource: m.ToolSource, FreezeSHA: strings.Repeat("b", 64), SettlementSHA: strings.Repeat("c", 64), Issued: now.Add(-time.Minute), Expires: now.Add(time.Minute)}
	if e := checkApproval(a, m, g, a.ArchiveSHA, "apply", m.ToolSource, now); e != nil {
		t.Fatal(e)
	}
	a.Expires = now.Add(-time.Second)
	if e := checkApproval(a, m, g, a.ArchiveSHA, "apply", m.ToolSource, now); e == nil {
		t.Fatal("expired approval accepted")
	}
	a.Expires = now.Add(time.Hour)
	if e := checkApproval(a, m, g, a.ArchiveSHA, "apply", m.ToolSource, now); e == nil {
		t.Fatal("phase may not reset or extend maintenance window")
	}
}

func TestDefaultCLIIsDryRun(t *testing.T) {
	c, e := flags(nil)
	if e != nil || c.action != "plan" || c.execute {
		t.Fatal("default must remain read-only plan")
	}
	if _, e = flags([]string{"--action=plan", "--execute"}); e == nil {
		t.Fatal("plan cannot acquire writes")
	}
	if _, e = flags([]string{"--timeout=31m"}); e == nil {
		t.Fatal("unbounded timeout accepted")
	}
}

func TestJournalTruncationAndHashChainFailClosed(t *testing.T) {
	dir := privateTestDir(t)
	entry := journalEntry{Action: "apply", Chunk: strings.Repeat("a", 64), State: "intent"}
	if e := appendJournal(dir, entry); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "journal.jsonl"), raw[:len(raw)-1], 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e = journalState(dir, entry.Chunk); e == nil {
		t.Fatal("torn journal cannot become an acknowledged operation")
	}
}

func TestSharedProtectionOriginalBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/shared-protected-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if digest(raw) != "957d351308ec62ca1d8774ca76383afd622b236f537842af0dabbfda21cfe90f" {
		t.Fatal("shared protection original bytes changed")
	}
	var p protected
	if err := strictDecode(raw, &p); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(raw), encode(p)) {
		t.Fatal("QS and IAM protection envelope disagree")
	}
}
