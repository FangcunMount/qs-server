package migration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

func initialPlanFixture(t *testing.T) InitialBindingPlan {
	t.Helper()
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	return InitialBindingPlan{Schema: InitialBindingSchema, OperationID: "reviewed-op", Binding: transaction.Binding{Owner: "qs.mysql", StoreID: "original", WriterEpoch: "m3-1"}, RouteRevision: "m3-1", ExpiresAt: time.Now().UTC().Add(time.Hour), StoreSchemaSHA256: strings.Repeat("a", 64), Original: OfflineSnapshot{SourceSHA: strings.Repeat("b", 40), MigrationResourcesSHA256: r.sha256, MySQLDatabase: "original_sql", MongoDatabase: "original_mongo", MySQLIdentitySHA256: strings.Repeat("c", 64), MongoClusterSHA256: strings.Repeat("d", 64), MySQLVersion: r.sql.head, MongoVersion: r.mongo.head, TargetMySQLVersion: r.sql.head, TargetMongoVersion: r.mongo.head}}
}
func TestInitialBindingExactFrozenOriginalPlan(t *testing.T) {
	p := initialPlanFixture(t)
	if err := p.Validate(p.Original.SourceSHA, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*InitialBindingPlan)
	}{
		{"owner", func(p *InitialBindingPlan) { p.Binding.Owner = "ai.mysql" }},
		{"route", func(p *InitialBindingPlan) { p.RouteRevision = "m3-2" }},
		{"expired", func(p *InitialBindingPlan) { p.ExpiresAt = time.Now().UTC().Add(-time.Second) }},
		{"unbounded", func(p *InitialBindingPlan) { p.ExpiresAt = time.Now().UTC().Add(3 * time.Hour) }},
		{"store-bytes", func(p *InitialBindingPlan) { p.Binding.StoreID = strings.Repeat("界", 43) }},
		{"epoch", func(p *InitialBindingPlan) { p.Binding.WriterEpoch = "\x00" }},
		{"zero-digest", func(p *InitialBindingPlan) { p.StoreSchemaSHA256 = strings.Repeat("0", 64) }},
		{"unknown-primary", func(p *InitialBindingPlan) { p.Original.MySQLDatabase = "unknown" }},
		{"old-pair", func(p *InitialBindingPlan) { p.Original.MongoVersion-- }},
		{"resources", func(p *InitialBindingPlan) { p.Original.MigrationResourcesSHA256 = strings.Repeat("f", 64) }},
		{"missing-identity", func(p *InitialBindingPlan) { p.Original.MongoClusterSHA256 = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := initialPlanFixture(t)
			tc.mutate(&p)
			if err := p.Validate(strings.Repeat("b", 40), time.Now().UTC()); err == nil {
				t.Fatal("invalid original plan accepted")
			}
		})
	}
	if _, err := PreviewInitialBinding(context.Background(), nil, nil, PairConfig{}, p.Binding, "op"); err == nil {
		t.Fatal("unbounded preview attempted")
	}
	result, err := ApplyInitialBinding(context.Background(), nil, nil, PairConfig{ExpectedSourceSHA: p.Original.SourceSHA}, p, false)
	if err == nil || result.Outcome != "not_attempted" || result.Verified || result.CutoverAuthorized {
		t.Fatal(result, err)
	}
}
