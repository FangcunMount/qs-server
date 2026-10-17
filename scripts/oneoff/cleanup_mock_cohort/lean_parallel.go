package main

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
)

type leanReadJob struct {
	store, name, where string
	args               []any
	filter             bson.D
	ids                []string
}
type leanReadResult struct {
	r   record
	job leanReadJob
	err error
}

func readLeanJobs(ctx context.Context, reader graphReader, jobs []leanReadJob, emit func(record, leanReadJob) error) error {
	if len(jobs) == 0 {
		return ctx.Err()
	}
	actual, ok := reader.(*leanGraphReader)
	if !ok {
		return runLeanReadPool(ctx, 1, jobs, func() graphReader { return reader }, nil, emit)
	}
	workers := actual.store.live.workers
	if workers == 0 {
		workers = 1
	}
	if workers < 1 || workers > 8 {
		return errors.New("lean read workers outside fixed one-to-eight bound")
	}
	definitions := map[string][]byte{}
	// The original reader owns the metadata set used by the final independent
	// re-observation. Worker readers have separate maps and cannot race this map.
	for _, job := range jobs {
		key := job.store + "/" + job.name
		if _, ok := definitions[key]; ok {
			continue
		}
		definition, e := leanJobDefinition(ctx, actual, job)
		if e != nil {
			return e
		}
		definitions[key] = definition
	}
	makeReader := func() graphReader { return newLeanReader(newGraphStore(actual.store.live)) }
	check := func(ctx context.Context, r graphReader, job leanReadJob) error {
		definition, e := leanJobDefinition(ctx, r.(*leanGraphReader), job)
		if e != nil {
			return e
		}
		if !bytes.Equal(definitions[job.store+"/"+job.name], definition) {
			return errors.New("worker technical namespace definition drifted")
		}
		return nil
	}
	return runLeanReadPool(ctx, workers, jobs, makeReader, check, emit)
}
func leanJobDefinition(ctx context.Context, r *leanGraphReader, job leanReadJob) ([]byte, error) {
	if job.store == "mysql" {
		shape, e := r.store.shape(ctx, job.name)
		return shape.definition, e
	}
	if job.store == "mongo" {
		return r.store.mongoDefinition(ctx, job.name)
	}
	return nil, errors.New("lean read job has unknown store")
}

// Each worker holds at most the existing 1000-row/64MiB SQL page or one Mongo
// cursor batch. The bounded result channel carries only technical records. All
// ownership maps, holds, counts and fsynced scope writes run in this caller.
func runLeanReadPool(ctx context.Context, workers int, jobs []leanReadJob, makeReader func() graphReader, check func(context.Context, graphReader, leanReadJob) error, emit func(record, leanReadJob) error) error {
	if workers < 1 || workers > 8 {
		return errors.New("lean read worker bound invalid")
	}
	if len(jobs) == 0 {
		return ctx.Err()
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	queue := make(chan leanReadJob)
	results := make(chan leanReadResult, workers)
	go func() {
		defer close(queue)
		for _, job := range jobs {
			select {
			case queue <- job:
			case <-workCtx.Done():
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for n := 0; n < workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader := makeReader()
			for job := range queue {
				if workCtx.Err() != nil {
					return
				}
				var e error
				if check != nil {
					e = check(workCtx, reader, job)
				}
				if e == nil {
					send := func(r record) error {
						select {
						case results <- leanReadResult{r: r, job: job}:
							return nil
						case <-workCtx.Done():
							return workCtx.Err()
						}
					}
					switch job.store {
					case "mysql":
						if actual, ok := reader.(*leanGraphReader); ok && leanStatisticsCursorTable(job.name) {
							e = actual.sqlOneCursor(workCtx, job.name, job.where, job.args, send)
							break
						}
						e = reader.sqlPages(workCtx, job.name, job.where, job.args, send)
					case "mongo":
						e = reader.mongoPages(workCtx, job.name, job.filter, send)
					default:
						e = errors.New("lean read job has unknown store")
					}
				}
				if e != nil {
					results <- leanReadResult{job: job, err: e}
					return
				}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	var first error
	for result := range results {
		if first != nil {
			continue
		}
		if result.err != nil {
			first = result.err
			cancel()
			continue
		}
		if e := emit(result.r, result.job); e != nil {
			first = e
			cancel()
		}
	}
	if first != nil {
		return first
	}
	return ctx.Err()
}
