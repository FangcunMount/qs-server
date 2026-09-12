package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestScopedClinicianMetricsExcludeTransferredSubjects(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		"CREATE TABLE clinician(id INTEGER,org_id INTEGER,store_id INTEGER,name TEXT,department TEXT,title TEXT,clinician_type TEXT,is_active INTEGER,deleted_at DATETIME)",
		"CREATE TABLE testee(id INTEGER,org_id INTEGER,store_id INTEGER,deleted_at DATETIME)",
		"CREATE TABLE assessment_entry(id INTEGER,org_id INTEGER,clinician_id INTEGER,is_active INTEGER,deleted_at DATETIME,token TEXT,target_type TEXT,target_code TEXT,target_version TEXT,expires_at DATETIME,created_at DATETIME)",
		"CREATE TABLE clinician_relation(org_id INTEGER,clinician_id INTEGER,testee_id INTEGER,relation_type TEXT,is_active INTEGER,deleted_at DATETIME)",
		"CREATE TABLE statistics_access_fact(org_id INTEGER,clinician_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT,stat_date DATETIME)",
		"CREATE TABLE statistics_assessment_fact(org_id INTEGER,clinician_id INTEGER,testee_id INTEGER,entry_id INTEGER,fact_type TEXT,stat_date DATETIME)",
		"INSERT INTO clinician VALUES(10,1,7,'A','','','doctor',1,NULL),(11,1,7,'B','','','doctor',1,NULL),(12,1,8,'C','','','doctor',1,NULL)",
		"INSERT INTO assessment_entry(id,org_id,clinician_id,is_active) VALUES(20,1,10,1),(21,1,12,1)",
		"INSERT INTO testee VALUES(1,1,7,NULL),(2,1,8,NULL)",
		"INSERT INTO clinician_relation VALUES(1,10,1,'primary',1,NULL),(1,10,2,'primary',1,NULL)",
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Exec("INSERT INTO statistics_assessment_fact VALUES(1,10,1,20,'report_generated',?),(1,10,2,20,'report_generated',?)", from, from).Error)
	store := NewReadStore(db, nil)
	entries, entryTotal, err := store.ScopedEntries(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, nil, nil, nil, from, from.AddDate(0, 0, 1), 1, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, entryTotal)
	require.Len(t, entries, 1)
	require.EqualValues(t, 20, entries[0].ID)
	require.EqualValues(t, 1, entries[0].ReportGeneratedCount)
	foreign := uint64(21)
	entries, entryTotal, err = store.ScopedEntries(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, &foreign, nil, nil, from, from.AddDate(0, 0, 1), 1, 10)
	require.NoError(t, err)
	require.Zero(t, entryTotal)
	require.Empty(t, entries)

	items, total, err := store.ScopedClinicians(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, nil, nil, from, from.AddDate(0, 0, 1), 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 1)
	require.EqualValues(t, 10, items[0].ID)
	require.EqualValues(t, 1, items[0].ReportGeneratedCount)
	require.EqualValues(t, 1, items[0].PrimaryTesteeCount)
	require.NoError(t, db.Exec("UPDATE testee SET store_id=8 WHERE id=1").Error)
	entries, entryTotal, err = store.ScopedEntries(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, nil, nil, nil, from, from.AddDate(0, 0, 1), 1, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, entryTotal)
	require.Zero(t, entries[0].ReportGeneratedCount)

	items, total, err = store.ScopedClinicians(context.Background(), 1, authz.StoreRange{StoreIDs: []uint64{7}}, nil, nil, from, from.AddDate(0, 0, 1), 1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Zero(t, items[0].ReportGeneratedCount)
	require.Zero(t, items[0].PrimaryTesteeCount)
}
