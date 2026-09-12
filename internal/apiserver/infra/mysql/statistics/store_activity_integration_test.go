package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"os"
	"testing"
	"time"
)

func TestStoreActivityRealDatabaseRebuildAndScope(t *testing.T) {
	raw, uri := os.Getenv("QS_SERVER_TEST_MYSQL_DSN"), os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if raw == "" || uri == "" {
		if os.Getenv("STATISTICS_DATABASE_REQUIRED") == "1" {
			t.Fatal("both isolated databases required")
		}
		t.Skip("isolated databases not configured")
	}
	cfg, e := driver.ParseDSN(raw)
	require.NoError(t, e)
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.MultiStatements = true
	cfg.Loc = domain.Shanghai
	admin, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{})
	require.NoError(t, e)
	name := fmt.Sprintf("qs_statistics_activity_test_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() { _ = admin.Exec("DROP DATABASE " + name).Error; p, _ := admin.DB(); _ = p.Close() })
	cfg.DBName = name
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{})
	require.NoError(t, e)
	t.Cleanup(func() { p, _ := db.DB(); _ = p.Close() })
	client, e := mongo.Connect(t.Context(), options.Client().ApplyURI(uri))
	require.NoError(t, e)
	mdb := client.Database(name)
	t.Cleanup(func() { _ = mdb.Drop(context.Background()); _ = client.Disconnect(context.Background()) })
	ddl, e := os.ReadFile("../../../../pkg/migration/migrations/mysql/000082_statistics_store_activity.up.sql")
	require.NoError(t, e)
	require.NoError(t, db.Exec(string(ddl)).Error)
	for _, sql := range []string{"CREATE TABLE assessment(id BIGINT PRIMARY KEY,org_id BIGINT,conducting_context JSON)", "CREATE TABLE evaluation_outcome(id BIGINT PRIMARY KEY,assessment_id BIGINT UNIQUE,org_id BIGINT,evaluated_at DATETIME(3))", "CREATE TABLE actor_stores(id BIGINT PRIMARY KEY,org_id BIGINT,code VARCHAR(10),name VARCHAR(30),is_active BOOLEAN)", "CREATE TABLE testee(id BIGINT PRIMARY KEY,org_id BIGINT,store_id BIGINT,deleted_at DATETIME)", "INSERT INTO actor_stores VALUES(10,1,'A','A店',false),(20,1,'B','B店',true),(30,2,'C','他公司',true)", "INSERT INTO testee VALUES(1,1,20,NULL)"} {
		require.NoError(t, db.Exec(sql).Error)
	}
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, domain.Shanghai)
	to := from.AddDate(0, 2, 0)
	sid := uint64(10)
	start := activityStart{ID: 101, StartedAt: from, StoreID: &sid, OwnershipVersion: 1, Version: 1}
	encoded, e := json.Marshal(start)
	require.NoError(t, e)
	require.NoError(t, db.Exec("INSERT INTO assessment VALUES(1,1,?)", string(encoded)).Error)
	require.NoError(t, db.Exec("INSERT INTO evaluation_outcome VALUES(1,1,1,?)", from.AddDate(0, 1, 0)).Error)
	_, e = mdb.Collection("answersheets").InsertMany(t.Context(), []any{bson.M{"domain_id": uint64(1), "org_id": uint64(1), "filled_at": from.AddDate(0, 0, 2), "start_context": start}, bson.M{"domain_id": uint64(2), "org_id": uint64(1), "filled_at": from.AddDate(0, 0, 2)}, bson.M{"domain_id": uint64(3), "org_id": uint64(2), "filled_at": from}})
	require.NoError(t, e)
	collector := NewStoreActivityCollector(db, mdb)
	request := domain.CollectRequest{OrgID: 1, Window: domain.InstantRange{From: from, To: to}, Mode: domain.CollectModeNormal}
	first, e := collector.Collect(t.Context(), request)
	require.NoError(t, e)
	require.EqualValues(t, 3, first.InsertedCount)
	second, e := collector.Collect(t.Context(), request)
	require.NoError(t, e)
	require.Zero(t, second.InsertedCount)
	require.EqualValues(t, 3, second.ExistingCount)
	projection := &StoreActivityDailyProjection{db: db}
	_, e = projection.Project(t.Context(), domain.ProjectionRequest{OrgID: 1, Window: request.Window})
	require.NoError(t, e)
	reader := NewReadStore(db, nil)
	pop, e := reader.OperationsPopulation(t.Context(), 1, authz.StoreRange{AllStores: true})
	require.NoError(t, e)
	require.Len(t, pop, 2)
	_, crossErr := reader.OperationsPopulation(t.Context(), 1, authz.StoreRange{StoreIDs: []uint64{30}})
	require.True(t, cberrors.IsCode(crossErr, code.ErrPermissionDenied), "cross-company store must be denied")

	require.EqualValues(t, 0, pop[0].CurrentServiceCount)
	require.EqualValues(t, 1, pop[1].CurrentServiceCount)
	a, e := reader.OperationsActivity(t.Context(), 1, authz.StoreRange{StoreIDs: []uint64{10}}, from, to)
	require.NoError(t, e)
	require.Len(t, a, 2)
	require.EqualValues(t, 1, a[0].Submissions)
	require.Zero(t, a[0].Completions)
	require.EqualValues(t, 1, a[1].Completions)
	b, e := reader.OperationsActivity(t.Context(), 1, authz.StoreRange{StoreIDs: []uint64{20}}, from, to)
	require.NoError(t, e)
	require.Empty(t, b)
	all, e := reader.OperationsActivity(t.Context(), 1, authz.StoreRange{AllStores: true}, from, to)
	require.NoError(t, e)
	require.Len(t, all, 3)
	// Frozen legacy unknown cannot be rewritten to a known store without a conflict.
	_, e = mdb.Collection("answersheets").UpdateOne(t.Context(), bson.M{"domain_id": uint64(2)}, bson.M{"$set": bson.M{"start_context": start}})
	require.NoError(t, e)
	_, e = collector.Collect(t.Context(), request)
	require.Error(t, e)
}
