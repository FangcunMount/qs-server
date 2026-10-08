package options

import (
	"fmt"
	"github.com/spf13/pflag"
	"path/filepath"
	"regexp"
)

// MigrationOptions defines options for database migration.
type MigrationOptions struct {
	Enabled                      bool   `json:"enabled"  mapstructure:"enabled"`
	AutoSeed                     bool   `json:"autoseed" mapstructure:"autoseed"`
	Database                     string `json:"database" mapstructure:"database"`
	BootstrapAuthorizationFile   string `json:"retirement-bootstrap-authorization-file" mapstructure:"retirement-bootstrap-authorization-file"`
	BootstrapAuthorizationSHA256 string `json:"retirement-bootstrap-authorization-sha256" mapstructure:"retirement-bootstrap-authorization-sha256"`
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
	if o == nil {
		return []error{fmt.Errorf("migration options are required")}
	}
	if (o.BootstrapAuthorizationFile == "") != (o.BootstrapAuthorizationSHA256 == "") {
		errs = append(errs, fmt.Errorf("migration pristine bootstrap authorization requires both file and approved raw SHA256"))
	}
	if o.BootstrapAuthorizationFile != "" && (!filepath.IsAbs(o.BootstrapAuthorizationFile) || filepath.Clean(o.BootstrapAuthorizationFile) != o.BootstrapAuthorizationFile) {
		errs = append(errs, fmt.Errorf("migration bootstrap authorization file must be a clean absolute path"))
	}
	if o.BootstrapAuthorizationSHA256 != "" && !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(o.BootstrapAuthorizationSHA256) {
		errs = append(errs, fmt.Errorf("migration bootstrap authorization SHA256 must be lowercase hexadecimal"))
	}

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
	fs.StringVar(&o.BootstrapAuthorizationFile, "migration.retirement-bootstrap-authorization-file", o.BootstrapAuthorizationFile, "Private externally approved pristine SQL99/Mongo38 bootstrap authorization file.")
	fs.StringVar(&o.BootstrapAuthorizationSHA256, "migration.retirement-bootstrap-authorization-sha256", o.BootstrapAuthorizationSHA256, "Externally approved raw SHA256 of the pristine bootstrap authorization; empty rejects cold bootstrap.")
}
