package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

const fixtureUUID = "00112233-4455-6677-8899-aabbccddeeff"
const fixtureSecret = "fixture-private-marker-never-output"

var fixtureConnection = connection{Host: fixtureSecret, Port: 3306, Database: "fixture_db", User: "fixture_user", Password: fixtureSecret}

func fixtureBinding() binding {
	return binding{SHA: strings.Repeat("a", 40), RunID: "12345-1", Expected: hashText(fixtureConnection.Host, "3306", fixtureConnection.Database, fixtureUUID)}
}
func identities(id uint64, roles string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "uuid", "database", "version", "roles"}).AddRow(id, fixtureUUID, fixtureConnection.Database, "8.0.36", roles)
}
func grants(values ...string) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"grants"})
	for _, s := range values {
		r.AddRow(s)
	}
	return r
}
func mockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, m, e := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if e != nil {
		t.Fatal("mock_setup_failed")
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if m.ExpectationsWereMet() != nil {
			t.Error("query_contract_failed")
		}
		_ = db.Close()
	})
	return db, m
}
func positive() string { return "GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON *.* TO `fixture_user`@`%`" }
func assertPrivate(t *testing.T, r receipt) {
	t.Helper()
	raw, e := json.Marshal(r)
	if e != nil {
		t.Fatal("encode_failed")
	}
	if !r.Complete {
		for _, fact := range []*bool{r.CurrentGlobalSelect, r.CurrentGlobalShowView, r.CurrentGlobalTrigger, r.CurrentGlobalEvent, r.CurrentPartialRevocations} {
			if fact != nil {
				t.Fatal("incomplete_receipt_retained_grant_fact")
			}
		}
	}
	for _, bad := range []string{fixtureSecret, fixtureUUID, "fixture_user", "rds_superuser_role", "SHOW GRANTS", "NONE", "127.0.0.1"} {
		if bytes.Contains(raw, []byte(bad)) {
			t.Fatal("private_source_value_leaked")
		}
	}
}

func TestInactiveFixedRolePotentialDoesNotBecomeCurrent(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants("GRANT USAGE ON *.* TO `fixture_user`@`%`", "GRANT ALL PRIVILEGES ON `fixture_db`.* TO `fixture_user`@`%`", "GRANT `rds_superuser_role`@`%` TO `fixture_user`@`%`"))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
	m.ExpectQuery(rdsSQL).WillReturnRows(grants(positive(), "GRANT `rds_superuser_role`@`%` TO `fixture_user`@`%`"))
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if !r.Complete || r.CurrentUnrestricted || r.RDSAvailable == nil || !*r.RDSAvailable || r.RDSUnrestricted == nil || !*r.RDSUnrestricted || !r.DiagnosticOnly {
		t.Fatal("inactive_role_semantics_changed")
	}
	assertPrivate(t, r)
}
func TestCurrentOtherActiveRoleDoesNotEnterFixedRolePotential(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "`other_active_role`@`%`"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
	m.ExpectQuery(rdsSQL).WillReturnRows(grants("GRANT SELECT ON *.* TO `fixture_user`@`%`"))
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "`other_active_role`@`%`"))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if !r.Complete || !r.CurrentUnrestricted || r.RDSUnrestricted == nil || *r.RDSUnrestricted {
		t.Fatal("using_set_was_misnamed_current")
	}
	assertPrivate(t, r)
}
func TestOnlyCanonical3530MakesRoleUnavailable(t *testing.T) {
	hy000 := [5]byte{'H', 'Y', '0', '0', '0'}
	cases := []struct {
		name   string
		number uint16
		state  [5]byte
		absent bool
	}{
		{"canonical", 3530, hy000, true},
		{"missing_state", 3530, [5]byte{}, false},
		{"different_state", 3530, [5]byte{'4', '2', '0', '0', '0'}, false},
		{"other_role_error", 3523, hy000, false},
		{"access_error", 1045, hy000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, m := mockDB(t)
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
			m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
			m.ExpectQuery(rdsSQL).WillReturnError(&mysql.MySQLError{Number: tc.number, SQLState: tc.state, Message: fixtureSecret})
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
			if tc.absent {
				if !r.Complete || r.RDSAvailable == nil || *r.RDSAvailable || r.RDSUnrestricted != nil {
					t.Fatal("role_unavailable_contract_failed")
				}
			} else {
				if r.Complete || r.RDSAvailable != nil || r.RDSUnrestricted != nil || r.CurrentUnrestricted || r.ErrorCategory != "rds_role_query_failed" {
					t.Fatal("unknown_error_became_absence")
				}
			}
			assertPrivate(t, r)
		})
	}
}
func TestUnknownRoleQueryErrorIsPrivateAndIncomplete(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
	m.ExpectQuery(rdsSQL).WillReturnError(errors.New(fixtureSecret + " SHOW GRANTS " + fixtureUUID))
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if r.Complete || r.RDSAvailable != nil || r.CurrentUnrestricted || r.ErrorCategory != "rds_role_query_failed" {
		t.Fatal("unknown_role_error_not_closed")
	}
	assertPrivate(t, r)
}
func TestMalformedRevokeAndUnknownGrantRejectAllPositiveRows(t *testing.T) {
	for _, bad := range []string{"REVOKE SELECT ON `hidden`.* FROM missing_account", "GRANT BROKEN-DYNAMIC ON *.* TO `fixture_user`@`%`", "GRANT ROLE_ADMIN ON `fixture_db`.* TO `fixture_user`@`%`", positive() + "; SELECT 'injected'", "GRANT SELECT ON *.* TO `fixture_user`@`%` REQUIRE SSL"} {
		db, m := mockDB(t)
		m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
		m.ExpectQuery(currentSQL).WillReturnRows(grants(positive(), bad))
		m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
		r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
		if r.Complete || r.CurrentUnrestricted || r.RDSAvailable != nil || r.ErrorCategory != "current_grants_rejected" {
			t.Fatal("unknown_or_partial_grants_accepted")
		}
		assertPrivate(t, r)
	}
}
func TestPotentialPartialRevokeNeverQualifies(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
	m.ExpectQuery(rdsSQL).WillReturnRows(grants(positive(), "REVOKE SELECT ON `hidden`.* FROM `fixture_user`@`%`"))
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if !r.Complete || !r.CurrentUnrestricted || r.RDSAvailable == nil || !*r.RDSAvailable || r.RDSUnrestricted == nil || *r.RDSUnrestricted || r.ErrorCategory != "none" {
		t.Fatal("partial_role_revoke_accepted")
	}
	assertPrivate(t, r)
}

