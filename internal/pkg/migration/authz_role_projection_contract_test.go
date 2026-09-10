package migration

import (
	"strings"
	"testing"
)

func TestAuthzRoleProjectionMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000070_authz_role_projection_semantics.up.sql")
	down := readMySQLMigration(t, "000070_authz_role_projection_semantics.down.sql")
	for _, fragment := range []string{"effective_roles", "authz_policy_version", "authz_projected_at", "authz_projection_pending", "SET `effective_roles` = `roles`"} {
		if !strings.Contains(up, fragment) {
			t.Fatalf("up migration missing %q", fragment)
		}
	}
	if !strings.Contains(down, "SET `roles` = `effective_roles`") {
		t.Fatal("down migration must preserve effective role evidence in roles")
	}
}

func TestIndependentRoleProjectionRefreshMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000071_refresh_independent_role_projections.up.sql")
	down := readMySQLMigration(t, "000071_refresh_independent_role_projections.down.sql")
	for _, sql := range []string{up, down} {
		if !strings.Contains(sql, "authz_projection_pending=TRUE") {
			t.Fatalf("migration must mark projections pending for IAM reconcile:\n%s", sql)
		}
		if strings.Contains(sql, "qs:staff") || strings.Contains(sql, "qs:evaluator") {
			t.Fatal("personnel mapping must not be hardcoded into general SQL migrations")
		}
	}
}
