//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// This required gate accepts only an explicitly owned loopback fixture, with
// independently supplied server UUID and a unique namespace, before any DDL.
func TestMetadataProfileNativeOwnedMySQL(t *testing.T) {
	if os.Getenv("QS_METADATA_PROFILE_NATIVE_REQUIRED") != "1" {
		t.Fatal("native_mysql_required_not_enabled")
	}
	cfg, ok := envConnection(os.Getenv)
	if !ok || cfg.Host != "127.0.0.1" || cfg.Port == 3306 || cfg.User != "root" || !regexp.MustCompile(`^qs_profile_owned_[a-z0-9]{8,24}$`).MatchString(cfg.Database) {
		t.Fatal("owned_mysql_input_guard_failed")
	}
	connector, err := mysql.NewConnector(mysqlConfig(cfg))
	if err != nil {
		t.Fatal("owned_mysql_connector_failed")
	}
	admin := sql.OpenDB(connector)
	admin.SetMaxOpenConns(1)
	admin.SetMaxIdleConns(0)
	defer func() {
		if admin.Close() != nil {
			t.Error("owned_admin_release_failed")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var uuid, version string
	if admin.QueryRowContext(ctx, "SELECT @@server_uuid, VERSION()").Scan(&uuid, &version) != nil || !uuidRE.MatchString(uuid) || uuid != os.Getenv("QS_METADATA_PROFILE_NATIVE_UUID") || version != "8.0.36" {
		t.Fatal("owned_mysql_identity_guard_failed")
	}
	for _, statement := range []string{
		"CREATE ROLE `rds_superuser_role`@`%`",
		"GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON *.* TO `rds_superuser_role`@`%`",
		"CREATE USER `profile_schema`@`%` IDENTIFIED BY 'native-profile-private-fixture'",
		"GRANT SELECT ON `" + cfg.Database + "`.* TO `profile_schema`@`%`",
		"CREATE USER `profile_global`@`%` IDENTIFIED BY 'native-profile-private-fixture'",
		"GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON *.* TO `profile_global`@`%`",
		"CREATE USER `profile_role`@`%` IDENTIFIED BY 'native-profile-private-fixture'",
		"GRANT SELECT ON `" + cfg.Database + "`.* TO `profile_role`@`%`",
		"GRANT `rds_superuser_role`@`%` TO `profile_role`@`%`",
		"CREATE USER `profile_partial`@`%` IDENTIFIED BY 'native-profile-private-fixture'",
		"GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON *.* TO `profile_partial`@`%`",
		"REVOKE SELECT ON `" + cfg.Database + "_hidden`.* FROM `profile_partial`@`%`",
	} {
		if _, err = admin.ExecContext(ctx, statement); err != nil {
			t.Fatal("owned_mysql_fixture_setup_failed")
		}
	}
	base := map[string]string{"MYSQL_HOST": cfg.Host, "MYSQL_PORT": strconv.Itoa(cfg.Port), "MYSQL_DATABASE": cfg.Database,
		"PROFILE_SOURCE_SHA": strings.Repeat("a", 40), "PROFILE_RUN_ID": "12345-1", "PROFILE_EXPECTED_TARGET_HASH": hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, uuid)}
	for _, tc := range []struct {
		name, user                                               string
		current, roleAvailable, rolePotential, partial, assigned bool
	}{
		{"schema_only", "profile_schema", false, false, false, false, false},
		{"global_four", "profile_global", true, false, false, false, false},
		{"inactive_role_potential", "profile_role", false, true, true, false, true},
		{"partial_revocation", "profile_partial", false, false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{}
			for k, v := range base {
				env[k] = v
			}
			env["MYSQL_USERNAME"] = tc.user
			env["MYSQL_PASSWORD"] = "native-profile-private-fixture"
			var output bytes.Buffer
			if run(nil, func(k string) string { return env[k] }, &output) != 0 {
				t.Fatal("native_profile_not_complete")
			}
			var got receipt
			if json.Unmarshal(output.Bytes(), &got) != nil || !got.Complete || !got.DiagnosticOnly || got.ErrorCategory != "none" || got.CurrentUnrestricted != tc.current || got.RDSAvailable == nil || *got.RDSAvailable != tc.roleAvailable || got.CurrentPartialRevocations == nil || *got.CurrentPartialRevocations != tc.partial || got.AssignedRoles == nil || *got.AssignedRoles != tc.assigned {
				t.Fatal("native_profile_semantics_failed")
			}
			if tc.roleAvailable && (got.RDSUnrestricted == nil || *got.RDSUnrestricted != tc.rolePotential) || !tc.roleAvailable && got.RDSUnrestricted != nil {
				t.Fatal("native_role_semantics_failed")
			}
			// The application diagnostic connection must already be gone, not just idle.
			var remaining int
			if admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.processlist WHERE USER = ?", tc.user).Scan(&remaining) != nil || remaining != 0 {
				t.Fatal("native_sql_connection_not_released")
			}
			if bytes.Contains(output.Bytes(), []byte(uuid)) || bytes.Contains(output.Bytes(), []byte(env["MYSQL_PASSWORD"])) || bytes.Contains(output.Bytes(), []byte(tc.user)) {
				t.Fatal("native_private_output_leaked")
			}
		})
	}
	t.Run("wrong_target_stops_before_grants", func(t *testing.T) {
		env := map[string]string{}
		for k, v := range base {
			env[k] = v
		}
		env["MYSQL_USERNAME"] = "profile_global"
		env["MYSQL_PASSWORD"] = "native-profile-private-fixture"
		env["PROFILE_EXPECTED_TARGET_HASH"] = strings.Repeat("0", 64)
		var output bytes.Buffer
		if run(nil, func(k string) string { return env[k] }, &output) != 1 {
			t.Fatal("native_wrong_target_accepted")
		}
		var got receipt
		if json.Unmarshal(output.Bytes(), &got) != nil || got.ErrorCategory != "target_hash_mismatch" || got.Complete || got.CurrentGlobalSelect != nil || got.AssignedRoles != nil {
			t.Fatal("native_wrong_target_retained_conclusion")
		}
		var remaining int
		if admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.processlist WHERE USER = 'profile_global'").Scan(&remaining) != nil || remaining != 0 {
			t.Fatal("native_failed_sql_connection_not_released")
		}
	})
}
