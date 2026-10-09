//go:build integration

package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func nonTargetNativeSQL(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("QS_HISTORY_MYSQL_DSN")
	if raw == "" {
		if os.Getenv("QS_HISTORY_REQUIRE_DATABASE") == "1" {
			t.Fatal("required owned SQL fixture is missing")
		}
		t.Skip("owned SQL fixture not requested")
	}
	cfg, e := drivermysql.ParseDSN(raw)
	if e != nil || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:34306" {
		t.Fatal("explicit loopback SQL fixture required")
	}
	cfg.DBName, cfg.ParseTime = "", false
	admin, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal("owned SQL admin open failed")
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := "qs_non_target_test_" + primitive.NewObjectID().Hex()
	if _, e = admin.ExecContext(t.Context(), "CREATE DATABASE "+quote(name)); e != nil {
		t.Fatal("owned SQL database create failed")
	}
	t.Cleanup(func() {
		q, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, e := admin.ExecContext(q, "DROP DATABASE "+quote(name)); e != nil {
			t.Error("owned SQL database cleanup failed")
		}
	})
	cfg.DBName = name
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil || db.PingContext(t.Context()) != nil {
		t.Fatal("owned SQL database open failed")
	}
	t.Cleanup(func() {
		if db.Close() != nil {
			t.Error("owned SQL connection close failed")
		}
	})
	return db
}

// These are genuine driver/data-reader tests in newly owned namespaces. They
// do not construct Archive, Window, DROP, Pair or acceptance capabilities.
func TestNonTargetDataNativePagingDriftAndBSONIdentity(t *testing.T) {
	db := nonTargetNativeSQL(t)
	if os.Getenv("QS_HISTORY_REQUIRE_DATABASE") == "1" && os.Getenv("QS_SERVER_TEST_MONGO_URI") == "" {
		t.Fatal("required owned Mongo fixture is missing")
	}
	_, mongoDB := mongodbtest.ReplicaSetDatabase(t)
	t.Run("SQL-composite-full-page-null-empty-and-drift", func(t *testing.T) {
		nativeExec(t, db, "CREATE TABLE all_business(org BIGINT UNSIGNED NOT NULL,id VARCHAR(20) NOT NULL,body BLOB NULL,PRIMARY KEY(org,id)) ENGINE=InnoDB")
		var tuples []string
		var args []any
		for i := 0; i < 1001; i++ {
			tuples = append(tuples, "(?,?,?)")
			var body any
			if i != 0 {
				body = []byte{}
			}
			args = append(args, uint64(18446744073709551615), fmt.Sprintf("%04d", i), body)
		}
		nativeExec(t, db, "INSERT INTO all_business VALUES "+strings.Join(tuples, ","), args...)
		first, e := nonTargetSQLData(t.Context(), db, "all_business", 99)
		if e != nil || first.Rows != 1001 || first.Pages != 2 {
			t.Fatal("native composite unsigned paging did not reach full EOF")
		}
		nativeExec(t, db, "UPDATE all_business SET body=X'' WHERE id='0000'")
		changed, e := nonTargetSQLData(t.Context(), db, "all_business", 99)
		if e != nil || changed.DataSHA256 == first.DataSHA256 {
			t.Fatal("native NULL to empty drift was hidden")
		}
		nativeExec(t, db, "UPDATE all_business SET body=NULL WHERE id='0000'")
		restored, e := nonTargetSQLData(t.Context(), db, "all_business", 99)
		if e != nil || !reflect.DeepEqual(first, restored) {
			t.Fatal("restored exact original SQL baseline did not match")
		}
	})
	t.Run("Mongo-ordered-RM-document-page-and-drift", func(t *testing.T) {
		var docs []any
		for i := 0; i < 1001; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: fmt.Sprintf("%04d", i)}, {Key: "destination", Value: "qs.fixture"}}}, {Key: "fact", Value: int64(i)}})
		}
		if _, e := mongoDB.Collection("rm_outbox").InsertMany(t.Context(), docs); e != nil {
			t.Fatal("owned Mongo fixture insert failed")
		}
		first, e := nonTargetMongoData(t.Context(), mongoDB, "rm_outbox", 38)
		if e != nil || first.Rows != 1001 || first.Pages != 2 {
			t.Fatal("native ordered RM document paging did not reach full EOF")
		}
		id := docs[0].(bson.D)[0].Value
		filter := bson.D{{Key: "_id", Value: id}}
		for _, n := range []int64{-1, 0} {
			if _, e := mongoDB.Collection("rm_outbox").UpdateOne(t.Context(), filter, bson.D{{Key: "$set", Value: bson.D{{Key: "fact", Value: n}}}}); e != nil {
				t.Fatal("owned Mongo fixture update failed")
			}
			actual, e := nonTargetMongoData(t.Context(), mongoDB, "rm_outbox", 38)
			if e != nil || (n == 0) != reflect.DeepEqual(first, actual) {
				t.Fatal("native Mongo drift or exact restore comparison failed")
			}
		}
	})
	t.Run("Mongo-mixed-ID-type-cannot-hide-tail", func(t *testing.T) {
		if _, e := mongoDB.Collection("mixed_business").InsertMany(t.Context(), []any{bson.D{{Key: "_id", Value: "typed"}}, bson.D{{Key: "_id", Value: primitive.NewObjectID()}}}); e != nil {
			t.Fatal("owned mixed-ID fixture insert failed")
		}
		if _, e := nonTargetMongoData(t.Context(), mongoDB, "mixed_business", 38); e == nil {
			t.Fatal("native type bracketing hid a stored original")
		}
	})
	t.Run("Mongo-real-schema-head-only-normalization", func(t *testing.T) {
		id := primitive.NewObjectID()
		if _, e := mongoDB.Collection("schema_migrations").InsertOne(t.Context(), bson.D{{Key: "_id", Value: id}, {Key: "version", Value: int64(38)}, {Key: "dirty", Value: false}}); e != nil {
			t.Fatal("owned schema-head fixture insert failed")
		}
		first, e := nonTargetMongoData(t.Context(), mongoDB, "schema_migrations", 38)
		if e != nil {
			t.Fatal("native clean schema-head read failed")
		}
		if _, e = mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: bson.D{{Key: "version", Value: int64(39)}}}}); e != nil {
			t.Fatal("owned schema-head fixture update failed")
		}
		after, e := nonTargetMongoData(t.Context(), mongoDB, "schema_migrations", 39)
		if e != nil || !reflect.DeepEqual(first, after) {
			t.Fatal("only clean native schema-head transition did not normalize")
		}
		if _, e = mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: bson.D{{Key: "dirty", Value: true}}}}); e != nil {
			t.Fatal("owned dirty fixture update failed")
		}
		if _, e = nonTargetMongoData(t.Context(), mongoDB, "schema_migrations", 39); e == nil {
			t.Fatal("native dirty schema-head was normalized")
		}
	})
}
