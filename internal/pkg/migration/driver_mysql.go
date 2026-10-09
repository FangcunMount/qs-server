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
	db           *sql.DB
	runConn      *sql.Conn
	borrowedConn *sql.Conn
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
	if d.db == nil && d.borrowedConn == nil {
		return nil, fmt.Errorf("mysql: database connection is nil")
	}
	if d.runConn != nil {
		return nil, fmt.Errorf("mysql: previous run-owned connection has not been released")
	}

	ctx, cancel := retirementContext(migrationRunContext(config))
	defer cancel()
	conn := d.borrowedConn
	var err error
	if conn == nil {
		conn, err = d.db.Conn(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("mysql: acquire run-owned connection: %w", err)
	}
	d.runConn = conn
	var selected string
	if err = conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&selected); err != nil || selected != config.Database {
		closeErr := d.finishRun()
		return nil, fmt.Errorf("mysql: selected run connection rejected (close: %v)", closeErr)
	}
	// WithConnection owns only this run connection; the borrowed pool stays open.
	databaseDriver, err := mysql.WithConnection(ctx, conn, &mysql.Config{
		DatabaseName:    config.Database,
		MigrationsTable: config.MigrationsTable,
	})
	if err != nil {
		closeErr := d.finishRun()
		return nil, fmt.Errorf("mysql: failed to create database driver: %w (close: %v)", err, closeErr)
	}
	instance, err := d.createInstance(fs, newRetirementDatabaseDriver(databaseDriver, BackendMySQL, config, conn, nil, fs))
	if err != nil {
		closeErr := d.finishRun()
		return nil, fmt.Errorf("mysql: create migration instance: %w (close: %v)", err, closeErr)
	}
	return instance, nil
}

func (d *MySQLDriver) finishRun() error {
	if d.runConn == nil {
		return nil
	}
	c := d.runConn
	d.runConn = nil
	if c == d.borrowedConn {
		return nil
	}
	return c.Close()
}
