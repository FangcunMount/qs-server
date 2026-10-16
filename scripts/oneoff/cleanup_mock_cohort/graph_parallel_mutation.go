package main

import (
	"context"
	"errors"
)

type graphMutationResult struct {
	entry journalEntry
	err   error
}

// The caller owns approval/preflight/window checks. Only this coordinator reads
// archives and writes the journal; workers perform one exact transaction each.
func graphMutateOrdered(ctx context.Context, s graphBatchProvider, m graphArchive, dir string, j *graphJournal, approvalSHA string, restore bool, authority func() error, workers int) error {
	if workers < 1 || workers > 8 || s == nil || j == nil || j.file == nil || authority == nil || !hashPattern.MatchString(approvalSHA) {
		return errors.New("bounded mutation workers and original writable journal authority required")
	}
	ordered := graphOrderedShards(m, restore)
	seen := map[string]bool{}
	for _, ref := range ordered {
		id := graphChunkID(ref)
		if seen[id] {
			return errors.New("duplicate mutation chunk cannot be dispatched")
		}
		seen[id] = true
		if prior, found := j.latest[id]; found && (prior.State == "intent" || prior.State == "unknown") {
			return errors.New("unknown batch requires read-only reconciliation before new approval")
		}
	}
	workCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	results := make(chan graphMutationResult, workers)
	var failure error
	active := 0
	stop := func(err error) {
		if err != nil {
			failure = errors.Join(failure, err)
			cancel(err)
		}
	}
	persist := func(result graphMutationResult) {
		active--
		result.entry.State = "committed"
		if result.err != nil {
			result.entry.State = "unknown"
			stop(errors.Join(errors.New("write outcome unknown; read-only reconciliation required"), result.err))
		}
		if err := j.append(result.entry); err != nil {
			stop(errors.Join(errors.New("write terminal journal unproven; do not resend"), err))
		}
	}
	for start := 0; start < len(ordered); {
		end := start + 1
		for end < len(ordered) && ordered[end].RestoreRank == ordered[start].RestoreRank && ordered[end].Store == ordered[start].Store && ordered[end].Name == ordered[start].Name {
			end++
		}
		phaseWorkers := workers
		if restore && ordered[start].Store == "mysql" {
			// Missing-PK locking reads can hold overlapping gaps even for disjoint
			// chunks. Serialize SQL inserts without weakening the transaction CAS.
			phaseWorkers = 1
		}
		next := start
		for next < end || active > 0 {
			// Observe completed failures before loading or dispatching another job.
			for active > 0 {
				select {
				case result := <-results:
					persist(result)
				default:
					goto drained
				}
			}
		drained:
			if failure == nil && workCtx.Err() == nil && next < end && active < phaseWorkers {
				if err := authority(); err != nil {
					stop(err)
					continue
				}
				ref := ordered[next]
				records, err := readGraphArchivePage(dir, m, ref)
				if err != nil {
					stop(err)
					continue
				}
				if prior, found := j.latest[graphChunkID(ref)]; found {
					absent, same, err := graphKnownChunkState(workCtx, s, records, prior)
					if err != nil {
						stop(err)
						continue
					}
					if restore && same || !restore && absent {
						next++
						continue
					}
				}
				if workCtx.Err() != nil {
					continue
				}
				entry := journalEntry{Action: "apply", Chunk: graphChunkID(ref), ApprovalSHA: approvalSHA, State: "intent"}
				if restore {
					entry.Action = "restore"
				}
				if err = j.append(entry); err != nil {
					stop(err)
					continue
				}
				// If another worker failed during fsync, retain a negative terminal
				// for this durable intent without issuing a new transaction.
				if workCtx.Err() != nil {
					entry.State = "unknown"
					stop(errors.Join(context.Cause(workCtx), j.append(entry)))
					continue
				}
				provider := s
				if real, ok := s.(*graphStore); ok {
					provider = newGraphStore(real.live) // worker-local metadata caches
				}
				active++
				next++
				go func(provider graphBatchProvider, records []record, entry journalEntry) {
					err := workCtx.Err()
					if err == nil {
						err = provider.mutateExactBatch(workCtx, records, restore)
					}
					if err != nil {
						cancel(err)
					}
					// Never discard an in-flight outcome, even after cancellation.
					results <- graphMutationResult{entry: entry, err: err}
				}(provider, records, entry)
				continue
			}
			if active == 0 {
				break
			}
			persist(<-results)
		}
		if failure != nil || workCtx.Err() != nil {
			return errors.Join(failure, context.Cause(workCtx))
		}
		// Every terminal has been fsynced before advancing the dependency phase.
		start = end
	}
	return context.Cause(workCtx)
}
