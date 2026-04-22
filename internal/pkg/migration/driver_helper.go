package migration

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

type embeddedMigrationDriver struct {
	backend    Backend
	sourcePath string
	prefix     string
}

func newEmbeddedMigrationDriver(backend Backend, sourcePath, prefix string) embeddedMigrationDriver {
	return embeddedMigrationDriver{
		backend:    backend,
		sourcePath: sourcePath,
		prefix:     prefix,
	}
}

func (d embeddedMigrationDriver) Backend() Backend {
	return d.backend
}

func (d embeddedMigrationDriver) SourcePath() string {
	return d.sourcePath
}

func (d embeddedMigrationDriver) createInstance(fs embed.FS, databaseDriver migratedb.Driver) (*migrate.Migrate, error) {
	return createEmbeddedMigrationInstance(fs, d.sourcePath, d.backend, databaseDriver, d.prefix)
}

func createEmbeddedMigrationInstance(
	fs embed.FS,
	sourcePath string,
	backend Backend,
	databaseDriver migratedb.Driver,
	prefix string,
) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(fs, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create source driver: %w", prefix, err)
	}

	instance, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		string(backend),
		databaseDriver,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create migrate instance: %w", prefix, err)
	}

	return instance, nil
}
