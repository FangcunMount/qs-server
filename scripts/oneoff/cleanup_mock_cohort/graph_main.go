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

func runGraphPlan(c config, out io.Writer) error {
	if c.execute || c.original || c.group != "" || c.groupSHA != "" || c.archive != "" || c.materialSHA != "" || c.approval != "" || c.approvalSHA != "" || c.freeze != "" || c.freezeSHA != "" || c.owner != "" {
		return errors.New("readonly whole-cohort discovery does not accept data-action authority")
	}
	var selector cohortSelector
	var protection protected
	if e := readExact(c.selector, c.selectorSHA, 16*1024*1024, &selector); e != nil {
		return e
	}
	if e := readExact(c.protection, c.protectionSHA, 32*1024*1024, &protection); e != nil {
		return e
	}
	if selector.ProtectionSHA != c.protectionSHA {
		return errors.New("selector protection original hash differs")
	}
	if e := validateSelector(selector, protection); e != nil {
		return e
	}
	if e := privateDirectory(c.scopeDir); e != nil {
		return e
	}
	entries, e := os.ReadDir(c.scopeDir)
	if e != nil {
		return e
	}
	if len(entries) != 0 {
		return errors.New("new readonly scope directory must be empty; failed material is retained")
	}
	if c.output != "" && c.output != filepath.Join(c.scopeDir, "graph-index.json") {
		return errors.New("planner output has one fixed filename inside scope directory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	ctx = graphRuleBatchContext(ctx, c.ruleBatch)
	s, closeAll, e := connectWithWorkers(ctx, selector.Binding, c.workers)
	if e != nil {
		return e
	}
	graphStore := newGraphStore(s)
	leanReader := newLeanReader(graphStore)
	observedAt := time.Now().UTC()
	original, e := observeLeanProtection(ctx, leanReader, protection)
	if e != nil {
		return errors.Join(e, closeAll())
	}
	index, e := planCohort(ctx, leanReader, selector, protection, c.scopeDir, c.selectorSHA, toolSource)
	if e == nil {
		index.Definitions, e = graphStore.verifyDefinitions(ctx)
	}
	if e == nil {
		var current []object
		current, e = observeLeanProtection(ctx, leanReader, protection)
		if e == nil && !reflect.DeepEqual(original, current) {
			e = errors.New("protection closure drifted during readonly discovery")
		}
	}
	if e == nil {
		e = s.identity(ctx, selector.Binding)
	}
	e = errors.Join(e, closeAll(), ctx.Err())
	if e != nil {
		return e
	}
	index.Released = true
	if c.ruleBatch {
		index.RuleBatch = &graphRuleBatchObservation{Started: observedAt, Released: time.Now().UTC()}
	}
	if e = readOnlyGraphResult(filepath.Join(c.scopeDir, "graph-index.json"), index); e != nil {
		return e
	}
	receipt := actionReceipt{Format: 1, Kind: "mock_cohort_qs_action_result", Action: c.action, Operation: selector.Operation, GroupSHA: c.selectorSHA, Source: toolSource, MaterialSHA: digest(encode(index)), Complete: true, DryRun: true, Whole: false, Pending: true, Exclusions: len(index.Holds), Released: true}
	_, e = out.Write(append(encode(receipt), '\n'))
	return e
}