func TestCanonicalCurrentRevokeStillQueriesPotentialFixedRole(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive(), "REVOKE SELECT ON `hidden`.* FROM `fixture_user`@`%`"))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
	m.ExpectQuery(rdsSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if !r.Complete || r.CurrentUnrestricted || r.RDSAvailable == nil || !*r.RDSAvailable || r.RDSUnrestricted == nil || !*r.RDSUnrestricted || r.ErrorCategory != "none" {
		t.Fatal("known_restriction_blocked_potential_diagnosis")
	}
	assertPrivate(t, r)
}

func TestOfficialRDSRoleStyleStaticAndDynamicGrantRows(t *testing.T) {
	static := "GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, RELOAD, PROCESS, REFERENCES, INDEX, ALTER, SHOW DATABASES, CREATE TEMPORARY TABLES, LOCK TABLES, EXECUTE, REPLICATION SLAVE, REPLICATION CLIENT, CREATE VIEW, SHOW VIEW, CREATE ROUTINE, ALTER ROUTINE, CREATE USER, EVENT, TRIGGER, CREATE ROLE, DROP ROLE ON *.* TO `fixture_user`@`%` WITH GRANT OPTION"
	dynamic := "GRANT APPLICATION_PASSWORD_ADMIN, ROLE_ADMIN, SET_USER_ID, XA_RECOVER_ADMIN ON *.* TO `fixture_user`@`%` WITH GRANT OPTION"
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants("GRANT USAGE ON *.* TO `fixture_user`@`%`"))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
	m.ExpectQuery(rdsSQL).WillReturnRows(grants(static, dynamic, "GRANT `rds_superuser_role`@`%` TO `fixture_user`@`%`"))
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if !r.Complete || r.CurrentUnrestricted || r.RDSUnrestricted == nil || !*r.RDSUnrestricted {
		t.Fatal("standard_rds_role_format_rejected")
	}
	assertPrivate(t, r)
	if full, known := unrestricted([]string{dynamic}); !known || full {
		t.Fatal("dynamic_privilege_faked_static_visibility")
	}
}
func TestPinnedSessionOrActiveRoleChangeDiscardsPositiveResults(t *testing.T) {
	for _, last := range []struct {
		id    uint64
		roles string
	}{{11, "NONE"}, {10, "`new_role`@`%`"}} {
		db, m := mockDB(t)
		m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
		m.ExpectQuery(currentSQL).WillReturnRows(grants(positive()))
		m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
		m.ExpectQuery(rdsSQL).WillReturnRows(grants(positive()))
		m.ExpectQuery(identitySQL).WillReturnRows(identities(last.id, last.roles))
		r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
		if r.Complete || r.CurrentUnrestricted || r.RDSAvailable != nil || r.RDSUnrestricted != nil || r.AssignedRoles != nil || r.MandatoryRoles != nil || r.ErrorCategory != "session_identity_changed" {
			t.Fatal("session_change_left_positive_output")
		}
		assertPrivate(t, r)
	}
}
func TestTargetHashMismatchNeverReadsPrivileges(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	b := fixtureBinding()
	b.Expected = strings.Repeat("0", 64)
	r := probe(context.Background(), db, fixtureConnection, b)
	if r.Complete || r.SourceTargetHash != "" || r.ErrorCategory != "target_hash_mismatch" {
		t.Fatal("wrong_target_was_queried")
	}
	assertPrivate(t, r)
}
func TestDriverAndInvalidEnvironmentNeverLogSecrets(t *testing.T) {
	c := mysqlConfig(fixtureConnection)
	if c.MultiStatements || c.InterpolateParams || c.ReadTimeout != queryLimit || c.WriteTimeout != queryLimit || c.Timeout > queryLimit {
		t.Fatal("driver_boundary_changed")
	}
	if _, ok := c.Logger.(*mysql.NopLogger); !ok {
		t.Fatal("driver_logger_not_discard")
	}
	c.Logger.Print(fixtureSecret)
	env := map[string]string{"PROFILE_SOURCE_SHA": fixtureSecret, "PROFILE_RUN_ID": "12345-1", "PROFILE_EXPECTED_TARGET_HASH": strings.Repeat("0", 64), "MYSQL_PASSWORD": fixtureSecret}
	get := func(k string) string { return env[k] }
	var out bytes.Buffer
	if run(nil, get, &out) != 1 || strings.Contains(out.String(), fixtureSecret) {
		t.Fatal("invalid_env_or_secret_output")
	}
}

