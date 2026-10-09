//go:build integration

package retirement

import (
	"database/sql"
	"errors"
	"os"
	"testing"

	golangmigrate "github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Retained historical CAS and source-resolution tests need production A's
// complete SQL99 schema, including the old source objects. Keep this test-only
// fixture at that explicit boundary; B's paired native upgrades own SQL100.
func migrateHistoricalA99Fixture(t *testing.T, pool *sql.DB, database string) {
	t.Helper()
	source, err := iofs.New(os.DirFS("../../../pkg/migration/migrations/mysql"), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	}()
	// The fixture owns this connection; closing it never closes the borrowed
	// pool used by the actual evidence transactions and readback assertions.
	conn, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	driver, err := migratemysql.WithConnection(t.Context(), conn, &migratemysql.Config{DatabaseName: database, MigrationsTable: "schema_migrations"})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := golangmigrate.NewWithInstance("iofs", source, database, driver)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		err := instance.Migrate(99)
		if (attempt == 0 && err != nil) || (attempt == 1 && !errors.Is(err, golangmigrate.ErrNoChange)) {
			t.Fatalf("actual A99 upgrade/restart attempt=%d error=%v", attempt, err)
		}
		version, dirty, err := instance.Version()
		if err != nil || version != 99 || dirty {
			t.Fatalf("actual A99 clean head: head=%d dirty=%v error=%v", version, dirty, err)
		}
	}
}
