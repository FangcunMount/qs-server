package migration

import (
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
	for _, state := range []struct {
		sql, mongo uint
		want       bool
	}{{99, 38, true}, {100, 39, true}, {100, 38, false}, {99, 39, false}, {98, 37, false}} {
		if installedPairComplete(state.sql, state.mongo) != state.want {
			t.Fatal("ordinary initial pair conflict")
		}
	}
	if !installedPairStepAllowed(99, 38, 99, 38, BackendMySQL, false) || !installedPairStepAllowed(99, 38, 100, 38, BackendMongo, true) || !installedPairStepAllowed(100, 39, 100, 39, BackendMySQL, false) || !installedPairStepAllowed(100, 39, 100, 39, BackendMongo, true) {
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
		if installedPairStepAllowed(state.is, state.im, state.as, state.am, state.backend, state.done) {
			t.Fatal("unbound partial or changed phase accepted")
		}
	}
}