func assertNoRoleConclusions(t *testing.T, r receipt) {
	t.Helper()
	if r.Complete || r.CurrentUnrestricted || r.RDSAvailable != nil || r.RDSUnrestricted != nil || r.AssignedRoles != nil || r.MandatoryRoles != nil {
		t.Fatal("incomplete_receipt_retained_role_conclusion")
	}
	assertPrivate(t, r)
}

func TestRoleCensusPresenceDoesNotInferPrivilegeOrExposeNames(t *testing.T) {
	cases := []struct {
		name, roleRow, mandatory string
		assigned, required       bool
	}{
		{"none", "", "", false, false},
		{"assigned_only", "GRANT `" + fixtureSecret + ";query`@`%` TO `fixture_user`@`%`", "", true, false},
		{"mandatory_only", "", "`" + fixtureSecret + "`@`%`", false, true},
		{"both", "GRANT `first_role`@`%`,`" + fixtureSecret + "`@`%` TO `fixture_user`@`%` WITH ADMIN OPTION", "`" + fixtureSecret + "`@`%`", true, true},
		{"proxy_is_not_role_assignment", "GRANT PROXY ON `" + fixtureSecret + "`@`%` TO `fixture_user`@`%`", " ", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, m := mockDB(t)
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			rows := grants("GRANT USAGE ON *.* TO `fixture_user`@`%`")
			if tc.roleRow != "" {
				rows.AddRow(tc.roleRow)
			}
			m.ExpectQuery(currentSQL).WillReturnRows(rows)
			m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(tc.mandatory))
			m.ExpectQuery(rdsSQL).WillReturnError(&mysql.MySQLError{Number: roleNotGranted, SQLState: [5]byte{'H', 'Y', '0', '0', '0'}, Message: fixtureSecret})
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
			if !r.Complete || r.CurrentUnrestricted || r.AssignedRoles == nil || *r.AssignedRoles != tc.assigned || r.MandatoryRoles == nil || *r.MandatoryRoles != tc.required || r.RDSAvailable == nil || *r.RDSAvailable || r.RDSUnrestricted != nil {
				t.Fatal("census_presence_semantics_changed")
			}
			assertPrivate(t, r)
		})
	}
}

