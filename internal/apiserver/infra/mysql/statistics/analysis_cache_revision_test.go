package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestAnalysisCacheRevisionTracksCurrentInputsWithoutHistoricalFacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	createScopedClinicianFixture(t, db)
	require.NoError(t, db.Exec("CREATE TABLE plan_enrollment(org_id INTEGER,testee_id INTEGER,status TEXT,deleted_at DATETIME)").Error)
	assertAnalysisCacheRevision(t, db)
}
func assertAnalysisCacheRevision(t *testing.T, db *gorm.DB) {
	t.Helper()
	s := NewReadStore(db, nil)
	scope := authz.StoreRange{StoreIDs: []uint64{7}}
	revision := func(kind string) string {
		t.Helper()
		v, err := s.AnalysisCacheRevision(context.Background(), 1, scope, kind)
		require.NoError(t, err)
		require.Len(t, v, 64)
		return v
	}
	before := revision("overview")
	require.Equal(t, before, revision("overview"))
	// Preserve the population count while swapping two owners: count-only cache
	// invalidation would incorrectly keep the previous history visible.
	require.NoError(t, db.Exec("UPDATE testee SET store_id=CASE WHEN id=1 THEN 8 ELSE 7 END WHERE id IN (1,2)").Error)
	require.NotEqual(t, before, revision("overview"))
	before = revision("clinicians")
	require.NoError(t, db.Exec("UPDATE clinician SET store_id=8 WHERE id=10").Error)
	require.NotEqual(t, before, revision("clinicians"))
	before = revision("entries")
	require.NoError(t, db.Exec("UPDATE assessment_entry SET target_code='changed' WHERE id=20").Error)
	require.Equal(t, before, revision("entries"), "out-of-scope entry should not invalidate")
	before = revision("clinicians")
	require.NoError(t, db.Exec("UPDATE clinician SET name='changed' WHERE id=11").Error)
	require.NotEqual(t, before, revision("clinicians"))
	before = revision("overview")
	require.NoError(t, db.Exec("INSERT INTO plan_enrollment VALUES(1,2,'active',NULL)").Error)
	require.NotEqual(t, before, revision("overview"))
	before = revision("clinicians")
	require.NoError(t, db.Exec("INSERT INTO clinician_relation VALUES(1,11,2,'primary',1,NULL)").Error)
	require.NotEqual(t, before, revision("clinicians"))
	before = revision("overview")
	require.NoError(t, db.Exec("UPDATE testee SET deleted_at='2026-09-13' WHERE id=2").Error)
	require.NotEqual(t, before, revision("overview"))
	_, err := s.AnalysisCacheRevision(context.Background(), 1, scope, "invalid")
	require.Error(t, err)
}
