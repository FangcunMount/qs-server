package main

import "testing"

func TestReconcileAcceptsExactBatchFactsAndConcurrentTraffic(t *testing.T) {
	cfg := options{BatchID: "batch", OrgID: 9}
	base := baseline{Counts: []dailyFactCount{{Date: "2025-01-01", FactType: "report_generated", Count: 10}}}
	exact := []exactFactCount{{Date: "2025-01-01", FactType: "report_generated", Expected: 2, Matched: 2}}
	current := []dailyFactCount{{Date: "2025-01-01", FactType: "report_generated", Count: 13}}
	result := reconcile(cfg, base, exact, current)
	if !result.Complete || len(result.DeltaChecks) != 1 || result.DeltaChecks[0].UnattributedDelta != 1 {
		t.Fatalf("unexpected verification: %+v", result)
	}
}

func TestReconcileRejectsMissingExactFactEvenWhenGlobalDeltaIsLarge(t *testing.T) {
	cfg := options{BatchID: "batch", OrgID: 9}
	exact := []exactFactCount{{Date: "2025-01-01", FactType: "outcome_committed", Expected: 2, Matched: 1}}
	current := []dailyFactCount{{Date: "2025-01-01", FactType: "outcome_committed", Count: 20}}
	result := reconcile(cfg, baseline{}, exact, current)
	if result.Complete || len(result.Problems) == 0 {
		t.Fatalf("missing exact fact was accepted: %+v", result)
	}
}

func TestAggregateCountsMergesFactTablesDeterministically(t *testing.T) {
	values := aggregateCounts([]dailyFactCount{
		{Date: "2025-01-02", FactType: "task_created", Count: 1},
		{Date: "2025-01-01", FactType: "entry_opened", Count: 2},
		{Date: "2025-01-01", FactType: "entry_opened", Count: 3},
	})
	if len(values) != 2 || values[0].Date != "2025-01-01" || values[0].Count != 5 {
		t.Fatalf("unexpected aggregate: %+v", values)
	}
}

func TestOptionsValidateNormalizesIdentityAndPaths(t *testing.T) {
	cfg := options{
		Mode: " capture-baseline ", MySQLDSN: " dsn ", OrgID: 7,
		From: " 2025-01-01 ", To: " 2026-07-27 ", OutputPath: " result.json ",
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() error = %v", err)
	}
	if cfg.Mode != "capture-baseline" || cfg.MySQLDSN != "dsn" || cfg.From != "2025-01-01" || cfg.To != "2026-07-27" || cfg.OutputPath != "result.json" {
		t.Fatalf("options not normalized: %+v", cfg)
	}
}