func TestMandatoryRoleCensusFailureClearsAllConclusions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		err   error
	}{
		{"query_error", nil, errors.New(fixtureSecret)},
		{"null", nil, nil},
		{"oversized", strings.Repeat("x", 65537), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, m := mockDB(t)
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			m.ExpectQuery(currentSQL).WillReturnRows(grants(positive(), "GRANT `"+fixtureSecret+"`@`%` TO `fixture_user`@`%`"))
			q := m.ExpectQuery(mandatorySQL)
			if tc.err != nil {
				q.WillReturnError(tc.err)
			} else {
				q.WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(tc.value))
			}
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
			if r.ErrorCategory != "mandatory_roles_query_failed" {
				t.Fatal("mandatory_failure_not_fixed")
			}
			assertNoRoleConclusions(t, r)
		})
	}
}

func TestMalformedRoleRowCannotProduceCensus(t *testing.T) {
	for _, bad := range []string{
		"GRANT `" + fixtureSecret + "`@`%` TO missing_account",
		"GRANT `role`@`%` TO `fixture_user`@`%`; SELECT '" + fixtureSecret + "'",
		"GRANT `role`@`%` TO `fixture_user`@`%` WITH ADMIN OPTION UNKNOWN",
	} {
		db, m := mockDB(t)
		m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
		m.ExpectQuery(currentSQL).WillReturnRows(grants(positive(), bad))
		m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
		r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
		if r.ErrorCategory != "current_grants_rejected" {
			t.Fatal("malformed_role_row_not_rejected")
		}
		assertNoRoleConclusions(t, r)
	}
}

func TestCensusIsClearedWhenFinalIdentityCannotBeRead(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive(), "GRANT `"+fixtureSecret+"`@`%` TO `fixture_user`@`%`"))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(fixtureSecret))
	m.ExpectQuery(rdsSQL).WillReturnRows(grants(positive()))
	m.ExpectQuery(identitySQL).WillReturnError(errors.New(fixtureSecret))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if r.ErrorCategory != "identity_final_failed" {
		t.Fatal("final_identity_failure_not_closed")
	}
	assertNoRoleConclusions(t, r)
}

func TestRejectedPotentialSyntaxClearsCensusAndAvailability(t *testing.T) {
	db, m := mockDB(t)
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	m.ExpectQuery(currentSQL).WillReturnRows(grants(positive(), "GRANT `"+fixtureSecret+"`@`%` TO `fixture_user`@`%`"))
	m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(fixtureSecret))
	m.ExpectQuery(rdsSQL).WillReturnRows(grants(positive(), "UNKNOWN "+fixtureSecret))
	m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
	r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
	if r.ErrorCategory != "rds_role_grants_rejected" {
		t.Fatal("potential_syntax_failure_not_closed")
	}
	assertNoRoleConclusions(t, r)
}

