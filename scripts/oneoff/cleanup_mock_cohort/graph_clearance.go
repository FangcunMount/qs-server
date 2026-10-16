package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"time"
)

type graphClearanceFile struct {
	File string `json:"file"`
	SHA  string `json:"sha256"`
}
type graphClearanceBatchInput struct {
	Group                 graphClearanceFile `json:"group"`
	Owner                 graphClearanceFile `json:"owner_confirmation"`
	Selector              graphClearanceFile `json:"selector"`
	ScopeDirectory        string             `json:"scope_directory"`
	MaterialSHA           string             `json:"material_sha256"`
	CaptureEvidence       graphClearanceFile `json:"capture_evidence"`
	PartialApply          graphClearanceFile `json:"partial_apply"`
	PartialApplyExecution graphClearanceFile `json:"partial_apply_execution"`
	JournalSHA            string             `json:"journal_sha256"`
	CurrentFreeze         graphClearanceFile `json:"current_frozen_receipt"`
}
type graphClearanceManifest struct {
	Format    int                        `json:"format_version"`
	Kind      string                     `json:"kind"`
	Operation string                     `json:"operation_id"`
	Batches   []graphClearanceBatchInput `json:"batches"`
}
type graphClearanceIAMCounts struct {
	Users      int `json:"users"`
	Profiles   int `json:"profiles"`
	Identities int `json:"login_identities"`
}
type graphClearanceMember struct {
	GroupSHA                 string                  `json:"group_manifest_sha256"`
	OwnerSHA                 string                  `json:"owner_confirmation_sha256"`
	IAMSHA                   string                  `json:"iam_sha256"`
	QSSHA                    string                  `json:"qs_sha256"`
	IAMCounts                graphClearanceIAMCounts `json:"iam_counts"`
	MaterialSHA              string                  `json:"material_sha256"`
	CaptureSHA               string                  `json:"qs_capture_sha256"`
	CaptureEvidenceSHA       string                  `json:"capture_evidence_sha256"`
	PartialApplySHA          string                  `json:"qs_partial_apply_sha256"`
	PartialApplyExecutionSHA string                  `json:"qs_partial_apply_execution_sha256"`
	JournalSHA               string                  `json:"journal_sha256"`
	CurrentFreezeSHA         string                  `json:"current_frozen_receipt_sha256"`
}
type graphClearanceReceipt struct {
	Format       int                    `json:"format_version"`
	Kind         string                 `json:"kind"`
	Action       string                 `json:"action"`
	Operation    string                 `json:"operation_id"`
	Source       string                 `json:"compiled_source"`
	ManifestSHA  string                 `json:"batch_manifest_sha256"`
	ProtectedSHA string                 `json:"protected_manifest_sha256"`
	Binding      binding                `json:"binding"`
	Started      time.Time              `json:"observed_started_at"`
	Completed    time.Time              `json:"observed_completed_at"`
	Expires      time.Time              `json:"expires_at"`
	Batches      []graphClearanceMember `json:"batches"`
	Absent       bool                   `json:"targets_absent"`
	Protected    bool                   `json:"protected_unchanged"`
	Clear        bool                   `json:"global_references_clear"`
	Complete     bool                   `json:"complete"`
	Released     bool                   `json:"resource_release_proven"`
}

type graphClearanceBatch struct {
	input    graphClearanceBatchInput
	group    group
	index    graphIndex
	material graphArchive
	member   graphClearanceMember
}

