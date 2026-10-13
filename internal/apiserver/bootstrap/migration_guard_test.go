package bootstrap

import (
	"context"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestDisabledDDLStillRequiresSelectedDatabasePair(t *testing.T) {
	opts := options.NewOptions()
	opts.MigrationOptions.Enabled = false
	dm := NewDatabaseManager(&config.Config{Options: opts})
	if err := dm.runMigrations(context.Background()); err == nil || !strings.Contains(err.Error(), "paired mysql") {
		t.Fatalf("disabled DDL bypassed database pair: %v", err)
	}
}
