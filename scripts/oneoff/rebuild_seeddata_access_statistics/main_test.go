package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildInsertInferredResolveLogsUsesStableOffset(t *testing.T) {
	cfg := config{orgID: 1}
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0)

	stmt := buildInsertInferredResolveLogs(cfg, start, end)
	if !strings.Contains(stmt.Query, "8000000000000000000 + l.id") {
		t.Fatalf("expected stable resolve id offset in query: %s", stmt.Query)
	}
	if !strings.Contains(stmt.Query, "l.org_id = ?") {
		t.Fatalf("expected org predicate in query: %s", stmt.Query)
	}
	if len(stmt.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(stmt.Args))
	}
}

func TestFootprintCountSQLStripsUpsertClause(t *testing.T) {
	cfg := config{orgID: 1}
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0)

	insertSQL := buildEntryOpenedFootprintInsert(cfg, start, end).Query
	countSQL := footprintCountSQL(insertSQL)
	if strings.Contains(countSQL, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("count sql should not contain upsert clause: %s", countSQL)
	}
	if !strings.HasPrefix(countSQL, "SELECT COUNT(*) FROM assessment_entry_resolve_log") {
		t.Fatalf("unexpected count sql: %s", countSQL)
	}
}

func TestBuildInferredManualRelationScopeTargetsManualSource(t *testing.T) {
	cfg := config{orgID: 1, inferredTesteeCreated: true, testeeSourceRaw: "daily_simulation"}
	query := buildInferredManualRelationScopeInsert(cfg)
	for _, fragment := range []string{
		"'inferred_manual'",
		"cr.source_type IN ('manual', 'import')",
		"FROM assessment_entry",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("expected %q in manual relation scope query: %s", fragment, query)
		}
	}
	if strings.Contains(query, "t.source IN") {
		t.Fatalf("manual relation scope must not filter testee.source: %s", query)
	}
}

func TestAppendOrgPredicateAllOrgs(t *testing.T) {
	cfg := config{allOrgs: true}
	query, args := appendOrgPredicate("WHERE 1=1", nil, cfg, "l")
	if strings.Contains(query, "org_id = ?") {
		t.Fatalf("all orgs should not add org predicate: %s", query)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}
