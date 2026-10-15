package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Test-only diagnostics contain closed categories and typed numeric metadata.
// SQL text, error messages, account/connection values and dynamic types are
// never serialized, including when an otherwise known error is wrapped.
type fixtureErrorSummary struct {
	Stage      string `json:"stage"`
	Type       string `json:"type"`
	Errno      uint16 `json:"errno"`
	SQLState   string `json:"sqlstate"`
	LocalClass string `json:"local_class"`
}

func fixtureErrorJSON(stage string, err error) string {
	if stage != "new_recovery" && stage != "operator_lock" && stage != "repo_recover" {
		stage = "unknown"
	}
	summary := fixtureErrorSummary{Stage: stage, Type: "other", SQLState: "unknown", LocalClass: "unknown"}
	var typed *mysql.MySQLError
	switch {
	case errors.As(err, &typed):
		summary.Type, summary.Errno = "mysql", typed.Number
		if state := string(typed.SQLState[:]); regexp.MustCompile(`^[A-Z0-9]{5}$`).MatchString(state) {
			summary.SQLState = state
		}
	case errors.Is(err, context.DeadlineExceeded):
		summary.Type = "deadline"
	case errors.Is(err, context.Canceled):
		summary.Type = "canceled"
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, gorm.ErrRecordNotFound):
		summary.Type = "not_found"
	case err != nil:
		known := map[string]string{
			"completed retirement and converged authorization required": "retirement_not_converged",
			"invalid operator recovery identity or version":             "recovery_identity_or_version",
			"recovery request and reason required":                      "recovery_request_or_reason",
			"operator recovery requires user exclusion":                 "user_exclusion_required",
			"retirement changed before recovery":                        "retirement_changed",
			"other operator membership prevents recovery":               "other_membership",
			"active company administrator required":                     "active_company_administrator_required",
			"administrator is retiring":                                 "administrator_retiring",
			"invalid original retirement facts":                         "original_retirement_facts",
			"operator changed during recovery":                          "operator_changed",
			"retirement changed during archival":                        "retirement_changed_during_archive",
		}
		if category, ok := known[err.Error()]; ok {
			summary.Type, summary.LocalClass = "local_rule", category
		}
	}
	raw, _ := json.Marshal(summary)
	return string(raw)
}

func TestFixtureErrorSummaryPreservesOnlySafeTypedMetadata(t *testing.T) {
	private := "SELECT private_body FROM secret_table; root:private@tcp(endpoint)"
	for _, tc := range []struct {
		name, stage string
		err         error
		want        fixtureErrorSummary
	}{
		{"mysql", "repo_recover", fmt.Errorf("%s: %w", private, &mysql.MySQLError{Number: 3024, SQLState: [5]byte{'H', 'Y', '0', '0', '0'}, Message: private}), fixtureErrorSummary{"repo_recover", "mysql", 3024, "HY000", "unknown"}},
		{"invalid_sqlstate", "operator_lock", &mysql.MySQLError{Number: 1064, SQLState: [5]byte{'x', '\n', 'y', 0, 1}, Message: private}, fixtureErrorSummary{"operator_lock", "mysql", 1064, "unknown", "unknown"}},
		{"known_local", "repo_recover", errors.New("active company administrator required"), fixtureErrorSummary{"repo_recover", "local_rule", 0, "unknown", "active_company_administrator_required"}},
		{"domain", "new_recovery", errors.New("invalid operator recovery identity or version"), fixtureErrorSummary{"new_recovery", "local_rule", 0, "unknown", "recovery_identity_or_version"}},
		{"deadline", "operator_lock", fmt.Errorf("%s: %w", private, context.DeadlineExceeded), fixtureErrorSummary{"operator_lock", "deadline", 0, "unknown", "unknown"}},
		{"not_found", "repo_recover", gorm.ErrRecordNotFound, fixtureErrorSummary{"repo_recover", "not_found", 0, "unknown", "unknown"}},
		{"unknown", "repo_recover", errors.New(private), fixtureErrorSummary{"repo_recover", "other", 0, "unknown", "unknown"}},
		{"unknown_stage", private, errors.New(private), fixtureErrorSummary{"unknown", "other", 0, "unknown", "unknown"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fixtureErrorJSON(tc.stage, tc.err)
			var got fixtureErrorSummary
			if strictDecode([]byte(raw), &got) != nil || got != tc.want || strings.Contains(raw, private) || strings.Contains(raw, "secret_table") || strings.Contains(raw, "private_body") {
				t.Fatal("unsafe or incomplete fixture error summary")
			}
		})
	}
}
