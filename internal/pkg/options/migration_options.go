package options

import (
	"github.com/spf13/pflag"
)

// MigrationOptions defines options for database migration.
type MigrationOptions struct {
	Enabled  bool   `json:"enabled"  mapstructure:"enabled"`
	AutoSeed bool   `json:"autoseed" mapstructure:"autoseed"`
	Database string `json:"database" mapstructure:"database"`
}

// NewMigrationOptions create a `zero` value instance.
func NewMigrationOptions() *MigrationOptions {
	return &MigrationOptions{
		Enabled:  true,
		AutoSeed: false,
		Database: "",
	}
}

// Validate verifies flags passed to MigrationOptions.
func (o *MigrationOptions) Validate() []error {
	errs := []error{}

	return errs
}

// AddFlags adds flags related to database migration for a specific APIServer to the specified FlagSet.
func (o *MigrationOptions) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.Enabled, "migration.enabled", o.Enabled, ""+
		"Enable automatic database migration on startup.")

	fs.BoolVar(&o.AutoSeed, "migration.autoseed", o.AutoSeed, ""+
		"Enable automatic seed data loading on startup (requires migration.enabled).")

	fs.StringVar(&o.Database, "migration.database", o.Database, ""+
		"Database name for migration.")
}
