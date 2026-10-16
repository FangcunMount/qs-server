package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
)

// Split only read-only admission work before a durable write intent. A mutation
// error is never retried, even if its category resembles a size failure.
func graphReadBounded(ctx context.Context, s graphBatchProvider, objects []object, restorePreflight bool, fn func([]record) error) error {
	current, e := s.readExactBatch(ctx, objects)
	if errors.Is(e, errGraphBatchBudget) && len(objects) > 1 {
		n := len(objects) / 2
		if e = graphReadBounded(ctx, s, objects[:n], restorePreflight, fn); e != nil {
			return e
		}
		return graphReadBounded(ctx, s, objects[n:], restorePreflight, fn)
	}
	if e != nil {
		return e
	}
	if len(current) != len(objects) {
		return errors.New("exact batch result cardinality changed")
	}
	records := make([]record, len(current))
	for i, v := range current {
		if !v.Exists {
			return errors.New("capture target/protected original missing")
		}
		if objectID(v.Record.Object) != objectID(objects[i]) {
			return errors.New("exact batch changed requested physical identity")
		}
		if objects[i].Fingerprint != "" && fingerprint(v.Record) != objects[i].Fingerprint {
			return errors.New("scope original changed before capture")
		}
		records[i] = v.Record
	}
	if restorePreflight {
		if graphIsFrozenRuleDelete(ctx) {
			return graphFrozenCapturePreflight(ctx, s, records, fn)
		}
		e = s.preflightExactBatch(ctx, records, true)
		if errors.Is(e, errGraphBatchBudget) && len(objects) > 1 {
			n := len(objects) / 2
			if e = graphReadBounded(ctx, s, objects[:n], true, fn); e != nil {
				return e
			}
			return graphReadBounded(ctx, s, objects[n:], true, fn)
		}
		if e != nil {
			return e
		}
	}
	return fn(records)
}
func graphGuardObjects(p protected) []object {
	return sortedObjects(append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...))
}
func graphObjectBatches(objects []object, fn func([]object) error) error {
	for len(objects) > 0 {
		limit := 1000
		if objects[0].Store == "mongo" {
			limit = 500
		}
		n := 1
		for n < len(objects) && n < limit && objects[n].Store == objects[0].Store && objects[n].Name == objects[0].Name {
			n++
		}
		if e := fn(objects[:n]); e != nil {
			return e
		}
		objects = objects[n:]
	}
	return nil
}
func captureGraph(ctx context.Context, s graphBatchProvider, g group, p protected, index graphIndex, scopeDir, archiveDir, groupSHA, source string, workers int) (graphArchive, error) {
	m := graphArchive{Format: 1, Kind: "mock_cohort_graph_archive", Operation: g.Operation, GroupSHA: groupSHA, ProtectionSHA: g.ProtectedSHA, Source: source, Binding: g.QS.Binding, Graph: *g.QS.Graph, Namespaces: map[string][]byte{}, Targets: []graphShard{}, Retained: []graphShard{}, Guards: []graphShard{}, Counts: map[string]int64{}}
	if workers < 1 || workers > 8 {
		return m, errors.New("capture workers must be between one and eight")
	}
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type capturedPage struct {
		records []record
		written chan error
	}
	type captureJob struct {
		objects []object
		rank    int
		guard   bool
		pages   chan capturedPage
	}
	jobs := make(chan captureJob)
	ordered := make(chan captureJob, workers)
	slots := make(chan struct{}, workers)
	var joined sync.WaitGroup
	var failed sync.Once
	var cause error
	fail := func(e error) {
		if e != nil {
			failed.Do(func() { cause = e; cancel() })
		}
	}
	joined.Add(workers + 1)
	for range workers {
		go func() {
			defer joined.Done()
			reader := s
			if real, ok := s.(*graphStore); ok {
				reader = newGraphStore(real.live)
			}
			for job := range jobs {
				e := graphReadBounded(readCtx, reader, job.objects, job.rank > 0 && !job.guard, func(records []record) error {
					page := capturedPage{records: records, written: make(chan error, 1)}
					select {
					case job.pages <- page:
					case <-readCtx.Done():
						return readCtx.Err()
					}
					// The next readonly split cannot retain another raw page while
					// this one is being encoded/fsynced by the sole archive writer.
					select {
					case e := <-page.written:
						return e
					case <-readCtx.Done():
						return readCtx.Err()
					}
				})
				fail(e)
				close(job.pages)
			}
		}()
	}
	go func() {
		defer joined.Done()
		defer close(ordered)
		defer close(jobs)
		enqueue := func(objects []object, rank int, guard bool) error {
			select {
			case slots <- struct{}{}:
			case <-readCtx.Done():
				return readCtx.Err()
			}
			job := captureJob{objects: objects, rank: rank, guard: guard, pages: make(chan capturedPage)}
			select {
			case ordered <- job:
			case <-readCtx.Done():
				return readCtx.Err()
			}
			select {
			case jobs <- job:
				return nil
			case <-readCtx.Done():
				return readCtx.Err()
			}
		}
		visit := func(page []graphObject, rank int) error {
			objects := make([]object, len(page))
			for i, o := range page {
				objects[i] = o.Object
			}
			return enqueue(objects, rank, false)
		}
		e := forSelectedScope(scopeDir, index, m.Graph.RootIDs, visit)
		if e == nil {
			e = forRetainedScope(scopeDir, index, m.Graph.RootIDs, visit)
		}
		if e == nil {
			e = graphObjectBatches(graphGuardObjects(p), func(objects []object) error { return enqueue(objects, 0, true) })
		}
		fail(e)
	}()
write:
	for {
		var job captureJob
		var ok bool
		select {
		case job, ok = <-ordered:
			if !ok {
				break write
			}
		case <-readCtx.Done():
			break write
		}
	pages:
		for {
			select {
			case page, ok := <-job.pages:
				if !ok {
					<-slots
					break pages
				}
				e := archiveGraphPage(archiveDir, &m, page.records, job.rank, job.guard)
				page.written <- e
				if e != nil {
					fail(e)
					break write
				}
			case <-readCtx.Done():
				break write
			}
		}
	}
	joined.Wait()
	if cause != nil {
		return m, cause
	}
	return m, ctx.Err()
}

