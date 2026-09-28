package main

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPublicOutputDoesNotExposeAssessmentIDs(t *testing.T) {
	result := report{AfterID: 100, UpperID: 104, NextAfterID: 101, Scanned: 1, Complete: true,
		Counts: map[string]int{"candidate_never_claimed": 1}, CandidateIDs: []uint64{101}}
	var output bytes.Buffer
	if err := encodeReport(&output, result, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "101") || strings.Contains(output.String(), "100") || strings.Contains(output.String(), "104") ||
		!strings.Contains(output.String(), "candidate_never_claimed") {
		t.Fatalf("unsafe public output: %s", output.String())
	}
}

func TestScanDistinguishesNeverClaimedFromClaimedAndCompleted(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	cutoff := time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("2026-09-29 04:00:00", uint64(100), uint64(104), 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "has_submitted_at", "matured", "has_model", "has_run"}).
			AddRow(101, "submitted", true, true, true, false).
			AddRow(102, "submitted", true, true, true, true).
			AddRow(103, "evaluated", true, true, true, true).
			AddRow(104, "submitted", true, false, true, false))
	mock.ExpectCommit()
	got, err := scan(t.Context(), db, config{afterID: 100, upperID: 104, cutoff: cutoff, maxRows: 4, includeIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || got.Scanned != 4 || got.NextAfterID != 104 || got.Counts["candidate_never_claimed"] != 1 ||
		got.Counts["run_present"] != 1 || got.Counts["evaluated"] != 1 || got.Counts["within_grace"] != 1 ||
		len(got.CandidateIDs) != 1 || got.CandidateIDs[0] != 101 {
		t.Fatalf("unexpected classification: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestScanStopsAtBudget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("2026-09-29 04:00:00", uint64(100), uint64(103), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "has_submitted_at", "matured", "has_model", "has_run"}).
			AddRow(101, "evaluated", true, true, true, true).
			AddRow(102, "submitted", true, true, true, false))
	mock.ExpectCommit()
	got, err := scan(context.Background(), db, config{afterID: 100, upperID: 103, cutoff: time.Date(2026, 9, 28, 20, 0, 0, 0, time.UTC), maxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete || got.Scanned != 1 || got.NextAfterID != 101 || len(got.CandidateIDs) != 0 {
		t.Fatalf("budget result: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestParseConfigRejectsUnsafeWindow(t *testing.T) {
	cutoff := time.Now().UTC().Add(-11 * time.Minute).Format(time.RFC3339)
	input := `{"mysql_dsn":"test@tcp(localhost:3306)/db"}`
	_, err := parseConfig([]string{"--connections-stdin", "--after-id=100", "--upper-id=100", "--submitted-before=" + cutoff}, strings.NewReader(input), &strings.Builder{})
	if err == nil {
		t.Fatal("empty window must fail")
	}
	_, err = parseConfig([]string{"--connections-stdin", "--after-id=100", "--upper-id=101", "--submitted-before=" + cutoff}, strings.NewReader(input), &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
}