func graphClearanceRead(ref graphClearanceFile, max int64, out any, guard *graphInputGuard) ([]byte, error) {
	if !filepath.IsAbs(ref.File) || filepath.Clean(ref.File) != ref.File || !hashPattern.MatchString(ref.SHA) {
		return nil, errors.New("clearance requires canonical fixed private original references")
	}
	raw, e := privateRead(ref.File, max)
	if e != nil {
		return nil, e
	}
	if digest(raw) != ref.SHA {
		return nil, errors.New("clearance original raw SHA mismatch")
	}
	if out != nil {
		if e = strictDecode(raw, out); e != nil {
			return nil, e
		}
	}
	info, change, e := graphPrivateInfo(ref.File)
	if e != nil {
		return nil, e
	}
	guard.files = append(guard.files, graphOriginalInput{path: ref.File, sha: ref.SHA, max: max, info: info, changeTime: change})
	return raw, nil
}
func graphClearanceExecution(ref graphClearanceFile, receipt graphClearanceFile, kind string, guard *graphInputGuard) error {
	var proof graphCaptureExecution
	if _, e := graphClearanceRead(ref, 32<<10, &proof, guard); e != nil {
		return e
	}
	if proof.Format != 1 || proof.Kind != kind || proof.ReceiptFile != receipt.File || proof.ReceiptSHA != receipt.SHA || proof.Termination != "exited" || proof.Exit == nil || *proof.Exit != 0 {
		return errors.New("clearance requires original successful external process termination")
	}
	return nil
}
func graphClearanceLoad(c config) (graphClearanceManifest, []graphClearanceBatch, *graphInputGuard, group, graphFrozenReferences, error) {
	var manifest graphClearanceManifest
	guard := &graphInputGuard{}
	union := group{IAM: json.RawMessage(`{"user_ids":[],"profile_ids":[],"login_identity_ids":[]}`), QS: qsScope{Graph: &graphBinding{RootIDs: []string{}}}}
	refs := graphFrozenReferences{ids: map[string]map[string]bool{}, physical: map[string]string{}}
	fail := func(e error) (graphClearanceManifest, []graphClearanceBatch, *graphInputGuard, group, graphFrozenReferences, error) {
		return manifest, nil, nil, union, refs, e
	}
	if _, e := graphClearanceRead(graphClearanceFile{c.clearanceManifest, c.clearanceManifestSHA}, 16<<20, &manifest, guard); e != nil {
		return fail(e)
	}
	if manifest.Format != 1 || manifest.Kind != "mock_cohort_qs_clearance_inputs" || !idPattern.MatchString(manifest.Operation) || len(manifest.Batches) == 0 || len(manifest.Batches) > 4096 || !sourcePattern.MatchString(toolSource) {
		return fail(errors.New("fixed bounded partial batch clearance manifest required"))
	}
	var protection protected
	if _, e := graphClearanceRead(graphClearanceFile{c.protection, c.protectionSHA}, 32<<20, &protection, guard); e != nil {
		return fail(e)
	}
	batches := make([]graphClearanceBatch, 0, len(manifest.Batches))
	seen := [4]map[string]bool{{}, {}, {}, {}}
	iamUnion := iamScope{Users: []string{}, Profiles: []string{}, Identities: []string{}}
	for _, in := range manifest.Batches {
		var b graphClearanceBatch
		b.input = in
		raw, e := graphClearanceRead(in.Group, 256<<20, &b.group, guard)
		if e != nil {
			return fail(e)
		}
		g := b.group
		if g.Operation != manifest.Operation || g.ProtectedSHA != c.protectionSHA || g.QS.Graph == nil || in.Owner.SHA != g.OwnerSHA {
			return fail(errors.New("clearance batch operation or protection differs"))
		}
		if len(batches) > 0 && !reflect.DeepEqual(g.QS.Binding, union.QS.Binding) {
			return fail(errors.New("clearance batches cross database identities"))
		}
		owner, e := graphClearanceRead(in.Owner, 32<<10, nil, guard)
		if e != nil {
			return fail(e)
		}
		if e = validateOwner(raw, owner, g); e != nil {
			return fail(e)
		}
		var parts map[string]json.RawMessage
		if e = json.Unmarshal(raw, &parts); e != nil {
			return fail(e)
		}
		var iam iamScope
		if e = strictDecode(g.IAM, &iam); e != nil {
			return fail(e)
		}
		for n, ids := range [][]string{g.QS.Graph.RootIDs, iam.Users, iam.Profiles, iam.Identities} {
			for _, id := range ids {
				if seen[n][id] {
					return fail(errors.New("clearance batches overlap complete identity components"))
				}
				seen[n][id] = true
			}
		}
		union.Operation, union.QS.Binding = g.Operation, g.QS.Binding
		union.QS.Graph.RootIDs = append(union.QS.Graph.RootIDs, g.QS.Graph.RootIDs...)
		iamUnion.Users = append(iamUnion.Users, iam.Users...)
		iamUnion.Profiles = append(iamUnion.Profiles, iam.Profiles...)
		iamUnion.Identities = append(iamUnion.Identities, iam.Identities...)
		cfg := config{archive: g.QS.Graph.ArchiveDirectory, scopeDir: in.ScopeDirectory, selector: in.Selector.File, selectorSHA: in.Selector.SHA}
		b.index, _, e = readGraphInputs(cfg, g, protection)
		if e != nil {
			return fail(e)
		}
		if _, e = graphClearanceRead(in.Selector, 16<<20, nil, guard); e != nil {
			return fail(e)
		}
		if _, e = graphClearanceRead(graphClearanceFile{filepath.Join(in.ScopeDirectory, "graph-index.json"), g.QS.Graph.IndexSHA}, 16<<20, nil, guard); e != nil {
			return fail(e)
		}
		if e = privateDirectory(cfg.archive); e != nil {
			return fail(e)
		}
		if _, e = graphClearanceRead(graphClearanceFile{filepath.Join(cfg.archive, "material.json"), in.MaterialSHA}, 16<<20, &b.material, guard); e != nil {
			return fail(e)
		}
		if e = validateGraphArchive(b.material, g, in.Group.SHA); e != nil {
			return fail(e)
		}
		if e = readGraphCaptureProof(in.CaptureEvidence.File, in.CaptureEvidence.SHA, b.material, in.MaterialSHA); e != nil {
			return fail(e)
		}
		var capture graphCaptureExecution
		if _, e = graphClearanceRead(in.CaptureEvidence, 32<<10, &capture, guard); e != nil {
			return fail(e)
		}
		var captured actionReceipt
		if _, e = graphClearanceRead(graphClearanceFile{capture.ReceiptFile, capture.ReceiptSHA}, 64<<10, &captured, guard); e != nil {
			return fail(e)
		}
		var applied actionReceipt
		if _, e = graphClearanceRead(in.PartialApply, 64<<10, &applied, guard); e != nil {
			return fail(e)
		}
		if e = graphClearanceExecution(in.PartialApplyExecution, in.PartialApply, "mock_cohort_apply_execution", guard); e != nil {
			return fail(e)
		}
		if applied.Format != 1 || applied.Kind != "mock_cohort_qs_action_result" || applied.Action != "apply" || applied.Operation != g.Operation || applied.GroupSHA != in.Group.SHA || applied.Source != b.material.Source || applied.MaterialSHA != in.MaterialSHA || !applied.Complete || !applied.Released || applied.DryRun || applied.Whole || !applied.Pending || applied.Exclusions != 0 {
			return fail(errors.New("clearance requires actual complete released partial apply stdout"))
		}
		journal, e := graphClearanceRead(graphClearanceFile{filepath.Join(cfg.archive, "graph-journal.jsonl"), in.JournalSHA}, 128<<20, nil, guard)
		if e != nil {
			return fail(e)
		}
		if e = graphClearanceJournal(journal, b.material); e != nil {
			return fail(e)
		}
		// Successful capture and every acknowledged apply bind these exact original
		// shard hashes. Clearance reads only target keys, not restorable bodies.
		if e = graphFrozenArchiveHeaders(b.material, g, protection, b.index); e != nil {
			return fail(e)
		}
		freeze, e := graphClearanceRead(in.CurrentFreeze, 32<<10, nil, guard)
		if e != nil {
			return fail(e)
		}
		if e = validateFreeze(freeze, g, in.Group.SHA); e != nil {
			return fail(e)
		}
		frozen, e := frozenGraphReferences(in.ScopeDirectory, b.index, g.QS.Graph.RootIDs, false)
		if e != nil {
			return fail(e)
		}
		for scope, ids := range frozen.ids {
			if refs.ids[scope] == nil {
				refs.ids[scope] = map[string]bool{}
			}
			for id := range ids {
				refs.ids[scope][id] = true
			}
		}
		b.member = graphClearanceMember{in.Group.SHA, in.Owner.SHA, digest(parts["iam"]), digest(parts["qs"]), graphClearanceIAMCounts{len(iam.Users), len(iam.Profiles), len(iam.Identities)}, in.MaterialSHA, capture.ReceiptSHA, in.CaptureEvidence.SHA, in.PartialApply.SHA, in.PartialApplyExecution.SHA, in.JournalSHA, in.CurrentFreeze.SHA}
		batches = append(batches, b)
	}
	union.IAM = encode(iamUnion)
	if e := guard.full(); e != nil {
		return fail(e)
	}
	return manifest, batches, guard, union, refs, nil
}

