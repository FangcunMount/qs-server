package main

import (
	"context"
	"errors"
	"net"
	"os"
	"regexp"

	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
)

// Diagnostics are a closed vocabulary. Driver text, addresses, paths and input
// JSON are never serialized, even when joined to an error from our own guards.
type phaseFailure struct {
	phase string
	cause error
}

func (e *phaseFailure) Error() string { return "mock cleanup phase failed" }
func (e *phaseFailure) Unwrap() error { return e.cause }
func safePhase(phase string, e error) error {
	if e == nil {
		return nil
	}
	return &phaseFailure{phase: phase, cause: e}
}

type safeDriverFailure struct {
	Kind     string `json:"kind"`
	Code     int64  `json:"code"`
	SQLState string `json:"sql_state,omitempty"`
	Name     string `json:"name,omitempty"`
}
type safeFailureReport struct {
	Format           int                 `json:"format_version"`
	Kind             string              `json:"kind"`
	Phases           []string            `json:"phases"`
	Codes            []string            `json:"codes"`
	Drivers          []safeDriverFailure `json:"drivers"`
	NetworkFailure   bool                `json:"network_failure"`
	NetworkTimeout   bool                `json:"network_timeout"`
	Canceled         bool                `json:"context_canceled"`
	Deadline         bool                `json:"deadline_exceeded"`
	FileNotFound     bool                `json:"file_not_found"`
	PermissionDenied bool                `json:"permission_denied"`
	Truncated        bool                `json:"error_chain_truncated"`
}

var diagnosticToken = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
var diagnosticSQLState = regexp.MustCompile(`^[A-Z0-9]{5}$`)
var diagnosticOwnCodes = map[string]string{
	"explicit raw SHA256 required":                                          "private_file_hash_required",
	"archive must be owned private 0700 directory":                          "private_directory_permissions",
	"sensitive archive must stay outside Git worktrees":                     "private_directory_in_git",
	"symlink archive path rejected":                                         "private_directory_symlink",
	"file read bound failed":                                                "private_file_bound",
	"private MySQL DSN file required":                                       "mysql_configuration_file",
	"private Mongo URI file required":                                       "mongo_configuration_file",
	"invalid private MySQL configuration":                                   "mysql_configuration_invalid",
	"DSN database/multi-statement mismatch":                                 "mysql_configuration_scope",
	"SQL identity changed":                                                  "sql_identity_changed",
	"clean SQL101 required":                                                 "sql_version_not_clean_101",
	"known Mongo primary replica set required":                              "mongo_primary_unknown",
	"invalid replica host set":                                              "mongo_replica_hosts_invalid",
	"mongo identity changed":                                                "mongo_identity_changed",
	"clean Mongo39 required":                                                "mongo_version_not_clean_39",
	"one Mongo migration ledger required":                                   "mongo_ledger_count",
	"same-database cleanup executor gate unavailable":                       "sql_executor_gate_unavailable",
	"SQL serialization gate release unknown":                                "sql_executor_release_unknown",
	"protected original object is missing":                                  "protected_original_missing",
	"SQL object not in fixed read catalog":                                  "sql_protection_catalog",
	"mongo object not in fixed read catalog":                                "mongo_protection_catalog",
	"complete actual physical primary key required":                         "sql_primary_key_incomplete",
	"caller key type cannot coerce the actual SQL primary key":              "sql_primary_key_type",
	"entire actual physical primary key is required":                        "sql_primary_key_scope",
	"selected SQL key differs from exact original key bytes":                "sql_primary_key_alias",
	"physical primary key selected multiple rows":                           "sql_primary_key_multiple",
	"mongo namespace/UUID must exist":                                       "mongo_namespace_missing",
	"mongo query matched a different physical typed _id":                    "mongo_primary_key_alias",
	"readonly whole-cohort discovery does not accept data-action authority": "readonly_authority_invalid",
	"selector protection original hash differs":                             "selector_protection_hash",
	"invalid exact cohort selector authority":                               "selector_authority_invalid",
	"explicit clean 101/39 identities required":                             "selector_baseline_invalid",
	"confirmed cohort source and evidence required":                         "selector_owner_evidence_invalid",
	"explicit shared system/clack protection required":                      "selector_protection_incomplete",
	"explicit root intersects protected testee":                             "selector_protected_root",
	"protected testee must use actual unsigned PK type":                     "selector_protected_root_type",
	"table outside fixed cohort graph":                                      "graph_sql_catalog",
	"actual primary key required for graph namespace":                       "graph_primary_key_missing",
	"unsupported actual graph primary key":                                  "graph_primary_key_type",
	"collection outside fixed cohort graph":                                 "graph_mongo_catalog",
	"graph namespace/UUID must exist":                                       "graph_mongo_namespace_missing",
	"one or more frozen roots failed source/org/window verification":        "graph_root_scope_changed",
	"explicit root no longer belongs to owner-confirmed source":             "graph_root_source_changed",
	"protection closure drifted during readonly discovery":                  "protection_drift",
	"absolute private archive path required":                                "private_path_invalid",
	"private owned single-link bounded file required":                       "private_file_permissions",
	"input raw hash changed":                                                "private_file_hash",
	"duplicate JSON key":                                                    "json_duplicate",
	"trailing JSON":                                                         "json_trailing",
	"nonliteral, missing or null JSON member":                               "json_schema",
}

func safeFailure(root error) safeFailureReport {
	r := safeFailureReport{Format: 1, Kind: "mock_cohort_safe_failure", Phases: []string{}, Codes: []string{}, Drivers: []safeDriverFailure{}, Canceled: errors.Is(root, context.Canceled), Deadline: errors.Is(root, context.DeadlineExceeded)}
	seen := map[string]bool{}
	add := func(dst *[]string, s string) {
		if !seen[s] {
			seen[s] = true
			*dst = append(*dst, s)
		}
	}
	r.FileNotFound = errors.Is(root, os.ErrNotExist)
	r.PermissionDenied = errors.Is(root, os.ErrPermission)
	pending := []error{root}
	for n := 0; len(pending) > 0 && n < 64; n++ {
		e := pending[0]
		pending = pending[1:]
		if e == nil {
			continue
		}
		switch v := e.(type) {
		case *phaseFailure:
			add(&r.Phases, v.phase)
		case *mysqldriver.MySQLError:
			d := safeDriverFailure{Kind: "mysql", Code: int64(v.Number)}
			state := string(v.SQLState[:])
			if diagnosticSQLState.MatchString(state) {
				d.SQLState = state
			}
			r.Drivers = append(r.Drivers, d)
		case mongo.CommandError:
			d := safeDriverFailure{Kind: "mongo_command", Code: int64(v.Code)}
			if diagnosticToken.MatchString(v.Name) {
				d.Name = v.Name
			}
			r.Drivers = append(r.Drivers, d)
		}
		if v, ok := e.(net.Error); ok {
			r.NetworkFailure = true
			r.NetworkTimeout = r.NetworkTimeout || v.Timeout()
		}
		if code, ok := diagnosticOwnCodes[e.Error()]; ok {
			add(&r.Codes, code)
		}
		if many, ok := e.(interface{ Unwrap() []error }); ok {
			pending = append(pending, many.Unwrap()...)
		} else if one, ok := e.(interface{ Unwrap() error }); ok {
			pending = append(pending, one.Unwrap())
		}
	}
	r.Truncated = len(pending) > 0
	if len(r.Codes) == 0 && len(r.Drivers) == 0 && !r.NetworkFailure && !r.Canceled && !r.Deadline {
		r.Codes = append(r.Codes, "unclassified_failure")
	}
	return r
}
