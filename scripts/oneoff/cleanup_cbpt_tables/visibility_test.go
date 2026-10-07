package main

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const visibilityFKQuery = "SELECT table_schema,table_name,referenced_table_schema,referenced_table_name FROM information_schema.key_column_usage WHERE referenced_table_name IS NOT NULL LIMIT 10001"

var visibilityDefinitionQueries = []string{
	"SELECT event_object_schema,event_object_table,action_statement FROM information_schema.triggers LIMIT 10001",
	"SELECT table_schema,table_name,view_definition FROM information_schema.views LIMIT 10001",
	"SELECT routine_schema,routine_name,routine_definition FROM information_schema.routines LIMIT 10001",
	"SELECT event_schema,event_name,event_definition FROM information_schema.events LIMIT 10001",
}

func helpersVisibilityConnection(t *testing.T) (*sql.Conn, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = db.Close()
	})
	return conn, mock
}

func helpersVisibilityExpect(t *testing.T, mock sqlmock.Sqlmock, query string, rows *sqlmock.Rows) {
	t.Helper()
	mock.ExpectQuery("^" + regexp.QuoteMeta(query) + "$").WillReturnRows(rows).RowsWillBeClosed()
}

func helpersVisibilityGrants(t *testing.T, mock sqlmock.Sqlmock, statements ...string) {
	t.Helper()
	rows := sqlmock.NewRows([]string{"grants"})
	for _, statement := range statements {
		rows.AddRow(statement)
	}
	helpersVisibilityExpect(t, mock, "SHOW GRANTS FOR CURRENT_USER", rows)
}

func helpersVisibilityGlobal(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	helpersVisibilityGrants(t, mock, "GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON *.* TO 'fixture'@'%'")
}

func helpersVisibilityNoFK(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	helpersVisibilityExpect(t, mock, visibilityFKQuery,
		sqlmock.NewRows([]string{"table_schema", "table_name", "referenced_table_schema", "referenced_table_name"}))
}

func helpersVisibilityCategory(t *testing.T, mock sqlmock.Sqlmock, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatal("dependency gate accepted an unsupported or dependent object")
	}
	category, _ := errorCategory(err)
	if category != expected {
		t.Fatalf("category = %q, want %q", category, expected)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestVisibilityGrantCoverageFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		grants []string
	}{
		{"global_plus_partial_revoke", []string{
			"GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON *.* TO 'fixture'@'%'",
			"REVOKE SELECT ON `hidden_schema`.* FROM 'fixture'@'%'",
		}},
		{"all_privileges_plus_partial_revoke", []string{
			"GRANT ALL PRIVILEGES ON *.* TO 'fixture'@'%'",
			"REVOKE SHOW VIEW ON `hidden_schema`.* FROM 'fixture'@'%'",
		}},
		{"schema_only", []string{
			"GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON `qs`.* TO 'fixture'@'%'",
		}},
		{"roles_without_direct_global", []string{
			"GRANT USAGE ON *.* TO 'fixture'@'%'",
			"GRANT `audit_role`@`%` TO 'fixture'@'%'",
		}},
		{"unsupported_statement", []string{
			"GRANT SELECT, SHOW VIEW, TRIGGER, EVENT ON *.* TO 'fixture'@'%'",
			"SET DEFAULT ROLE ALL TO 'fixture'@'%'",
		}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			conn, mock := helpersVisibilityConnection(t)
			helpersVisibilityGrants(t, mock, item.grants...)
			err := dependencies(context.Background(), conn, "qs")
			helpersVisibilityCategory(t, mock, err, "metadata_visibility_blocked")
		})
	}
}

func TestVisibilityCrossSchemaFKBothDirectionsBlock(t *testing.T) {
	target := targetTables()[0]
	cases := []struct {
		name, schema, table, referencedSchema, referencedTable string
	}{
		{"inbound", "external_schema", "business_refs", "qs", target},
		{"outbound", "qs", target, "external_schema", "business_parent"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			conn, mock := helpersVisibilityConnection(t)
			helpersVisibilityGlobal(t, mock)
			helpersVisibilityExpect(t, mock, visibilityFKQuery,
				sqlmock.NewRows([]string{"table_schema", "table_name", "referenced_table_schema", "referenced_table_name"}).
					AddRow(item.schema, item.table, item.referencedSchema, item.referencedTable))
			err := dependencies(context.Background(), conn, "qs")
			helpersVisibilityCategory(t, mock, err, "foreign_key_dependency")
		})
	}
}

func TestVisibilityNullDefinitionsFailClosed(t *testing.T) {
	for index, query := range visibilityDefinitionQueries {
		t.Run([]string{"trigger", "view", "routine", "event"}[index], func(t *testing.T) {
			conn, mock := helpersVisibilityConnection(t)
			helpersVisibilityGlobal(t, mock)
			helpersVisibilityNoFK(t, mock)
			for before := 0; before < index; before++ {
				helpersVisibilityExpect(t, mock, visibilityDefinitionQueries[before],
					sqlmock.NewRows([]string{"schema", "name", "definition"}))
			}
			helpersVisibilityExpect(t, mock, query,
				sqlmock.NewRows([]string{"schema", "name", "definition"}).AddRow("external_schema", "opaque_object", nil))
			err := dependencies(context.Background(), conn, "qs")
			helpersVisibilityCategory(t, mock, err, "metadata_definition_hidden")
		})
	}
}

func TestVisibilityCrossSchemaViewReferenceBlocks(t *testing.T) {
	conn, mock := helpersVisibilityConnection(t)
	helpersVisibilityGlobal(t, mock)
	helpersVisibilityNoFK(t, mock)
	helpersVisibilityExpect(t, mock, visibilityDefinitionQueries[0],
		sqlmock.NewRows([]string{"schema", "name", "definition"}))
	helpersVisibilityExpect(t, mock, visibilityDefinitionQueries[1],
		sqlmock.NewRows([]string{"schema", "name", "definition"}).
			AddRow("external_schema", "report_view", "select id from `qs`.`"+targetTables()[0]+"`"))
	err := dependencies(context.Background(), conn, "qs")
	helpersVisibilityCategory(t, mock, err, "definition_dependency")
}

func TestVisibilityAttachedTriggerBlocks(t *testing.T) {
	conn, mock := helpersVisibilityConnection(t)
	helpersVisibilityGlobal(t, mock)
	helpersVisibilityNoFK(t, mock)
	helpersVisibilityExpect(t, mock, visibilityDefinitionQueries[0],
		sqlmock.NewRows([]string{"schema", "name", "definition"}).
			AddRow("qs", targetTables()[0], "SET NEW.id=NEW.id"))
	err := dependencies(context.Background(), conn, "qs")
	helpersVisibilityCategory(t, mock, err, "trigger_dependency")
}

func TestVisibilityCompleteDirectGlobalAndNoDependenciesPass(t *testing.T) {
	conn, mock := helpersVisibilityConnection(t)
	helpersVisibilityGlobal(t, mock)
	helpersVisibilityNoFK(t, mock)
	for _, query := range visibilityDefinitionQueries {
		helpersVisibilityExpect(t, mock, query,
			sqlmock.NewRows([]string{"schema", "name", "definition"}))
	}
	if err := dependencies(context.Background(), conn, "qs"); err != nil {
		t.Fatalf("complete dependency visibility should pass: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
