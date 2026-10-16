package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type graphClearanceFixture struct {
	config   config
	manifest graphClearanceManifest
	store    *graphMemory
	batch    graphClearanceBatch
}

func clearanceFixture(t *testing.T) graphClearanceFixture {
	t.Helper()
	prior := toolSource
	toolSource = strings.Repeat("e", 40)
	t.Cleanup(func() { toolSource = prior })
	s, g, p, index, m, scope, dir := graphEngineFixture(t)
	_, selector, _, _ := graphFixture(t)
	inputs := privateTestDir(t)
	write := func(name string, raw []byte) graphClearanceFile {
		ref := graphClearanceFile{filepath.Join(inputs, name), digest(raw)}
		if e := writeNew(ref.File, raw); e != nil {
			t.Fatal(e)
		}
		return ref
	}
	protection := write("protection.json", encode(p))
	g.ProtectedSHA = protection.SHA
	g.QS.Binding = selector.Binding
	g.QS.Selector = selector.Selection
	g.IAM = encode(iamScope{Users: []string{"10"}, Profiles: []string{"11"}, Identities: []string{"12"}})
	selector.ProtectionSHA = protection.SHA
	selectorRef := write("selector.json", encode(selector))
	index.Holds = map[string]int64{}
	index.Kind = "mock_cohort_graph_index"
	index.Source = strings.Repeat("b", 40)
	index.SelectorSHA = selectorRef.SHA
	index.ProtectionSHA = protection.SHA
	index.OwnerEvidenceSHA = selector.OwnerEvidenceSHA
	index.Binding = selector.Binding
	index.ReadOnly = true
	index.DiscoveryComplete = true
	index.Released = true
	indexRaw := encode(index)
	if e := writeNew(filepath.Join(scope, "graph-index.json"), indexRaw); e != nil {
		t.Fatal(e)
	}
	// Historical write authority is expired. Clearance must not change these bytes.
	g.QS.Graph.IndexSHA = digest(indexRaw)
	g.QS.Graph.IndexSource = index.Source
	g.QS.Graph.SelectorSHA = selectorRef.SHA
	g.QS.Graph.WindowStarted = time.Now().UTC().Add(-2 * time.Hour)
	g.QS.Graph.WindowExpires = g.QS.Graph.WindowStarted.Add(20 * time.Minute)
	owner := write("owner.json", encode(ownerConfirmation{1, "mock_cohort_owner_confirmation", g.Operation, "10001", digest(g.IAM), digest(encode(g.QS)), g.ProtectedSHA, true, true, []string{}}))
	g.OwnerSHA = owner.SHA
	groupRef := write("group.json", encode(g))
	m.Operation = g.Operation
	m.GroupSHA = groupRef.SHA
	m.ProtectionSHA = protection.SHA
	m.Graph = *g.QS.Graph
	m.Binding = g.QS.Binding
	materialRaw := encode(m)
	if e := writeNew(filepath.Join(dir, "material.json"), materialRaw); e != nil {
		t.Fatal(e)
	}
	capture := write("capture-stdout.json", append(encode(actionReceipt{1, "mock_cohort_qs_action_result", "capture", g.Operation, groupRef.SHA, m.Source, digest(materialRaw), true, false, false, false, 0, true}), '\n'))
	zero := 0
	proof := write("capture-execution.json", encode(graphCaptureExecution{1, "mock_cohort_capture_execution", capture.File, capture.SHA, "exited", &zero}))
	applied := write("apply-stdout.json", append(encode(actionReceipt{1, "mock_cohort_qs_action_result", "apply", g.Operation, groupRef.SHA, m.Source, digest(materialRaw), true, false, false, true, 0, true}), '\n'))
	applyProof := write("apply-execution.json", encode(graphCaptureExecution{1, "mock_cohort_apply_execution", applied.File, applied.SHA, "exited", &zero}))
	j, e := openGraphJournal(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, shard := range m.Targets {
		entry := journalEntry{Action: "apply", Chunk: graphChunkID(shard), ApprovalSHA: strings.Repeat("a", 64), State: "intent"}
		if e = j.append(entry); e != nil {
			t.Fatal(e)
		}
		entry.State = "committed"
		if e = j.append(entry); e != nil {
			t.Fatal(e)
		}
		rows, e := readGraphArchivePage(dir, m, shard)
		if e != nil {
			t.Fatal(e)
		}
		for _, row := range rows {
			delete(s.rows, objectID(row.Object))
		}
	}
	if e = j.close(); e != nil {
		t.Fatal(e)
	}
	journal, e := privateRead(filepath.Join(dir, "graph-journal.jsonl"), 128<<20)
	if e != nil {
		t.Fatal(e)
	}
	frozen := write("current-freeze.json", encode(freezeReceipt{1, "mock_cohort_frozen", g.Operation, groupRef.SHA, true, true, true, true}))
	in := graphClearanceBatchInput{groupRef, owner, selectorRef, scope, digest(materialRaw), proof, applied, applyProof, digest(journal), frozen}
	manifest := graphClearanceManifest{1, "mock_cohort_qs_clearance_inputs", g.Operation, []graphClearanceBatchInput{in}}
	ref := write("clearance-inputs.json", encode(manifest))
	return graphClearanceFixture{config{action: "clearance", clearanceManifest: ref.File, clearanceManifestSHA: ref.SHA, protection: protection.File, protectionSHA: protection.SHA, timeout: time.Minute, workers: 1}, manifest, s, graphClearanceBatch{input: in, group: g, index: index, material: m}}
}
func TestGraphClearanceExpiredWriteWindowAndForwardBoundaryRemainReadonly(t *testing.T) {
	f := clearanceFixture(t)
	marker := []byte(`{"business_reopened":true}`)
	if e := writeNew(filepath.Join(f.batch.material.Graph.ArchiveDirectory, "forward-only.json"), marker); e != nil {
		t.Fatal(e)
	}
	if e := graphForwardGuard(f.batch.material.Graph.ArchiveDirectory); e == nil {
		t.Fatal("write boundary was not active")
	}
	_, batches, guard, union, refs, e := graphClearanceLoad(f.config)
	if e != nil {
		t.Fatal(e)
	}
	if len(batches) != 1 || len(union.QS.Graph.RootIDs) != 1 || !refs.ids["mongo/answersheets"]["20"] {
		t.Fatal("frozen typed parent union lost")
	}
	before, e := privateRead(filepath.Join(f.batch.material.Graph.ArchiveDirectory, "graph-journal.jsonl"), 128<<20)
	if e != nil {
		t.Fatal(e)
	}
	if e = graphClearanceTargets(t.Context(), f.store, batches[0]); e != nil {
		t.Fatal(e)
	}
	after, e := privateRead(filepath.Join(f.batch.material.Graph.ArchiveDirectory, "graph-journal.jsonl"), 128<<20)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(before, after) || f.store.writes != 0 {
		t.Fatal("readonly clearance mutated journal or data")
	}
	if e = guard.full(); e != nil {
		t.Fatal(e)
	}
	current, e := os.ReadFile(filepath.Join(f.batch.material.Graph.ArchiveDirectory, "forward-only.json"))
	if e != nil || !bytes.Equal(current, marker) {
		t.Fatal("clearance changed forward boundary")
	}
}
func TestGraphClearanceJournalRejectsUnknownMissingRestoreAndForeignChunks(t *testing.T) {
	f := clearanceFixture(t)
	raw, e := privateRead(filepath.Join(f.batch.material.Graph.ArchiveDirectory, "graph-journal.jsonl"), 128<<20)
	if e != nil {
		t.Fatal(e)
	}
	entries, e := readJournal(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"unknown", "intent", "restore", "missing", "foreign", "truncated"} {
		t.Run(kind, func(t *testing.T) {
			changed := append([]journalEntry{}, entries...)
			switch kind {
			case "unknown", "intent":
				changed[len(changed)-1].State = kind
			case "restore":
				changed[len(changed)-1].Action = "restore"
			case "missing":
				changed = changed[:len(changed)-2]
			case "foreign":
				changed[len(changed)-1].Chunk = strings.Repeat("f", 64)
			}
			var buf bytes.Buffer
			prev := ""
			for i, v := range changed {
				v.Seq = i + 1
				v.Previous = prev
				buf.Write(append(encode(v), '\n'))
				prev = digest(encode(v))
			}
			testRaw := buf.Bytes()
			if kind == "truncated" {
				testRaw = testRaw[:len(testRaw)-1]
			}
			if e := graphClearanceJournal(testRaw, f.batch.material); e == nil {
				t.Fatal("unsafe journal qualified")
			}
		})
	}
}
func TestGraphClearanceActualPartialExecutionAndLiteralFieldsRequired(t *testing.T) {
	f := clearanceFixture(t)
	raw, e := privateRead(f.manifest.Batches[0].PartialApplyExecution.File, 32<<10)
	if e != nil {
		t.Fatal(e)
	}
	for _, variant := range []string{"exit1", "null", "alias", "missing", "wrongreceipt"} {
		t.Run(variant, func(t *testing.T) {
			var doc map[string]any
			if e := json.Unmarshal(raw, &doc); e != nil {
				t.Fatal(e)
			}
			switch variant {
			case "exit1":
				doc["exit_code"] = 1
			case "null":
				doc["exit_code"] = nil
			case "alias":
				doc["Exit_Code"] = doc["exit_code"]
				delete(doc, "exit_code")
			case "missing":
				delete(doc, "exit_code")
			case "wrongreceipt":
				doc["receipt_sha256"] = strings.Repeat("f", 64)
			}
			changed := encode(doc)
			path := filepath.Join(privateTestDir(t), "execution.json")
			if e := writeNew(path, changed); e != nil {
				t.Fatal(e)
			}
			if e := graphClearanceExecution(graphClearanceFile{path, digest(changed)}, f.manifest.Batches[0].PartialApply, "mock_cohort_apply_execution", &graphInputGuard{}); e == nil {
				t.Fatal("unproven or nonliteral process evidence qualified")
			}
		})
	}
}
func TestGraphClearanceTargetReappearanceAndProtectionConflictFailWithoutWrites(t *testing.T) {
	for _, kind := range []string{"target", "protected", "definition"} {
		t.Run(kind, func(t *testing.T) {
			f := clearanceFixture(t)
			ref := f.batch.material.Targets[0]
			if kind == "protected" {
				ref = f.batch.material.Guards[0]
			}
			rows, e := readGraphArchivePage(f.batch.material.Graph.ArchiveDirectory, f.batch.material, ref)
			if e != nil {
				t.Fatal(e)
			}
			row := rows[0]
			if kind == "protected" {
				row.Columns = append([]cell{}, row.Columns...)
				row.Columns[1].Raw = []byte("foreign")
			}
			if kind == "definition" {
				f.store.definitions[row.Object.Store+"/"+row.Object.Name] = []byte("changed definition")
			} else {
				f.store.rows[objectID(row.Object)] = row
			}
			if e := graphClearanceTargets(t.Context(), f.store, f.batch); e == nil {
				t.Fatal("unsafe current fact qualified")
			}
			if f.store.writes != 0 {
				t.Fatal("readonly check wrote data")
			}
		})
	}
}
func TestGraphClearanceManifestRejectsDuplicateComponentsAndChangedFreeze(t *testing.T) {
	for _, kind := range []string{"duplicate", "freeze", "hash"} {
		t.Run(kind, func(t *testing.T) {
			f := clearanceFixture(t)
			switch kind {
			case "duplicate":
				f.manifest.Batches = append(f.manifest.Batches, f.manifest.Batches[0])
			case "freeze":
				f.manifest.Batches[0].CurrentFreeze = f.manifest.Batches[0].Owner
			case "hash":
				f.manifest.Batches[0].MaterialSHA = strings.Repeat("f", 64)
			}
			raw := encode(f.manifest)
			path := filepath.Join(privateTestDir(t), "inputs.json")
			if e := writeNew(path, raw); e != nil {
				t.Fatal(e)
			}
			f.config.clearanceManifest = path
			f.config.clearanceManifestSHA = digest(raw)
			if _, _, _, _, _, e := graphClearanceLoad(f.config); e == nil {
				t.Fatal("changed or overlapping authority accepted")
			}
		})
	}
}
func TestGraphClearanceAndFrozenDeleteFlagsStayExplicit(t *testing.T) {
	for _, args := range [][]string{{"--action=clearance"}, {"--action=apply", "--clearance-manifest-file="}, {"--action=clearance", "--clearance-manifest-file=/private/input", "--clearance-manifest-sha256=" + strings.Repeat("a", 64), "--execute"}, {"--frozen-rule-delete", "--action=apply"}, {"--rule-batch", "--frozen-rule-delete", "--action=restore"}, {"--rule-batch", "--frozen-rule-delete", "--action=verify", "--expect-original"}} {
		if _, e := flags(args); e == nil {
			t.Fatal("incompatible flag authority accepted")
		}
	}
	for _, action := range []string{"capture", "apply", "verify"} {
		if _, e := flags([]string{"--rule-batch", "--frozen-rule-delete", "--action=" + action}); e != nil {
			t.Fatal(e)
		}
	}
}

func TestGraphClearanceRejectsWholeIncompleteAndMismatchedApplyOriginals(t *testing.T) {
	for _, variant := range []string{"whole", "incomplete", "unreleased", "notpartial", "source", "material", "group"} {
		t.Run(variant, func(t *testing.T) {
			f := clearanceFixture(t)
			in := f.manifest.Batches[0]
			raw, e := privateRead(in.PartialApply.File, 64<<10)
			if e != nil {
				t.Fatal(e)
			}
			var r actionReceipt
			if e := strictDecode(raw, &r); e != nil {
				t.Fatal(e)
			}
			switch variant {
			case "whole":
				r.Whole = true
			case "incomplete":
				r.Complete = false
			case "unreleased":
				r.Released = false
			case "notpartial":
				r.Pending = false
			case "source":
				r.Source = strings.Repeat("f", 40)
			case "material":
				r.MaterialSHA = strings.Repeat("f", 64)
			case "group":
				r.GroupSHA = strings.Repeat("f", 64)
			}
			dir := privateTestDir(t)
			changed := append(encode(r), '\n')
			in.PartialApply = graphClearanceFile{filepath.Join(dir, "apply.json"), digest(changed)}
			if e = writeNew(in.PartialApply.File, changed); e != nil {
				t.Fatal(e)
			}
			zero := 0
			proof := encode(graphCaptureExecution{1, "mock_cohort_apply_execution", in.PartialApply.File, in.PartialApply.SHA, "exited", &zero})
			in.PartialApplyExecution = graphClearanceFile{filepath.Join(dir, "execution.json"), digest(proof)}
			if e = writeNew(in.PartialApplyExecution.File, proof); e != nil {
				t.Fatal(e)
			}
			f.manifest.Batches[0] = in
			manifest := encode(f.manifest)
			f.config.clearanceManifest = filepath.Join(dir, "manifest.json")
			f.config.clearanceManifestSHA = digest(manifest)
			if e = writeNew(f.config.clearanceManifest, manifest); e != nil {
				t.Fatal(e)
			}
			if _, _, _, _, _, e = graphClearanceLoad(f.config); e == nil {
				t.Fatal("invalid actual apply receipt qualified after new custody hashes")
			}
		})
	}
}

func TestGraphClearanceKnownACKReadsProtectionOnceWithoutJournalMutation(t *testing.T) {
	f := clearanceFixture(t)
	_, batches, _, _, _, err := graphClearanceLoad(f.config)
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(f.batch.material.Graph.ArchiveDirectory, "graph-journal.jsonl")
	before, err := privateRead(journalPath, 128<<20)
	if err != nil {
		t.Fatal(err)
	}
	f.store.readBatches = 0
	if err = graphClearanceTargets(t.Context(), f.store, batches[0]); err != nil {
		t.Fatal(err)
	}
	m := f.batch.material
	if f.store.readBatches != len(m.Targets)+len(m.Guards)+len(m.Retained) || f.store.writes != 0 {
		t.Fatal("clearance repeated protection comparison or issued mutation", f.store.readBatches)
	}
	after, err := privateRead(journalPath, 128<<20)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("clearance changed acknowledged journal")
	}
}

func TestGraphClearanceKeyOnlyReadStillRequiresOriginalRawShardSHA(t *testing.T) {
	f := clearanceFixture(t)
	_, batches, _, _, _, err := graphClearanceLoad(f.config)
	if err != nil {
		t.Fatal(err)
	}
	ref := f.batch.material.Targets[0]
	path := filepath.Join(f.batch.material.Graph.ArchiveDirectory, ref.File)
	raw, err := privateRead(path, int64(ref.Bytes))
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] = ' '
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = graphClearanceTargets(t.Context(), f.store, batches[0]); err == nil || f.store.writes != 0 {
		t.Fatal("changed raw archive admitted through key-only clearance", err)
	}
}
