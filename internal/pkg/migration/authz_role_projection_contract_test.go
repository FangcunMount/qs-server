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
