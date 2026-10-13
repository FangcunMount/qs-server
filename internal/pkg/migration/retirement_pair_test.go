package migration

import (
	"bytes"
	"strings"
	"testing"
)

func TestFinalMigrationResourceBoundary(t *testing.T) {
	sqlBody, e := migrations.ReadFile("migrations/mysql/000100_retire_compatibility_message_storage.up.sql")
	if e != nil || strings.TrimSpace(string(sqlBody)) != "DROP TABLE IF EXISTS `domain_event_outbox`;\nDROP TABLE IF EXISTS `ai_bridge_commands`;\nDROP TABLE IF EXISTS `ai_messaging_legacy_commands`;" {
		t.Fatal("SQL final boundary changed")
	}
	mongoBody, e := migrations.ReadFile("migrations/mongodb/000039_retire_compatibility_message_storage.up.json")
	if e != nil || strings.TrimSpace(string(mongoBody)) != `[{"drop":"domain_event_outbox"}]` {
		t.Fatal("Mongo final command changed")
	}
	for _, target := range []uint{0, 100, 101} {
		d := &retirementFailureDriver{embeddedMigrationDriver: newEmbeddedMigrationDriver(BackendMySQL, "migrations/mysql", "mysql")}
		if _, _, e := (&Migrator{driver: d, config: ensureConfigDefaults(&Config{Enabled: true, Database: "synthetic_no_connection"})}).run(target); e == nil || d.releases != 0 {
			t.Fatal("historical runner crossed final boundary")
		}
	}
}

func TestOrdinaryPairCannotAdoptPartialOrUnknownMigration(t *testing.T) {
	resources, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []struct {
		sql, mongo uint
		want       bool
	}{{99, 38, true}, {100, 39, true}, {100, 38, false}, {99, 39, false}, {98, 37, false}} {
		if installedPairComplete(state.sql, state.mongo) != state.want {
			t.Fatal("ordinary initial pair conflict")
		}
	}
	if !installedPairStepAllowed(99, 38, 99, 38, BackendMySQL, false, resources) || !installedPairStepAllowed(99, 38, 100, 38, BackendMongo, true, resources) || !installedPairStepAllowed(100, 39, 100, 39, BackendMySQL, false, resources) || !installedPairStepAllowed(100, 39, 100, 39, BackendMongo, true, resources) {
		t.Fatal("true same-process migration phase rejected")
	}
	for _, state := range []struct {
		is, im, as, am uint
		backend        Backend
		done           bool
	}{
		{100, 38, 100, 38, BackendMongo, true},
		{99, 39, 100, 39, BackendMongo, true},
		{99, 38, 100, 38, BackendMongo, false},
		{99, 38, 100, 38, BackendMySQL, true},
		{100, 39, 100, 38, BackendMongo, true},
		{99, 38, 100, 39, BackendMongo, true},
	} {
		if installedPairStepAllowed(state.is, state.im, state.as, state.am, state.backend, state.done, resources) {
			t.Fatal("unbound partial or changed phase accepted")
		}
	}
}

func TestFuturePairStagesUseDeclaredStartAndActualTargets(t *testing.T) {
	r, err := readMigrationResources(futureMigrationSource(t), []schemaVersionPair{{99, 38}, {100, 39}, {101, 39}, {101, 40}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name                                             string
		initialSQL, initialMongo, actualSQL, actualMongo uint
		backend                                          Backend
		done, want                                       bool
	}{
		{"sql_start", 100, 39, 100, 39, BackendMySQL, false, true},
		{"mongo_after_sql", 100, 39, 101, 39, BackendMongo, true, true},
		{"declared_independent_restart", 101, 39, 101, 39, BackendMySQL, false, true},
		{"mongo_no_change_start", 101, 40, 101, 40, BackendMongo, true, true},
		{"old_boundary_live_phase", 99, 38, 101, 38, BackendMongo, true, true},
		{"undeclared_start", 100, 40, 101, 40, BackendMongo, true, false},
		{"sql_not_finished", 100, 39, 101, 39, BackendMongo, false, false},
		{"mongo_changed_externally", 100, 39, 101, 40, BackendMongo, true, false},
		{"sql_changed_externally", 100, 39, 101, 39, BackendMySQL, false, false},
		{"old_partial_restart", 100, 38, 101, 38, BackendMongo, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := installedPairStepAllowed(test.initialSQL, test.initialMongo, test.actualSQL, test.actualMongo, test.backend, test.done, r); got != test.want {
				t.Fatalf("allowed=%v want=%v", got, test.want)
			}
		})
	}
}

func TestBoundaryAndFutureCleanWritesCannotForceSuccess(t *testing.T) {
	r, err := readMigrationResources(futureMigrationSource(t), []schemaVersionPair{{100, 39}, {101, 40}})
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{99, 100, 101, 102} {
		provider := &retirementFailureProvider{version: 100}
		driver := &retirementDatabaseDriver{Driver: provider, backend: BackendMySQL, resources: r, sourceValid: true, bodyValid: true, pair: &PairPreflight{resources: r}}
		if err := driver.SetVersion(version, false); err == nil {
			t.Fatalf("force to %d accepted", version)
		}
		if provider.version != 100 {
			t.Fatal("rejected force changed schema metadata")
		}
	}
}

func TestFutureStepRejectsChangedDirtyMetadataBeforeRunAndClean(t *testing.T) {
	r, err := readMigrationResources(futureMigrationSource(t), []schemaVersionPair{{100, 39}, {101, 40}})
	if err != nil {
		t.Fatal(err)
	}
	provider := &retirementFailureProvider{version: 101, dirty: false}
	driver := &retirementDatabaseDriver{Driver: provider, backend: BackendMySQL, resources: r, sourceValid: true, bodyValid: true, version: 101, dirty: true, stepSucceeded: true, pair: &PairPreflight{resources: r}}
	if err := driver.Run(bytes.NewBufferString("CREATE TABLE migration_future_probe (id BIGINT PRIMARY KEY);\n")); err == nil {
		t.Fatal("externally cleared pending metadata accepted")
	}
	if err := driver.SetVersion(101, false); err == nil {
		t.Fatal("externally changed metadata re-certified clean")
	}
}
