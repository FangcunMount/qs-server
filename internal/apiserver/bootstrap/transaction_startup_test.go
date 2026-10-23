package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func TestTransactionStartupRejectsDDLBeforeDatabaseResources(t *testing.T) {
	o := options.NewOptions()
	o.MessagingOptions.Provider = "rocketmq"
	o.MessagingOptions.RocketMQ = &generic.RocketMQOptions{Transactions: map[string]generic.TransactionOptions{"qs.mysql": {}}}
	// Missing provider handles make this a pure rejection test, not a live DB test.
	o.MySQLOptions = nil
	o.MongoDBOptions = nil
	o.RedisOptions = nil
	dm := NewDatabaseManager(&config.Config{Options: o})
	if err := dm.Initialize(); !errors.Is(err, options.ErrTransactionStartupMigration) {
		t.Fatalf("startup policy not enforced before resources: %v", err)
	}
	if err := dm.runMigrations(context.Background()); !errors.Is(err, options.ErrTransactionStartupMigration) {
		t.Fatalf("direct migration path bypassed policy: %v", err)
	}
}
