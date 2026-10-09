package migration

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func transitionTestBinding() CompatibilityPairRunBinding {
	return CompatibilityPairRunBinding{ApprovedSourceSHA: strings.Repeat("a", 40), OriginalSourceSHA: strings.Repeat("b", 40), OperationID: "123-1", OriginalRunID: "124-1", ActualRunID: "125-1", ManifestSHA256: strings.Repeat("c", 64), ArchiveSHA256: strings.Repeat("d", 64), RequestSHA256: strings.Repeat("e", 64), WindowStartSHA256: strings.Repeat("f", 64), ResourcesSHA256: CompatibilityMigrationResourcesSHA256()}
}
func TestControlledPairRequiresExactMaterialAndCompiledSource(t *testing.T) {
	good := transitionTestBinding()
	if !transitionBindingValid(good, good.ApprovedSourceSHA) {
		t.Fatal("exact expected material rejected")
	}
	for _, test := range []struct {
		name   string
		change func(*CompatibilityPairRunBinding)
	}{
		{"other_build", func(b *CompatibilityPairRunBinding) { b.ApprovedSourceSHA = strings.Repeat("c", 40) }},
		{"unknown_original_source", func(b *CompatibilityPairRunBinding) { b.OriginalSourceSHA = "unknown" }},
		{"other_resources", func(b *CompatibilityPairRunBinding) { b.ResourcesSHA256 = strings.Repeat("0", 64) }},
		{"unbound_op", func(b *CompatibilityPairRunBinding) { b.OperationID = "pending" }},
		{"missing_manifest", func(b *CompatibilityPairRunBinding) { b.ManifestSHA256 = "" }},
		{"missing_original_run", func(b *CompatibilityPairRunBinding) { b.OriginalRunID = "" }},
		{"missing_actual_run", func(b *CompatibilityPairRunBinding) { b.ActualRunID = "" }},
		{"missing_archive", func(b *CompatibilityPairRunBinding) { b.ArchiveSHA256 = "" }},
		{"missing_request", func(b *CompatibilityPairRunBinding) { b.RequestSHA256 = "" }},
		{"missing_window", func(b *CompatibilityPairRunBinding) { b.WindowStartSHA256 = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := good
			test.change(&b)
			if transitionBindingValid(b, good.ApprovedSourceSHA) {
				t.Fatal("unbound material accepted")
			}
		})
	}
	if transitionBindingValid(good, "unknown") {
		t.Fatal("unknown build accepted")
	}
}
func TestControlledPairReportCannotConstructNativeProof(t *testing.T) {
	for _, p := range []*CompatibilityPairMigrationProof{nil, {}, {observation: CompatibilityPairMigrationObservation{SQLBefore: 99, SQLAfter: 100, MongoBefore: 38, MongoAfter: 39, SQLChanged: true, MongoChanged: true}}} {
		if p.VerifyAfter(context.Background(), nil, nil) == nil {
			t.Fatal("caller observation admitted")
		}
		if p.Observation() != (CompatibilityPairMigrationObservation{}) {
			t.Fatal("zero/native-unbound proof exposed observations")
		}
		if p != nil {
			if _, e := json.Marshal(p); e == nil {
				t.Fatal("proof serialized")
			}
		}
	}
	var pair *PairPreflight
	proof, a, e := pair.Run(context.Background(), transitionTestBinding(), nil)
	if e == nil || proof != nil || a.SQLAttempted || a.MongoAttempted || a.Completion != "not_started" {
		t.Fatal("unbound Run started")
	}
	if _, e = PreflightCompatibilityPairConnection(context.Background(), nil, nil, PairConfig{}); e == nil {
		t.Fatal("missing host handles admitted")
	}
	if _, e = PreflightCompatibilityPair(context.Background(), nil, nil, PairConfig{}); e == nil {
		t.Fatal("nil ordinary pair admitted")
	}
	if !retirementSQLMissing((*sql.DB)(nil)) || !retirementSQLMissing((*sql.Conn)(nil)) {
		t.Fatal("typed nil escaped real handle check")
	}
}
func TestControlledBorrowedConnectionFinalizerNeverClosesHostConnection(t *testing.T) {
	conn := new(sql.Conn)
	d := &MySQLDriver{borrowedConn: conn, runConn: conn}
	if e := d.finishRun(); e != nil || d.runConn != nil || d.borrowedConn != conn {
		t.Fatal("host-owned connection finalization rejected")
	}
	if e := d.finishRun(); e != nil {
		t.Fatal("repeated run finalization failed")
	}
	// A zero sql.Conn would panic if Close were called. This unit asserts only
	// ownership; a real MaxOpenConns=1 native test is still required.
}
func TestControlledPairExactResourceBoundary(t *testing.T) {
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
