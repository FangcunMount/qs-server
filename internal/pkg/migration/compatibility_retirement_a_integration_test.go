//go:build integration

package migration

import (
	"os"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestCompatibilityRetirementAUpgradesWithOldMySQLTargetsMissing(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_COMPAT_REQUIRE_DATABASE") == "1" {
			t.Fatal("MYSQL_DSN required")
		}
		t.Skip("isolated MySQL is not configured")
	}
	targets := []string{"domain_event_outbox", "ai_bridge_commands", "ai_messaging_legacy_commands"}
	for mask := 0; mask < 8; mask++ {
		t.Run(string(rune('0'+mask)), func(t *testing.T) {
			db, name := openStatisticsMigrationDatabase(t, dsn)
			cfg := ensureConfigDefaults(&Config{Enabled: true, Database: name})
			instance, err := NewMySQLDriver(db).CreateInstance(migrations, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := instance.Migrate(95); err != nil {
				t.Fatal(err)
			}
			for i, target := range targets {
				if mask&(1<<i) != 0 {
					if _, err := db.ExecContext(t.Context(), "DROP TABLE `"+target+"`"); err != nil {
						t.Fatal(err)
					}
				}
			}
			version, changed, err := NewMigrator(db, cfg).run(98)
			if err != nil || !changed || version != 98 {
				t.Fatalf("A upgrade version=%d changed=%v error=%v", version, changed, err)
			}
			for i, target := range targets {
				assertMySQLTable(t, db, name, target, mask&(1<<i) == 0)
			}
			version, changed, err = NewMigrator(db, cfg).run(98)
			if err != nil || changed || version != 98 {
				t.Fatalf("repeated A startup version=%d changed=%v error=%v", version, changed, err)
			}
		})
	}
}
func TestCompatibilityRetirementAUpgradesWithOldMongoTargetMissing(t *testing.T) {
	if os.Getenv("QS_SERVER_TEST_MONGO_URI") == "" && os.Getenv("QS_COMPAT_REQUIRE_DATABASE") == "1" {
		t.Fatal("QS_SERVER_TEST_MONGO_URI required")
	}
	for _, absent := range []bool{false, true} {
		name := "present"
		if absent {
			name = "absent"
		}
		t.Run(name, func(t *testing.T) {
			client, db := mongodbtest.ReplicaSetDatabase(t)
			cfg := ensureConfigDefaults(&Config{Enabled: true, Database: db.Name()})
			driver := NewMongoDriver(client)
			instance, err := driver.CreateInstance(migrations, cfg)
			if err != nil {
				t.Fatal(err)
			}
			cleanup, err := driver.PrepareRun(t.Context(), cfg, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := instance.Migrate(36); err != nil {
				t.Fatal(err)
			}
			if err := cleanup(t.Context()); err != nil {
				t.Fatal(err)
			}
			if absent {
				if err := db.Collection("domain_event_outbox").Drop(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			version, changed, err := NewMongoMigrator(client, cfg).run(37)
			if err != nil || !changed || version != 37 {
				t.Fatalf("A upgrade version=%d changed=%v error=%v", version, changed, err)
			}
			names, err := db.ListCollectionNames(t.Context(), bson.M{"name": "domain_event_outbox"})
			if err != nil {
				t.Fatal(err)
			}
			if (len(names) == 0) != absent {
				t.Fatalf("A rebuilt or dropped the old collection: %v", names)
			}
			version, changed, err = NewMongoMigrator(client, cfg).run(37)
			if err != nil || changed || version != 37 {
				t.Fatalf("repeated A startup version=%d changed=%v error=%v", version, changed, err)
			}
		})
	}
}