// Header references cannot expand, omit, reorder or duplicate a single scope
// object. Coverage is checked with only one raw archive page in memory.
func validateGraphCoverage(m graphArchive, g group, p protected, index graphIndex, scopeDir, dir string) error {
	return graphArchiveCoverage(m, g, p, index, scopeDir, dir, false)
}

// Fresh explicit capture already computed each complete preimage fingerprint,
// encoded its original page, fsynced it and compared the exact readback bytes.
// Repeat full scope/key coverage and raw SHA here without decoding those bodies
// and computing their same fingerprints again. Other consumers remain full.
func graphArchiveCoverage(m graphArchive, g group, p protected, index graphIndex, scopeDir, dir string, freshFrozen bool) error {
	readPage := func(ref graphShard) ([]record, error) {
		if freshFrozen {
			return readGraphFrozenKeys(dir, m, ref)
		}
		return readGraphArchivePage(dir, m, ref)
	}
	if e := validateGraphArchive(m, g, m.GroupSHA); e != nil {
		return e
	}
	counts := map[string]int64{}
	seen := map[string]bool{}
	usedDefinitions := map[string]bool{}
	checkScope := func(refs []graphShard, retained bool) error {
		nextRef, nextRecord := 0, 0
		var current []record
		visit := func(objects []graphObject, rank int) error {
			for _, o := range objects {
				for nextRecord == len(current) {
					if nextRef == len(refs) {
						return errors.New("archive omitted a selected scope object")
					}
					ref := refs[nextRef]
					file := graphArchiveFilename(false, nextRef)
					if retained {
						file = fmt.Sprintf("r-%08d.json", nextRef)
					}
					if ref.File != file || ref.RestoreRank != rank {
						return errors.New("archive scope ordinal/rank changed")
					}
					namespace := ref.Store + "/" + ref.Name
					if !hashPattern.MatchString(index.Definitions[namespace]) || index.Definitions[namespace] != digest(m.Namespaces[namespace]) {
						return errors.New("archive namespace differs from the typed-key plan")
					}
					var e error
					current, e = readPage(ref)
					if e != nil {
						return e
					}
					usedDefinitions[ref.Store+"/"+ref.Name] = true
					nextRef++
					nextRecord = 0
				}
				r := current[nextRecord]
				nextRecord++
				id := objectID(o.Object)
				if seen[id] || objectID(r.Object) != id || o.Object.Fingerprint != "" {
					return errors.New("archive exact typed-key scope coverage changed")
				}
				seen[id] = true
				if !retained {
					counts[o.Object.Store+"/"+o.Object.Name]++
				}
			}
			return nil
		}
		var e error
		if retained {
			e = forRetainedScope(scopeDir, index, m.Graph.RootIDs, visit)
		} else {
			e = forSelectedScope(scopeDir, index, m.Graph.RootIDs, visit)
		}
		if e != nil {
			return e
		}
		if nextRef != len(refs) || nextRecord != len(current) {
			return errors.New("archive contains extra scope records")
		}
		return nil
	}
	if e := checkScope(m.Targets, false); e != nil {
		return e
	}
	if e := checkScope(m.Retained, true); e != nil {
		return e
	}
	if !reflect.DeepEqual(counts, m.Counts) {
		return errors.New("archive target counts changed")
	}
	guards := graphGuardObjects(p)
	offset := 0
	for i, ref := range m.Guards {
		if ref.File != graphArchiveFilename(true, i) {
			return errors.New("archive guard ordinal changed")
		}
		records, e := readPage(ref)
		if e != nil {
			return e
		}
		for _, r := range records {
			if offset >= len(guards) || objectID(r.Object) != objectID(guards[offset]) {
				return errors.New("archive protection exact coverage changed")
			}
			if seen[objectID(r.Object)] {
				return errors.New("archive target/protection physical intersection")
			}
			seen[objectID(r.Object)] = true
			offset++
		}
		usedDefinitions[ref.Store+"/"+ref.Name] = true
	}
	for _, ref := range m.Targets {
		usedDefinitions[ref.Store+"/"+ref.Name] = true
		rank := graphSQLTables[ref.Name]
		if ref.Store == "mongo" {
			rank = graphMongoTables[ref.Name]
		}
		if rank <= 0 || ref.RestoreRank != rank {
			return errors.New("archive writable namespace/rank changed")
		}
	}
	if offset != len(guards) || offset != m.ProtectedCount || len(usedDefinitions) != len(m.Namespaces) {
		return errors.New("archive protection/namespace coverage incomplete")
	}
	return nil
}
func graphArchiveFilename(guard bool, i int) string {
	prefix := "t"
	if guard {
		prefix = "p"
	}
	return fmt.Sprintf("%s-%08d.json", prefix, i)
}
func protectGraph(ctx context.Context, s graphBatchProvider, m graphArchive, dir string) error {
	for _, ref := range append(append([]graphShard{}, m.Guards...), m.Retained...) {
		original, e := readGraphArchivePage(dir, m, ref)
		if e != nil {
			return e
		}
		objects := make([]object, len(original))
		for i, r := range original {
			objects[i] = r.Object
		}
		current, e := s.readExactBatch(ctx, objects)
		if e != nil {
			return e
		}
		if len(current) != len(original) {
			return errors.New("protection result count changed")
		}
		for i, r := range original {
			if !current[i].Exists || !sameRecord(current[i].Record, r) {
				return errors.New("protected original changed; all target components stop")
			}
		}
	}
	return nil
}
func graphChunkState(ctx context.Context, s graphBatchProvider, records []record) (allAbsent, allOriginal bool, err error) {
	objects := make([]object, len(records))
	for i, r := range records {
		objects[i] = r.Object
	}
	current, e := s.readExactBatch(ctx, objects)
	if e != nil {
		return false, false, e
	}
	if len(current) != len(records) {
		return false, false, errors.New("exact state cardinality changed")
	}
	allAbsent, allOriginal = true, true
	for i, v := range current {
		if !bytes.Equal(v.Record.Definition, records[i].Definition) {
			return false, false, errors.New("archive namespace/session changed")
		}
		allAbsent = allAbsent && !v.Exists
		allOriginal = allOriginal && v.Exists && sameRecord(v.Record, records[i])
		if v.Exists && !sameRecord(v.Record, records[i]) {
			return false, false, errors.New("retained/new fact conflicts with exact original")
		}
	}
	return allAbsent, allOriginal, nil
}
func graphChunkID(ref graphShard) string { return digest(encode(ref)) }
func graphVerify(ctx context.Context, s graphBatchProvider, m graphArchive, dir string, j *graphJournal, original bool) error {
	knownFast := graphIsFrozenRuleDelete(ctx) && !original && graphAllTargetsAcknowledged(m, j)
	if !knownFast {
		if e := protectGraph(ctx, s, m, dir); e != nil {
			return e
		}
	}
	complete := true
	for _, ref := range m.Targets {
		var records []record
		var e error
		if knownFast {
			records, e = readGraphFrozenKeys(dir, m, ref)
		} else {
			records, e = readGraphArchivePage(dir, m, ref)
		}
		if e != nil {
			return e
		}
		prior, found := j.latest[graphChunkID(ref)]
		absent, same, e := graphVerificationState(ctx, s, records, original, !knownFast)
		if e != nil {
			return e
		}
		if found && (prior.State == "intent" || prior.State == "unknown") {
			if !absent && !same {
				return errors.New("mixed unknown batch remains blocked; no replay")
			}
			prior.State = "reconciled_absent"
			if same {
				prior.State = "reconciled_original"
			}
			if e = j.append(prior); e != nil {
				return e
			}
		}
		complete = complete && (original && same || !original && absent)
	}
	if e := protectGraph(ctx, s, m, dir); e != nil {
		return e
	}
	if !complete {
		return errors.New("whole selected graph state is not verified")
	}
	return nil
}
func graphPrecheck(ctx context.Context, s graphBatchProvider, m graphArchive, dir string, j *graphJournal, restore bool) error {
	if e := protectGraph(ctx, s, m, dir); e != nil {
		return e
	}
	for _, ref := range graphOrderedShards(m, restore) {
		prior, found := j.latest[graphChunkID(ref)]
		if found && (prior.State == "intent" || prior.State == "unknown") {
			return errors.New("unknown batch requires read-only reconciliation before new approval")
		}
		// Entry already validated the complete immutable scope/archive coverage.
		// Fresh direct deletes recheck typed keys and metadata in each transaction.
		if graphIsFrozenRuleDelete(ctx) && !restore && !found {
			continue
		}
		var records []record
		var e error
		if graphIsFrozenRuleDelete(ctx) && !restore && found && graphAcknowledgedAbsent(prior) {
			records, e = readGraphFrozenKeys(dir, m, ref)
		} else {
			records, e = readGraphArchivePage(dir, m, ref)
		}
		if e != nil {
			return e
		}
		if e = s.preflightExactBatch(ctx, records, restore); e != nil {
			return e
		}
		// Restore must find every foreign replacement before any earlier chunk is
		// restored. New deletes instead use the provider's locked transaction CAS.
		if found {
			if _, _, e = graphKnownChunkState(ctx, s, records, prior); e != nil {
				return e
			}
		} else if restore {
			if _, _, e = graphChunkState(ctx, s, records); e != nil {
				return e
			}
		}
	}
	return nil
}

