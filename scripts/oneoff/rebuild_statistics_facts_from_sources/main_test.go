package main

import (
	"strings"
	"testing"
)

func TestAssessmentSourceUsesEvaluatedCompatibilityProjection(t *testing.T) {
	query, _ := assessmentSourceSQL(config{orgID: 1})
	if strings.Contains(query, "interpreted_at") || strings.Contains(query, "assessment_score") {
		t.Fatalf("assessment source uses retired compatibility storage: %s", query)
	}
	for _, token := range []string{"base.evaluated_at AS report_at"} {
		if !strings.Contains(query, token) {
			t.Fatalf("assessment source must contain %q: %s", token, query)
		}
	}
}
