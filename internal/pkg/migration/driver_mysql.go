package migration

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// MySQLDriver implements the Driver interface for MySQL databases.
type MySQLDriver struct {
	db *sql.DB
}

// NewMySQLDriver creates a new MySQL migration driver.
func NewMySQLDriver(db *sql.DB) *MySQLDriver {
	return &MySQLDriver{db: db}
}

// Backend returns the backend type.
func (d *MySQLDriver) Backend() Backend {
	return BackendMySQL
}

// SourcePath returns the path to MySQL migration files.
func (d *MySQLDriver) SourcePath() string {
	return "migrations/mysql"
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

	// Create source driver from embedded filesystem
	sourceDriver, err := iofs.New(fs, d.SourcePath())
	if err != nil {
		return nil, fmt.Errorf("mysql: failed to create source driver: %w", err)
	}

	// Create migrate instance
	instance, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		string(d.Backend()),
		databaseDriver,
	)
	if err != nil {
		return nil, fmt.Errorf("mysql: failed to create migrate instance: %w", err)
	}

	return instance, nil
}
