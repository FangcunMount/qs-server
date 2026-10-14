// Package testfixture contains only the fixed Actor-column native-test seam.
// Business binaries do not import it. It does not certify a historical paired
// bootstrap or offer recovery, arbitrary version selection, Force, or dirty edits.
package testfixture

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

// RetireActorColumns runs the original102 body through the real provider on an
// already-created disposable SQL shape. Both101 and102 ledger transitions are
// provider writes; no clean row is manufactured by the fixture.
func RetireActorColumns(t testing.TB, db *gorm.DB, up []byte) {
	t.Helper()
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := migratemysql.WithConnection(ctx, conn, &migratemysql.Config{StatementTimeout: 5 * time.Second})
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	source, err := iofs.New(fstest.MapFS{
		"000101_owned_actor_fixture.up.sql":                &fstest.MapFile{Data: []byte("SELECT 1;")},
		"000102_retire_actor_compatibility_columns.up.sql": &fstest.MapFile{Data: up},
	}, ".")
	if err != nil {
		_ = provider.Close()
		t.Fatal(err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "mysql", provider)
	if err != nil {
		_ = source.Close()
		_ = provider.Close()
		t.Fatal(err)
	}
	runErr := m.Migrate(102)
	sourceErr, closeErr := m.Close()
	if runErr != nil || sourceErr != nil || closeErr != nil {
		t.Fatalf("fixed Actor fixture migration or release failed: %v / %v / %v", runErr, sourceErr, closeErr)
	}
	var rows []struct {
		Version uint64
		Dirty   bool
	}
	if err := db.Raw("SELECT version,dirty FROM schema_migrations").Scan(&rows).Error; err != nil || len(rows) != 1 || rows[0].Version != 102 || rows[0].Dirty {
		t.Fatal("fixed Actor fixture did not finish at clean SQL102", err)
	}
}
