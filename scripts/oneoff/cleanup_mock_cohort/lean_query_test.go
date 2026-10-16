package main

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLeanStatisticsDirectedCursorSendsParentSetsOnceAndKeepsTechnicalRows(t *testing.T) {
	s, mock := graphBatchMock(t)
	graphBatchExpectShape(mock, "statistics_assessment_fact", "id", "bigint", "bigint unsigned")
	query := "SELECT x.`id`,x.`org_id`,x.`testee_id`,x.`filler_id`,x.`source_type`,x.`source_ref`,x.`stat_date`,x.`answersheet_id`,x.`assessment_id`,x.`outcome_id`,x.`report_id`,x.`plan_id`,x.`enrollment_id`,x.`task_id` FROM `statistics_assessment_fact` x WHERE x.`assessment_id` IN (SELECT fixed_ids.id FROM JSON_TABLE(?, '$[*]' COLUMNS(id BIGINT UNSIGNED PATH '$' ERROR ON ERROR)) fixed_ids) ORDER BY x.`id`"
	rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""), sqlmock.NewColumn("testee_id").OfType("BIGINT", ""), sqlmock.NewColumn("assessment_id").OfType("BIGINT", ""))
	for n := 1; n <= 2001; n++ {
		rows.AddRow(strconv.Itoa(n), nil, "100")
	}
	mock.ExpectQuery(query).WithArgs(`["100"]`).WillReturnRows(rows).RowsWillBeClosed()
	count := 0
	where := "x.`assessment_id` IN (SELECT fixed_ids.id FROM JSON_TABLE(?, '$[*]' COLUMNS(id BIGINT UNSIGNED PATH '$' ERROR ON ERROR)) fixed_ids)"
	e := newLeanReader(s).sqlOneCursor(t.Context(), "statistics_assessment_fact", where, []any{`["100"]`}, func(r record) error {
		count++
		if id, e := statisticPhysicalID(r.Object); e != nil || id != uint64(count) {
			t.Fatal("physical identity/order changed", r.Object, e)
		}
		if _, exists := graphSQLValue(r, "testee_id"); exists {
			t.Fatal("SQL NULL became an ownership identity")
		}
		if id, _ := graphSQLValue(r, "assessment_id"); id != "100" || string(r.Definition) != string(graphBatchTestDefinition()) {
			t.Fatal("technical parent or original namespace metadata changed")
		}
		return nil
	})
	if e != nil || count != 2001 {
		t.Fatal("single directed cursor lost pages", count, e)
	}
}

func TestLeanStatisticsDirectedCursorPreservesReadCallbackCloseAndDeadlineFailures(t *testing.T) {
	for _, kind := range []string{"query", "rows", "callback_and_close", "cancelled_callback"} {
		t.Run(kind, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			graphBatchExpectShape(mock, "statistics_access_fact", "id", "bigint", "bigint unsigned")
			query := "SELECT x.`id`,x.`org_id`,x.`testee_id`,x.`source_type`,x.`source_ref`,x.`stat_date`,x.`clinician_id`,x.`entry_id` FROM `statistics_access_fact` x WHERE directed_intersection ORDER BY x.`id`"
			primary, closeFailure := errors.New("synthetic original failure"), errors.New("synthetic cursor close failure")
			expect := mock.ExpectQuery(query)
			if kind == "query" {
				expect.WillReturnError(primary)
			} else {
				rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""))
				for n := 1; n <= 1002; n++ {
					rows.AddRow(strconv.Itoa(n))
				}
				if kind == "rows" {
					rows.RowError(1000, primary)
				}
				if kind == "callback_and_close" {
					rows.CloseError(closeFailure)
				}
				expect.WillReturnRows(rows).RowsWillBeClosed()
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			e := newLeanReader(s).sqlOneCursor(ctx, "statistics_access_fact", "directed_intersection", nil, func(record) error {
				if kind == "callback_and_close" {
					return primary
				}
				if kind == "cancelled_callback" {
					cancel()
				}
				return nil
			})
			if kind == "cancelled_callback" {
				if !errors.Is(e, context.Canceled) {
					t.Fatal("deadline/cancellation was not retained", e)
				}
			} else if !errors.Is(e, primary) {
				t.Fatal("original read or callback error was masked", e)
			}
			if kind == "callback_and_close" && !errors.Is(e, closeFailure) {
				t.Fatal("callback error masked uncertain cursor release", e)
			}
		})
	}
}
