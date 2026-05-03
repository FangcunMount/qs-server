package assembler

import (
	"testing"

	"gorm.io/gorm"
)

func TestNormalizeStatisticsModuleDepsRequiresMySQLDB(t *testing.T) {
	t.Parallel()

	if _, err := normalizeStatisticsModuleDeps(StatisticsModuleDeps{}); err == nil {
		t.Fatal("normalizeStatisticsModuleDeps() error = nil, want missing MySQL error")
	}
}

func TestNewStatisticsModuleBuildsServicesWithoutExposingCache(t *testing.T) {
	t.Parallel()

	module, err := NewStatisticsModule(StatisticsModuleDeps{MySQLDB: &gorm.DB{}})
	if err != nil {
		t.Fatalf("NewStatisticsModule() error = %v", err)
	}
	if module.ReadService == nil || module.SystemStatisticsService == nil || module.SyncService == nil {
		t.Fatalf("statistics services not initialized: %#v", module)
	}
}
