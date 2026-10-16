package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

func graphInputAuthority(c config, g group) error {
	raw, e := privateRead(c.group, 256*1024*1024)
	if e != nil {
		return e
	}
	if digest(raw) != c.groupSHA {
		return errors.New("group original changed")
	}
	owner, e := privateRead(c.owner, 32*1024)
	if e != nil {
		return e
	}
	if e = validateOwner(raw, owner, g); e != nil {
		return e
	}
	freeze, e := privateRead(c.freeze, 32*1024)
	if e != nil {
		return e
	}
	if digest(freeze) != c.freezeSHA {
		return errors.New("freeze original changed")
	}
	return validateFreeze(freeze, g, c.groupSHA)
}
func graphStaticAuthority(c config, g group, p protected) (graphIndex, cohortSelector, error) {
	index, selector, e := readGraphInputs(c, g, p)
	if e != nil {
		return index, selector, e
	}
	if !sourcePattern.MatchString(toolSource) {
		return index, selector, errors.New("full graph tool must bind exact compiled source")
	}
	if e = graphInputAuthority(c, g); e != nil {
		return index, selector, e
	}
	if e = privateDirectory(c.archive); e != nil {
		return index, selector, e
	}
	return index, selector, nil
}
func runGraphAction(c config, g group, p protected, out io.Writer) error {
	index, selector, e := graphStaticAuthority(c, g, p)
	if e != nil {
		return e
	}
	if c.action == "plan" {
		return graphEmit(out, c, g, "", true, false)
	}
	var m graphArchive
	var a graphApproval
	if c.action != "capture" {
		if e = readExact(filepath.Join(c.archive, "material.json"), c.materialSHA, 16*1024*1024, &m); e != nil {
			return e
		}
		if e = validateGraphArchive(m, g, c.groupSHA); e != nil {
			return e
		}
		if e = validateGraphCoverage(m, g, p, index, c.scopeDir, c.archive); e != nil {
			return e
		}
		if e = readGraphCaptureProof(c.captureEvidence, c.captureEvidenceSHA, m, c.materialSHA); e != nil {
			return e
		}
	}
	guard, e := newGraphInputGuard(c, g)
	if e != nil {
		return e
	}
	if c.execute {
		if e = readExact(c.approval, c.approvalSHA, 32*1024, &a); e != nil {
			return e
		}
		if e = graphCheckApproval(a, m, g, c.materialSHA, c.action, toolSource, time.Now()); e != nil {
			return e
		}
		if a.FreezeSHA != c.freezeSHA || a.SettlementSHA != c.freezeSHA {
			return errors.New("action approval must bind shared freeze original")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	if c.execute {
		var stop context.CancelFunc
		ctx, stop, e = graphActionContext(ctx, m, a, c.action == "restore")
		if e != nil {
			return e
		}
		defer stop()
	} else if c.action == "capture" {
		var stop context.CancelFunc
		ctx, stop, e = graphAbsoluteContext(ctx, *g.QS.Graph)
		if e != nil {
			return e
		}
		defer stop()
	} else {
		var stop context.CancelFunc
		ctx, stop, e = graphReadContext(ctx, m, c.materialSHA, c.original || c.action == "restore")
		if e != nil {
			return e
		}
		defer stop()
	}
	live, release, e := connect(ctx, g.QS.Binding)
	if e != nil {
		return e
	}
	store := newGraphStore(live)
	var journal *graphJournal
	finish := func(cause error) error {
		var je error
		if journal != nil {
			je = journal.close()
		}
		return errors.Join(cause, je, release(), ctx.Err())
	}
	authority := func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := graphForwardGuard(c.archive); e != nil {
			return e
		}
		if e := guard.check(); e != nil {
			return e
		}
		if e := graphFreezeAuthority(c, g); e != nil {
			return e
		}
		if e := readGraphCaptureProof(c.captureEvidence, c.captureEvidenceSHA, m, c.materialSHA); e != nil {
			return e
		}
		if c.execute {
			var current graphApproval
			if e := readExact(c.approval, c.approvalSHA, 32*1024, &current); e != nil {
				return e
			}
			if !reflect.DeepEqual(a, current) {
				return errors.New("approval original changed")
			}
			if e := graphCheckApproval(a, m, g, c.materialSHA, c.action, toolSource, time.Now()); e != nil {
				return e
			}
		}
		return live.identity(ctx, g.QS.Binding)
	}
	claimAuthority := func() error {
		if e := guard.full(); e != nil {
			return e
		}
		return authority()
	}
	switch c.action {
	case "capture":
		entries, err := os.ReadDir(c.archive)
		if err != nil {
			return finish(err)
		}
		if len(entries) != 0 {
			return finish(errors.New("capture archive must be new and empty; failed originals are retained"))
		}
		// A readonly plan obtained while writers were active is not a freeze. Repeat
		// discovery under the independent stopped-writer receipt, and compare the
		// exact selected physical set before archiving or authorizing any deletion.
		before, err := graphProtectionSnapshots(ctx, store, p)
		if err != nil {
			return finish(err)
		}
		freshDir, err := graphFreshDirectory(c.archive, "frozen-scope")
		if err != nil {
			return finish(err)
		}
		freshSelector := selector
		freshSelector.TesteeIDs = append([]string{}, g.QS.Graph.RootIDs...)
		fresh, err := planCohort(ctx, store, freshSelector, p, freshDir, digest(encode(freshSelector)), toolSource)
		if err != nil {
			return finish(err)
		}
		if err = validateGraphHolds(freshDir, fresh, graphRootSet(g.QS.Graph.RootIDs)); err != nil {
			return finish(err)
		}
		if err = compareFrozenScope(c.scopeDir, index, g.QS.Graph.RootIDs, freshDir, fresh); err != nil {
			return finish(err)
		}
		if err = graphRelationshipCheck(ctx, store, g, index, c.scopeDir, true); err != nil {
			return finish(err)
		}
		m, e = captureGraph(ctx, store, g, p, index, c.scopeDir, c.archive, c.groupSHA, toolSource)
		if e == nil {
			e = validateGraphCoverage(m, g, p, index, c.scopeDir, c.archive)
		}
		if e == nil {
			e = protectGraph(ctx, store, m, c.archive)
		}
		if e == nil {
			var after []record
			after, e = graphProtectionSnapshots(ctx, store, p)
			if e == nil && !equalProtectionSnapshots(before, after) {
				e = errors.New("protected originals drifted during complete capture")
			}
		}
		if e == nil {
			e = graphInputAuthority(c, g)
		}
		if e == nil {
			_, e = store.verifyDefinitions(ctx)
		}
		if e == nil {
			e = live.identity(ctx, g.QS.Binding)
		}
		if e = finish(e); e != nil {
			return e
		}
		if e = guard.full(); e != nil {
			return e
		}
		// The material header alone is not successful execution evidence. All later
		// consumers also require an external executor's original stdout+actual exit0.
		raw := encode(m)
		if e = writeNew(filepath.Join(c.archive, "material.json"), raw); e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		return graphEmit(out, c, g, digest(raw), false, false)
	case "verify", "apply", "restore":
		journal, e = openGraphJournal(c.archive, c.execute)
		if e != nil {
			return finish(e)
		}
		if e = authority(); e == nil {
			if c.execute {
				e = graphRelationshipCheck(ctx, store, g, index, c.scopeDir, true)
			}
			if e == nil {
				if c.action == "verify" {
					e = graphVerify(ctx, store, m, c.archive, journal, c.original)
				} else if c.execute {
					e = mutateGraph(ctx, store, m, c.archive, journal, a, c.approvalSHA, c.action == "restore", authority, claimAuthority)
				} else {
					e = graphPrecheck(ctx, store, m, c.archive, journal, c.action == "restore")
				}
			}
		}
		if e == nil && (c.action == "verify" && !c.original || c.action == "apply" && c.execute) {
			e = graphNoRemainingReferences(ctx, store, g, index, c.scopeDir)
		}
		if e == nil {
			e = authority()
		}
		if e == nil {
			_, e = store.verifyDefinitions(ctx)
		}
		if e = finish(e); e != nil {
			return e
		}
		if e = guard.full(); e != nil {
			return e
		}
		// Recheck file authority after resource release; failure never emits success.
		if e = readGraphCaptureProof(c.captureEvidence, c.captureEvidenceSHA, m, c.materialSHA); e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		whole := c.action == "verify" && !c.original || c.action == "apply" && c.execute
		return graphEmit(out, c, g, c.materialSHA, (c.action == "apply" || c.action == "restore") && !c.execute, whole)
	default:
		return finish(errors.New("unknown full graph action"))
	}
}
func graphEmit(out io.Writer, c config, g group, sha string, dry, whole bool) error {
	r := actionReceipt{Format: 1, Kind: "mock_cohort_qs_action_result", Action: c.action, Operation: g.Operation, GroupSHA: c.groupSHA, Source: toolSource, MaterialSHA: sha, Complete: true, DryRun: dry, Whole: whole, Pending: false, Exclusions: 0, Released: true}
	_, e := out.Write(append(encode(r), '\n'))
	return e
}
func graphRootSet(ids []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}
func compareFrozenScope(originalDir string, index graphIndex, roots []string, freshDir string, fresh graphIndex) error {
	// Both writable identities and retained replay evidence belong to the exact
	// frozen plan. Changing a retention role is not an implicit scope update.
	type planned struct {
		fingerprint string
		rank        int
	}
	expected := map[string]planned{}
	visit := func(dir string, current graphIndex, fn func([]graphObject, int) error) error {
		for _, retained := range []bool{false, true} {
			if e := forGraphScope(dir, current, roots, retained, fn); e != nil {
				return e
			}
		}
		return nil
	}
	if e := visit(originalDir, index, func(page []graphObject, rank int) error {
		for _, o := range page {
			id := objectID(o.Object)
			if _, exists := expected[id]; exists {
				return errors.New("duplicate physical identity in original scope")
			}
			expected[id] = planned{o.Object.Fingerprint, rank}
		}
		return nil
	}); e != nil {
		return e
	}
	if e := visit(freshDir, fresh, func(page []graphObject, rank int) error {
		for _, o := range page {
			id := objectID(o.Object)
			original, exists := expected[id]
			if !exists || original.fingerprint != o.Object.Fingerprint || original.rank != rank {
				return errors.New("frozen graph differs from original plan; no automatic scope expansion")
			}
			delete(expected, id)
		}
		return nil
	}); e != nil {
		return e
	}
	if len(expected) != 0 {
		return errors.New("frozen graph lost planned objects")
	}
	return nil
}
