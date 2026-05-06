package statistics

import (
	"strings"
	"testing"
)

func TestContentDailyInsertSQLGroupsByExpressions(t *testing.T) {
	contentTypeExpr := "CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END"
	contentCodeExpr := "COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code)"
	originTypeExpr := "COALESCE(origin_type, '')"

	for _, column := range []string{"created_at", "interpreted_at", "submitted_at", "failed_at"} {
		want := "GROUP BY org_id, " + contentTypeExpr + ", " + contentCodeExpr + ", " + originTypeExpr + ", DATE(" + column + ")"
		if !strings.Contains(contentDailyInsertSQL, want) {
			t.Fatalf("content daily SQL must group %s branch by expressions, not select aliases", column)
		}
	}

	if strings.Contains(contentDailyInsertSQL, "GROUP BY org_id, content_type, content_code, origin_type") {
		t.Fatal("content daily SQL must not group inner assessment branches by select aliases")
	}
}

func TestAccessFunnelInsertSQLUsesIntakeLogFacts(t *testing.T) {
	for _, token := range []string{
		"GREATEST(SUM(raw.entry_opened_count), SUM(raw.intake_confirmed_count)), SUM(raw.intake_confirmed_count)",
		"FROM assessment_entry_intake_log WHERE org_id = ? AND deleted_at IS NULL AND intake_at >= ? AND intake_at < ?",
		"FROM assessment_entry_intake_log WHERE org_id = ? AND deleted_at IS NULL AND testee_created = 1",
		"FROM assessment_entry_intake_log WHERE org_id = ? AND deleted_at IS NULL AND assignment_created = 1",
	} {
		if !strings.Contains(accessFunnelOrgInsertSQL, token) {
			t.Fatalf("access funnel SQL does not contain %q", token)
		}
	}
	for _, token := range []string{
		"FROM testee WHERE",
		"FROM clinician_relation WHERE",
	} {
		if strings.Contains(accessFunnelOrgInsertSQL, token) {
			t.Fatalf("access funnel SQL must use intake-log facts, found %q", token)
		}
	}
}
