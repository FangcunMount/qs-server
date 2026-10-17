package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestFrozenArchiveKeysPreservePhysicalIdentityWithoutMaterializingBodies(t *testing.T) {
	_, g, p, index, m, _, dir := graphEngineFixture(t)
	if err := graphFrozenArchiveHeaders(m, g, p, index); err != nil {
		t.Fatal(err)
	}
	for _, ref := range m.Targets {
		original, err := readGraphArchivePage(dir, m, ref)
		if err != nil {
			t.Fatal(err)
		}
		keys, err := readGraphFrozenKeys(dir, m, ref)
		if err != nil {
			t.Fatal(err)
		}
		for i, r := range keys {
			if !reflect.DeepEqual(r.Object, original[i].Object) || !bytes.Equal(r.Definition, original[i].Definition) || r.Columns != nil || r.BSON != nil {
				t.Fatal("key projection changed original identity or materialized the archived body")
			}
		}
		if _, err = graphBatchRecords(keys); err == nil {
			t.Fatal("key-only projection entered ordinary full-original path")
		}
	}
}
func TestFrozenArchiveKeysRejectChangedBytesAndTypedShardShape(t *testing.T) {
	_, _, _, _, m, _, dir := graphEngineFixture(t)
	ref := m.Targets[0]
	raw, err := privateRead(filepath.Join(dir, ref.File), int64(ref.Bytes))
	if err != nil {
		t.Fatal(err)
	}
	other := privateTestDir(t)
	if err = writeNew(filepath.Join(other, ref.File), append(raw, ' ')); err != nil {
		t.Fatal(err)
	}
	if _, err = readGraphFrozenKeys(other, m, ref); err == nil {
		t.Fatal("changed original raw SHA admitted")
	}
	for _, variant := range []string{"missing_definition", "count", "store", "oversize", "hash"} {
		t.Run(variant, func(t *testing.T) {
			mm, rr := m, ref
			switch variant {
			case "missing_definition":
				mm.Namespaces = map[string][]byte{}
			case "count":
				rr.Count++
			case "store":
				rr.Store = "foreign"
			case "oversize":
				rr.Bytes = graphBatchBytes + 1
			case "hash":
				rr.SHA = strings.Repeat("f", 64)
			}
			if _, err := readGraphFrozenKeys(dir, mm, rr); err == nil {
				t.Fatal("changed key shard authority admitted")
			}
		})
	}
}
func TestFrozenArchiveHeadersRejectChangedRankCountsAndDefinition(t *testing.T) {
	_, g, p, index, m, _, _ := graphEngineFixture(t)
	for _, variant := range []string{"rank", "counts", "definition", "guard_count", "namespace", "ordinal"} {
		t.Run(variant, func(t *testing.T) {
			mm := m
			mm.Targets = append([]graphShard{}, m.Targets...)
			switch variant {
			case "rank":
				mm.Targets[0].RestoreRank = 0
			case "counts":
				mm.Counts = map[string]int64{}
			case "definition":
				mm.Namespaces = map[string][]byte{}
			case "guard_count":
				mm.ProtectedCount++
			case "namespace":
				mm.Targets[0].Name = "ai_bridge_requests"
			case "ordinal":
				mm.Targets[0].File = "t-99999999.json"
			}
			if err := graphFrozenArchiveHeaders(mm, g, p, index); err == nil {
				t.Fatal("changed trusted capture header admitted")
			}
		})
	}
}
func TestFrozenArchiveKeyOnlySQLStillBindsMetadataCountAndCommit(t *testing.T) {
	s, mock := graphBatchMock(t)
	original := graphBatchRow("1", strings.Repeat("preserved original", 1000))
	dir := privateTestDir(t)
	material := graphArchive{Namespaces: map[string][]byte{}, Counts: map[string]int64{}}
	if err := archiveGraphPage(dir, &material, []record{original}, graphSQLTables["assessment_score"], false); err != nil {
		t.Fatal(err)
	}
	keys, err := readGraphFrozenKeys(dir, material, material.Targets[0])
	if err != nil {
		t.Fatal(err)
	}
	graphBatchExpectDeletePreflight(mock)
	mock.ExpectBegin()
	graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
	mock.ExpectQuery("SELECT x.`id` FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(frozenRuleSQLKeys("1"))
	graphBatchExpectNoSideEffects(mock)
	graphBatchExpectPacket(mock, 64*1024*1024)
	mock.ExpectExec("DELETE FROM `assessment_score` WHERE (`id` = ?)").WithArgs([]byte("1")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT x.`id` FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(frozenRuleSQLKeys())
	mock.ExpectCommit()
	if err = s.mutateExactBatch(graphFrozenRuleDeleteContext(t.Context(), true), keys, false); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func frozenArchiveCommitted(t *testing.T, s *graphMemory, m graphArchive, dir string) *graphJournal {
	t.Helper()
	j, err := openGraphJournal(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.close() })
	for _, ref := range m.Targets {
		rows, err := readGraphArchivePage(dir, m, ref)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			delete(s.rows, objectID(row.Object))
		}
		if err = j.append(journalEntry{Action: "apply", Chunk: graphChunkID(ref), ApprovalSHA: strings.Repeat("a", 64), State: "committed"}); err != nil {
			t.Fatal(err)
		}
	}
	return j
}
func TestFrozenArchiveKnownACKVerifyReadsFinalProtectionOnce(t *testing.T) {
	s, _, _, _, m, _, dir := graphEngineFixture(t)
	j := frozenArchiveCommitted(t, s, m, dir)
	s.readBatches = 0
	if err := graphVerify(graphFrozenRuleDeleteContext(t.Context(), true), s, m, dir, j, false); err != nil {
		t.Fatal(err)
	}
	if s.readBatches != len(m.Targets)+len(m.Guards)+len(m.Retained) || s.writes != 0 {
		t.Fatal("known ACK repeated full protection or wrote data", s.readBatches)
	}
}
func TestFrozenArchiveUnknownStillReadsOriginalAndNeverReplays(t *testing.T) {
	s, _, _, _, m, _, dir := graphEngineFixture(t)
	j, err := openGraphJournal(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.close() }()
	ref := m.Targets[0]
	if err = j.append(journalEntry{Action: "apply", Chunk: graphChunkID(ref), ApprovalSHA: strings.Repeat("a", 64), State: "unknown"}); err != nil {
		t.Fatal(err)
	}
	if err = graphVerify(graphFrozenRuleDeleteContext(t.Context(), true), s, m, dir, j, false); err == nil {
		t.Fatal("original existing targets became known absent")
	}
	if j.latest[graphChunkID(ref)].State != "reconciled_original" || s.writes != 0 {
		t.Fatal("unknown did not require complete original equality or issued a write")
	}
}
func TestFrozenArchiveAcknowledgedTargetReappearanceCannotBeDeletedAgain(t *testing.T) {
	s, _, _, _, m, _, dir := graphEngineFixture(t)
	j := frozenArchiveCommitted(t, s, m, dir)
	rows, err := readGraphArchivePage(dir, m, m.Targets[0])
	if err != nil {
		t.Fatal(err)
	}
	s.rows[objectID(rows[0].Object)] = rows[0]
	before, err := os.ReadFile(filepath.Join(dir, "graph-journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	err = graphMutateOrdered(graphFrozenRuleDeleteContext(context.Background(), true), s, m, dir, j, strings.Repeat("b", 64), false, func() error { return nil }, 4)
	if err == nil || s.writes != 0 {
		t.Fatal("known absent target was automatically deleted again", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "graph-journal.jsonl"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("reappeared ACK target acquired a new intent")
	}
}
