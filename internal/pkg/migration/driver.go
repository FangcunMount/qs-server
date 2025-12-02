package migration

import (
	"embed"

	"github.com/golang-migrate/migrate/v4"
)

// Backend defines the migration backend type.
type Backend string

const (
	BackendMySQL Backend = "mysql"
	BackendMongo Backend = "mongodb"
)

// Driver defines the interface for database migration drivers.
// Each database type (MySQL, MongoDB, etc.) should implement this interface.
type Driver interface {
	// Backend returns the backend type of this driver.
	Backend() Backend

	// SourcePath returns the path to migration files within the embedded FS.
	SourcePath() string

	// CreateInstance creates a migrate.Migrate instance for this driver.
	CreateInstance(fs embed.FS, config *Config) (*migrate.Migrate, error)
}