func graphClearanceJournal(raw []byte, m graphArchive) error {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return errors.New("clearance cannot qualify missing or partial journal")
	}
	entries, e := readJournal(bytes.NewReader(raw))
	if e != nil {
		return e
	}
	latest := map[string]journalEntry{}
	expected := map[string]bool{}
	for _, ref := range m.Targets {
		expected[graphChunkID(ref)] = true
	}
	for _, entry := range entries {
		if !expected[entry.Chunk] || !hashPattern.MatchString(entry.ApprovalSHA) || entry.Action != "apply" && entry.Action != "restore" {
			return errors.New("clearance journal crosses exact archive authority")
		}
		latest[entry.Chunk] = entry
	}
	for chunk := range expected {
		entry, found := latest[chunk]
		if !found || entry.Action != "apply" || entry.State != "committed" && entry.State != "reconciled_absent" {
			return errors.New("clearance needs known acknowledged apply for every exact chunk")
		}
	}
	return nil
}
func graphClearanceTargets(ctx context.Context, s graphBatchProvider, b graphClearanceBatch) error {
	ctx = graphFrozenRuleDeleteContext(ctx, true)
	for _, ref := range b.material.Targets {
		records, e := readGraphFrozenKeys(b.material.Graph.ArchiveDirectory, b.material, ref)
		if e != nil {
			return e
		}
		absent, _, e := graphVerificationState(ctx, s, records, false, false)
		if e != nil {
			return e
		}
		if !absent {
			return errors.New("clearance exact archived targets remain")
		}
	}
	return protectGraph(ctx, s, b.material, b.material.Graph.ArchiveDirectory)
}
func runGraphClearance(c config, out io.Writer) error {
	started := time.Now().UTC()
	ctx, cancel := context.WithDeadline(context.Background(), started.Add(c.timeout))
	defer cancel()
	manifest, batches, guard, union, refs, e := graphClearanceLoad(c)
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	live, release, e := connectWithWorkers(ctx, union.QS.Binding, c.workers)
	if e != nil {
		return e
	}
	s := newGraphStore(live)
	check := func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := guard.full(); e != nil {
			return e
		}
		return live.identity(ctx, union.QS.Binding)
	}
	observe := func() error {
		if e := check(); e != nil {
			return e
		}
		if e := graphRelationshipReferences(graphFrozenRuleDeleteContext(ctx, true), newLeanReader(s), union, refs, false); e != nil {
			return e
		}
		for _, batch := range batches {
			if e := graphClearanceTargets(ctx, s, batch); e != nil {
				return e
			}
		}
		return check()
	}
	e = errors.Join(observe(), release(), ctx.Err())
	if e != nil {
		return e
	}
	if e = guard.full(); e != nil {
		return e
	}
	completed := time.Now().UTC()
	if e = ctx.Err(); e != nil {
		return e
	}
	members := make([]graphClearanceMember, len(batches))
	for i := range batches {
		members[i] = batches[i].member
	}
	receipt := graphClearanceReceipt{1, "mock_cohort_qs_clearance", "clearance", manifest.Operation, toolSource, c.clearanceManifestSHA, c.protectionSHA, union.QS.Binding, started, completed, started.Add(c.timeout), members, true, true, true, true, true}
	_, e = out.Write(append(encode(receipt), '\n'))
	return e
}
