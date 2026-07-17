package statistics_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	"gorm.io/gorm"
)

func TestNewRequiresMySQLDB(t *testing.T) {
	t.Parallel()

	if _, err := statistics.New(statistics.Deps{}); err == nil {
		t.Fatal("New() error = nil, want missing MySQL error")
	}
}

func TestNewBuildsServicesWithoutExposingCache(t *testing.T) {
	t.Parallel()

	module, err := statistics.New(statistics.Deps{MySQLDB: &gorm.DB{}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if module.ReadService == nil || module.SyncService == nil {
		t.Fatalf("statistics services not initialized: %#v", module)
	}
}
