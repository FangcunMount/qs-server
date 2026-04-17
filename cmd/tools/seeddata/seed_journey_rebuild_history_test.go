package main

import (
	"strings"
	"testing"
)

func TestJourneyHistoryRebuildAccessGrantRelationTypesSQL(t *testing.T) {
	got := journeyHistoryRebuildAccessGrantRelationTypesSQL()
	want := "'assigned','primary','attending','collaborator'"
	if got != want {
		t.Fatalf("unexpected access grant relation types sql: got=%s want=%s", got, want)
	}
}

func TestBuildJourneyHistoryEpisodeAttributionFromRelationSQL(t *testing.T) {
	sql := buildJourneyHistoryEpisodeAttributionFromRelationSQL(journeyHistoryRebuildTables())
	if !strings.Contains(sql, "e.clinician_id = COALESCE(e.clinician_id, matched.clinician_id)") {
		t.Fatalf("expected clinician attribution clause in sql, got=%s", sql)
	}
	if !strings.Contains(sql, "cr.relation_type IN ('assigned','primary','attending','collaborator')") {
		t.Fatalf("expected access grant relation filter in sql, got=%s", sql)
	}
}

func TestBuildJourneyHistoryCareTransferredSQL(t *testing.T) {
	sql := buildJourneyHistoryCareTransferredSQL(journeyHistoryRebuildTables())
	if !strings.Contains(sql, "cr.source_type = 'transfer'") {
		t.Fatalf("expected transfer source filter in sql, got=%s", sql)
	}
	if !strings.Contains(sql, "source_clinician_id") {
		t.Fatalf("expected source clinician projection in sql, got=%s", sql)
	}
}

func TestBuildJourneyHistoryAssessmentEpisodesSQL(t *testing.T) {
	sql := buildJourneyHistoryAssessmentEpisodesSQL(journeyHistoryRebuildTables())
	if !strings.Contains(sql, "WHEN a.interpreted_at IS NOT NULL OR a.status = 'interpreted' THEN a.id") {
		t.Fatalf("expected report id derived from interpreted assessment in sql, got=%s", sql)
	}
	if !strings.Contains(sql, "a.answer_sheet_id <> 0") {
		t.Fatalf("expected answersheet guard in sql, got=%s", sql)
	}
}