func graphKnownChunkState(ctx context.Context, s graphBatchProvider, records []record, prior journalEntry) (bool, bool, error) {
	var absent, original bool
	var e error
	if graphIsFrozenRuleDelete(ctx) && graphAcknowledgedAbsent(prior) {
		absent, original, e = graphVerificationState(ctx, s, records, false, false)
	} else {
		absent, original, e = graphChunkState(ctx, s, records)
	}
	if e != nil {
		return false, false, e
	}
	wantAbsent := prior.State == "reconciled_absent" || prior.State == "committed" && prior.Action == "apply"
	wantOriginal := prior.State == "reconciled_original" || prior.State == "committed" && prior.Action == "restore"
	if !wantAbsent && !wantOriginal || wantAbsent && !absent || wantOriginal && !original {
		return false, false, errors.New("acknowledged chunk state changed; no automatic replay")
	}
	return absent, original, nil
}

func mutateGraph(ctx context.Context, s graphBatchProvider, m graphArchive, dir string, j *graphJournal, a graphApproval, approvalSHA string, restore bool, authority, beforeClaim func() error, workers int) error {
	if workers < 1 || workers > 8 {
		return errors.New("mutation workers must be between one and eight")
	}
	if e := graphPrecheck(ctx, s, m, dir, j, restore); e != nil {
		return e
	}
	if e := beforeClaim(); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if restore {
		if e := graphRecoveryClaim(dir, m, a.ArchiveSHA, a); e != nil {
			return e
		}
	}
	if e := writeNew(filepath.Join(dir, "approval-"+approvalSHA+".consumed"), encode(a)); e != nil {
		return errors.New("approval already consumed or private claim failed")
	}
	if e := graphMutateOrdered(ctx, s, m, dir, j, approvalSHA, restore, authority, workers); e != nil {
		return e
	}
	if e := authority(); e != nil {
		return e
	}
	return graphVerify(ctx, s, m, dir, j, restore)
}

// All guards are fixed original namespace batches, not one query per row.
func graphProtectionSnapshots(ctx context.Context, s graphBatchProvider, p protected) ([]record, error) {
	out := []record{}
	e := graphObjectBatches(graphGuardObjects(p), func(objects []object) error {
		return graphReadBounded(ctx, s, objects, false, func(records []record) error { out = append(out, records...); return nil })
	})
	return out, e
}
func equalProtectionSnapshots(a, b []record) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameRecord(a[i], b[i]) {
			return false
		}
	}
	return true
}
func graphFreshDirectory(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	if e := os.Mkdir(p, 0700); e != nil {
		return "", e
	}
	return p, privateDirectory(p)
}