func TestCurrentMetadataGrantFactsUseOnlyKnownCurrentGlobalRows(t *testing.T) {
	cases := []struct {
		name  string
		rows  []string
		want  currentGrantFacts
		known bool
	}{
		{"schema_only", []string{"GRANT ALL PRIVILEGES ON `fixture_db`.* TO `fixture_user`@`%`"}, currentGrantFacts{}, true},
		{"global_all", []string{"GRANT ALL PRIVILEGES ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Trigger: true, Event: true}, true},
		{"global_four", []string{positive()}, currentGrantFacts{Select: true, ShowView: true, Trigger: true, Event: true}, true},
		{"no_select", []string{"GRANT SHOW VIEW, TRIGGER, EVENT ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{ShowView: true, Trigger: true, Event: true}, true},
		{"no_show_view", []string{"GRANT SELECT, TRIGGER, EVENT ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{Select: true, Trigger: true, Event: true}, true},
		{"no_trigger", []string{"GRANT SELECT, SHOW VIEW, EVENT ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Event: true}, true},
		{"no_event", []string{"GRANT SELECT, SHOW VIEW, TRIGGER ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Trigger: true}, true},
		{"partial_revoke", []string{positive(), "REVOKE SELECT ON `hidden`.* FROM `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Trigger: true, Event: true, PartialRevocations: true}, true},
		{"split_global_rows", []string{"GRANT SELECT, SHOW VIEW ON *.* TO `fixture_user`@`%`", "GRANT TRIGGER, EVENT ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Trigger: true, Event: true}, true},
		{"dynamic_role_proxy_ignored", []string{"GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON `fixture_db`.* TO `fixture_user`@`%`", "GRANT ROLE_ADMIN ON *.* TO `fixture_user`@`%`", "GRANT `fixture_role`@`%` TO `fixture_user`@`%`", "GRANT PROXY ON `fixture_proxy`@`%` TO `fixture_user`@`%`"}, currentGrantFacts{}, true},
		{"unknown_after_positive", []string{positive(), "UNKNOWN " + fixtureSecret}, currentGrantFacts{}, false},
		{"malformed_revoke", []string{positive(), "REVOKE SELECT ON `hidden`.* FROM missing_account"}, currentGrantFacts{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, known := currentMetadataGrantFacts(tc.rows)
			if known != tc.known || got != tc.want {
				t.Fatal("current_global_fact_contract_failed")
			}
			_, originalKnown := unrestricted(tc.rows)
			if known != originalKnown {
				t.Fatal("diagnostic_broadened_canonical_syntax")
			}
		})
	}
}

func TestCurrentGrantFactsPublishOnlyAfterCompleteProbe(t *testing.T) {
	cases := []struct {
		name         string
		rows         []string
		want         currentGrantFacts
		unrestricted bool
	}{
		{"schema_only_despite_positive_rds", []string{"GRANT ALL PRIVILEGES ON `fixture_db`.* TO `fixture_user`@`%`"}, currentGrantFacts{}, false},
		{"global_all", []string{"GRANT ALL PRIVILEGES ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Trigger: true, Event: true}, true},
		{"missing_event", []string{"GRANT SELECT, SHOW VIEW, TRIGGER ON *.* TO `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Trigger: true}, false},
		{"current_revoke", []string{positive(), "REVOKE SELECT ON `hidden`.* FROM `fixture_user`@`%`"}, currentGrantFacts{Select: true, ShowView: true, Trigger: true, Event: true, PartialRevocations: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, m := mockDB(t)
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			m.ExpectQuery(currentSQL).WillReturnRows(grants(tc.rows...))
			m.ExpectQuery(mandatorySQL).WillReturnRows(sqlmock.NewRows([]string{"mandatory_roles"}).AddRow(""))
			m.ExpectQuery(rdsSQL).WillReturnRows(grants(positive()))
			m.ExpectQuery(identitySQL).WillReturnRows(identities(10, "NONE"))
			r := probe(context.Background(), db, fixtureConnection, fixtureBinding())
			if !r.Complete || r.CurrentUnrestricted != tc.unrestricted || r.ErrorCategory != "none" {
				t.Fatal("unrestricted_semantics_changed")
			}
			facts := []*bool{r.CurrentGlobalSelect, r.CurrentGlobalShowView, r.CurrentGlobalTrigger, r.CurrentGlobalEvent, r.CurrentPartialRevocations}
			want := []bool{tc.want.Select, tc.want.ShowView, tc.want.Trigger, tc.want.Event, tc.want.PartialRevocations}
			for i, fact := range facts {
				if fact == nil || *fact != want[i] {
					t.Fatal("complete_current_fact_contract_failed")
				}
			}
			assertPrivate(t, r)
		})
	}
}

func TestIncompleteRunEmitsNullCurrentFactsAndOnlyReceiptFields(t *testing.T) {
	var out bytes.Buffer
	if run(nil, func(string) string { return fixtureSecret }, &out) != 1 {
		t.Fatal("invalid_input_returned_success")
	}
	var fields map[string]any
	if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
		t.Fatal("receipt_json_invalid")
	}
	allowed := map[string]bool{}
	for _, key := range []string{"format_version", "source_sha", "run_id", "expected_target_hash", "source_target_hash", "current_unrestricted_metadata_grants", "current_global_select_grant", "current_global_show_view_grant", "current_global_trigger_grant", "current_global_event_grant", "current_partial_revocations_present", "rds_role_grants_available", "rds_role_unrestricted_metadata_grants", "assigned_roles_present", "mandatory_roles_present", "diagnostic_only", "complete", "error_category"} {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			t.Fatal("unapproved_receipt_field")
		}
	}
	for _, key := range []string{"current_global_select_grant", "current_global_show_view_grant", "current_global_trigger_grant", "current_global_event_grant", "current_partial_revocations_present"} {
		if value, exists := fields[key]; !exists || value != nil {
			t.Fatal("incomplete_fact_not_explicit_null")
		}
	}
	if strings.Contains(out.String(), fixtureSecret) {
		t.Fatal("input_value_leaked")
	}
}
