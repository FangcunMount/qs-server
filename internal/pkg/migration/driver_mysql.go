package migration

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
)

// MySQLDriver implements the Driver interface for MySQL databases.
type MySQLDriver struct {
	embeddedMigrationDriver
	db *sql.DB
}

// NewMySQLDriver creates a new MySQL migration driver.
func NewMySQLDriver(db *sql.DB) *MySQLDriver {
	return &MySQLDriver{
		embeddedMigrationDriver: newEmbeddedMigrationDriver(BackendMySQL, "migrations/mysql", "mysql"),
		db:                      db,
	}
}

// CreateInstance creates a migrate.Migrate instance for MySQL.
func (d *MySQLDriver) CreateInstance(fs embed.FS, config *Config) (*migrate.Migrate, error) {
	if d.db == nil {
		return nil, fmt.Errorf("mysql: database connection is nil")
	}

	// Create MySQL database driver
	databaseDriver, err := mysql.WithInstance(d.db, &mysql.Config{
		DatabaseName:    config.Database,
		MigrationsTable: config.MigrationsTable,
	})
	if err != nil {
		return nil, fmt.Errorf("mysql: failed to create database driver: %w", err)
	}

	return d.createInstance(fs, databaseDriver)
}
