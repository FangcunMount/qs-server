package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testBootstrapApproval(t *testing.T, r *migrationResources) (PairConfig, bootstrapAuthorization) {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := PairConfig{MySQLDatabase: "qs_m3_test", MongoDatabase: "qs_m3_test", ExpectedSourceSHA: strings.Repeat("a", 40), BootstrapAuthorizationFile: filepath.Join(directory, "approved.json")}
	a := bootstrapAuthorization{FormatVersion: 2, Kind: "qs_schema_pair_pristine_bootstrap", OperationID: "1-1", ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), ApprovedSourceSHA: cfg.ExpectedSourceSHA, ApprovalSummarySHA256: strings.Repeat("b", 64), MySQLDatabase: cfg.MySQLDatabase, MongoDatabase: cfg.MongoDatabase, MySQLIdentitySHA256: strings.Repeat("c", 64), MongoClusterSHA256: strings.Repeat("d", 64), MySQLVersion: r.sql.head, MongoVersion: r.mongo.head, MigrationResourcesSHA256: r.sha256}
	return cfg, a
}

func writeBootstrapApproval(t *testing.T, cfg *PairConfig, a bootstrapAuthorization) {
	t.Helper()
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.BootstrapAuthorizationFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAuthorizationSHA256 = retirementHash(raw)
}

func TestBootstrapV2BindsFullResourcesHeadsAndOneUse(t *testing.T) {
	r, err := readMigrationResources(futureMigrationSource(t), []schemaVersionPair{{100, 39}, {101, 40}})
	if err != nil {
		t.Fatal(err)
	}
	cfg, a := testBootstrapApproval(t, r)
	writeBootstrapApproval(t, &cfg, a)
	observed, err := readBootstrapAuthorization(cfg, r)
	if err != nil || *observed != a {
		t.Fatal(observed, err)
	}
	if err := consumeBootstrapAuthorization(cfg, observed, r); err != nil {
		t.Fatal(err)
	}
	if _, err := readBootstrapAuthorization(cfg, r); err == nil {
		t.Fatal("consumed approval accepted")
	}
	if err := consumeBootstrapAuthorization(cfg, observed, r); err == nil {
		t.Fatal("approval consumed twice")
	}
}

func TestBootstrapV2RejectsStaleOrUnknownBinding(t *testing.T) {
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*bootstrapAuthorization)
	}{
		{"old_format", func(a *bootstrapAuthorization) { a.FormatVersion = 1 }},
		{"old_kind", func(a *bootstrapAuthorization) { a.Kind = "qs_compatibility_retirement_b_pristine_bootstrap" }},
		{"wrong_source", func(a *bootstrapAuthorization) { a.ApprovedSourceSHA = strings.Repeat("e", 40) }},
		{"wrong_head", func(a *bootstrapAuthorization) { a.MySQLVersion++ }},
		{"wrong_resources", func(a *bootstrapAuthorization) { a.MigrationResourcesSHA256 = legacyBootstrapResourcesSHA256(t) }},
		{"expired", func(a *bootstrapAuthorization) { a.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg, a := testBootstrapApproval(t, r)
			test.change(&a)
			writeBootstrapApproval(t, &cfg, a)
			if _, err := readBootstrapAuthorization(cfg, r); err == nil {
				t.Fatal("invalid approval accepted")
			}
			if _, err := os.Stat(cfg.BootstrapAuthorizationFile + ".consumed"); !os.IsNotExist(err) {
				t.Fatal("rejected approval consumed")
			}
		})
	}
}

func legacyBootstrapResourcesSHA256(t *testing.T) string {
	t.Helper()
	a, err := migrations.ReadFile("migrations/mysql/000100_retire_compatibility_message_storage.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	b, err := migrations.ReadFile("migrations/mongodb/000039_retire_compatibility_message_storage.up.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal([]string{retirementHash(a), retirementHash(b)})
	if err != nil {
		t.Fatal(err)
	}
	return retirementHash(raw)
}
