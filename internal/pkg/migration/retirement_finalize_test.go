package migration

import (
	"embed"
	"errors"
	"io"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
)

// Provider failures and connection release failures are independent facts.
// The runner must retain both and must never call the borrowed provider's Close.
func TestMigrationRunFinalizesOwnedConnectionOnEveryProviderExit(t *testing.T) {
	primary := errors.New("test provider failure")
	release := errors.New("test release result unknown")
	for _, test := range []struct {
		name           string
		construct      bool
		versionFailure int
		runFailure     bool
		unchanged      bool
		releaseFailure bool
	}{
		{name: "construction", construct: true},
		{name: "initial_version", versionFailure: 1},
		{name: "up", runFailure: true},
		{name: "final_version", versionFailure: 3},
		{name: "no_change", unchanged: true},
		{name: "success"},
		{name: "release_unknown", releaseFailure: true},
		{name: "provider_and_release_unknown", runFailure: true, releaseFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &retirementFailureProvider{version: 97, versionFailure: test.versionFailure, primary: primary}
			if test.unchanged {
				provider.version = 98
			}
			if test.runFailure {
				provider.runError = primary
			}
			driver := &retirementFailureDriver{embeddedMigrationDriver: newEmbeddedMigrationDriver(BackendMySQL, "migrations/mysql", "mysql"), provider: provider}
			if test.construct {
				driver.constructError = primary
			}
			if test.releaseFailure {
				driver.releaseError = release
			}
			_, _, err := (&Migrator{driver: driver, config: ensureConfigDefaults(&Config{Enabled: true, Database: "synthetic_no_connection"})}).run(98)
			if driver.releases != 1 {
				t.Fatalf("run-owned release count=%d", driver.releases)
			}
			if provider.closes != 0 {
				t.Fatal("borrowed provider was closed")
			}
			wantPrimary := test.construct || test.versionFailure != 0 || test.runFailure
			if errors.Is(err, primary) != wantPrimary {
				t.Fatalf("primary error retained=%v expected=%v", errors.Is(err, primary), wantPrimary)
			}
			if errors.Is(err, release) != test.releaseFailure {
				t.Fatalf("release error retained=%v expected=%v", errors.Is(err, release), test.releaseFailure)
			}
			if !wantPrimary && !test.releaseFailure && err != nil {
				t.Fatal(err)
			}
		})
	}
}

type retirementFailureDriver struct {
	embeddedMigrationDriver
	provider                     *retirementFailureProvider
	constructError, releaseError error
	releases                     int
}

func (d *retirementFailureDriver) CreateInstance(fs embed.FS, _ *Config) (*migrate.Migrate, error) {
	if d.constructError != nil {
		return nil, d.constructError
	}
	return d.createInstance(fs, d.provider)
}
func (d *retirementFailureDriver) finishRun() error { d.releases++; return d.releaseError }

type retirementFailureProvider struct {
	version                              int
	dirty                                bool
	versionFailure, versionReads, closes int
	primary, runError                    error
}

func (d *retirementFailureProvider) Open(string) (migratedb.Driver, error) { return d, nil }
func (d *retirementFailureProvider) Close() error                          { d.closes++; return nil }
func (d *retirementFailureProvider) Lock() error                           { return nil }
func (d *retirementFailureProvider) Unlock() error                         { return nil }
func (d *retirementFailureProvider) Run(reader io.Reader) error {
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return err
	}
	return d.runError
}
func (d *retirementFailureProvider) SetVersion(v int, dirty bool) error {
	d.version, d.dirty = v, dirty
	return nil
}
func (d *retirementFailureProvider) Version() (int, bool, error) {
	d.versionReads++
	if d.versionReads == d.versionFailure {
		return 0, false, d.primary
	}
	return d.version, d.dirty, nil
}
func (d *retirementFailureProvider) Drop() error { return nil }
