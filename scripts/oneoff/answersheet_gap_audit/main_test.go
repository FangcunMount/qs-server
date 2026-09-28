package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/answersheetgap"
)

type scannerStep struct {
	page answersheetgap.Page
	err  error
}

type scriptedScanner struct {
	steps []scannerStep
	calls int
}

func (s *scriptedScanner) ScanPage(_ context.Context, _, _ uint64, _ time.Time, _ int) (answersheetgap.Page, error) {
	if s.calls >= len(s.steps) {
		return answersheetgap.Page{}, errors.New("unexpected extra scan")
	}
	step := s.steps[s.calls]
	s.calls++
	return step.page, step.err
}

func TestScanWindowPreservesCursorAndDoesNotCallPartialScanComplete(t *testing.T) {
	steps := []scannerStep{
		{page: answersheetgap.Page{Findings: []answersheetgap.Finding{
			{AnswerSheetID: 11, EventID: "event-11", Disposition: answersheetgap.Missing},
			{AnswerSheetID: 13, EventID: "event-13", Disposition: answersheetgap.Present},
		}, NextID: 13}},
		{page: answersheetgap.Page{Findings: []answersheetgap.Finding{
			{AnswerSheetID: 18, EventID: "event-18", Disposition: answersheetgap.ManualRequired},
		}, NextID: 18, Exhausted: true}},
	}
	cutoff := time.Date(2026, 9, 28, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	for _, tt := range []struct {
		name         string
		maxSheets    int
		wantComplete bool
		wantNext     uint64
		wantScanned  int
		wantCalls    int
	}{
		{name: "complete", maxSheets: 5, wantComplete: true, wantNext: 18, wantScanned: 3, wantCalls: 2},
		{name: "bounded partial", maxSheets: 2, wantNext: 13, wantScanned: 2, wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scanner := &scriptedScanner{steps: steps}
			result, err := scanWindow(t.Context(), scanner, config{afterID: 10, upperID: 20, batchSize: 2, maxSheets: tt.maxSheets}, cutoff)
			if err != nil {
				t.Fatal(err)
			}
			if result.Complete != tt.wantComplete || result.NextAfterID != tt.wantNext ||
				result.Scanned != tt.wantScanned || scanner.calls != tt.wantCalls {
				t.Fatalf("result=%+v calls=%d", result, scanner.calls)
			}
			if result.Counts[answersheetgap.Missing] != 1 || result.Counts[answersheetgap.Present] != 1 {
				t.Fatalf("counts=%v", result.Counts)
			}
		})
	}
}

func TestScanWindowRejectsUnstableCursorAndUnknownDisposition(t *testing.T) {
	for _, tt := range []struct {
		name string
		page answersheetgap.Page
	}{
		{name: "duplicate ID", page: answersheetgap.Page{Findings: []answersheetgap.Finding{
			{AnswerSheetID: 11, Disposition: answersheetgap.Present},
			{AnswerSheetID: 11, Disposition: answersheetgap.Present},
		}, NextID: 11}},
		{name: "unknown outcome", page: answersheetgap.Page{Findings: []answersheetgap.Finding{
			{AnswerSheetID: 11, Disposition: "future_state"},
		}, NextID: 11}},
		{name: "no progress", page: answersheetgap.Page{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scanner := &scriptedScanner{steps: []scannerStep{{page: tt.page}}}
			if _, err := scanWindow(t.Context(), scanner, config{afterID: 10, upperID: 20, batchSize: 2, maxSheets: 2}, time.Now()); err == nil {
				t.Fatal("invalid scan page must fail closed")
			}
		})
	}
}

func TestParseConfigRequiresExplicitFixedCutoffAndWindow(t *testing.T) {
	args := []string{
		"--mongo-uri=mongodb://localhost", "--mongo-db=qs", "--mysql-dsn=user@tcp(localhost:3306)/qs",
		"--after-id=10", "--upper-id=20", "--accepted-before=2026-09-28T12:00:00+08:00",
	}
	if _, _, err := parseConfig(args, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseConfig(args[:len(args)-1], io.Discard); err == nil {
		t.Fatal("missing acceptance cutoff must fail")
	}
	if _, _, err := parseConfig(append(args, "--upper-id=10"), io.Discard); err == nil {
		t.Fatal("empty ID window must fail")
	}
}
