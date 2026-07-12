// backfill_interpretation_report_catalog builds the compact assessment-level
// report query catalog from immutable archives and current report artifacts.
// It is dry-run by default and is safe to resume and repeat.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	mongoURI, mongoDB, mysqlDSN, source string
	pageSize                            int64
	afterID                             uint64
	apply, verifyOnly                   bool
}
type summary struct{ scanned, inserted, updated, unchanged, missingTestee, missingOrg, conflict, failed int64 }

func main() {
	var c config
	flag.StringVar(&c.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&c.mongoDB, "mongo-db", os.Getenv("MONGO_DB"), "MongoDB database")
	flag.StringVar(&c.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	flag.StringVar(&c.source, "source", "all", "archive|artifact|all")
	flag.Int64Var(&c.pageSize, "page-size", 500, "documents per page")
	flag.Uint64Var(&c.afterID, "after-id", 0, "resume after domain_id")
	flag.BoolVar(&c.apply, "apply", false, "write catalog entries")
	flag.BoolVar(&c.verifyOnly, "verify-only", false, "only reconcile catalog")
	flag.Parse()
	if c.mongoURI == "" || c.mongoDB == "" || c.mysqlDSN == "" || c.pageSize < 1 || (c.source != "archive" && c.source != "artifact" && c.source != "all") {
		log.Fatal("mongo-uri, mongo-db, mysql-dsn, positive page-size and valid source are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(c.mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	mysqlDB, err := sql.Open("mysql", c.mysqlDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer mysqlDB.Close()
	db := client.Database(c.mongoDB)
	if err := ensureIndexes(ctx, db); err != nil {
		log.Fatal(err)
	}
	if c.verifyOnly {
		if err := verify(ctx, db); err != nil {
			log.Fatal(err)
		}
		return
	}
	var s summary
	if c.source == "archive" || c.source == "all" {
		if err := backfillArchive(ctx, db, mysqlDB, c, &s); err != nil {
			log.Fatal(err)
		}
	}
	if c.source == "artifact" || c.source == "all" {
		if err := backfillArtifact(ctx, db, c, &s); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("mode=%s source=%s scanned=%d inserted=%d updated=%d unchanged=%d missing_testee=%d missing_org=%d conflict=%d failed=%d\n", map[bool]string{true: "apply", false: "dry-run"}[c.apply], c.source, s.scanned, s.inserted, s.updated, s.unchanged, s.missingTestee, s.missingOrg, s.conflict, s.failed)
}

func ensureIndexes(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("report_query_catalog").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "assessment_id", Value: 1}}, Options: options.Index().SetName("uk_report_catalog_assessment").SetUnique(true)},
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "sort_at", Value: -1}, {Key: "assessment_id", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_org_sort")},
		{Keys: bson.D{{Key: "testee_id", Value: 1}, {Key: "sort_at", Value: -1}, {Key: "assessment_id", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_testee_sort")},
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "model_code", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_org_model_sort")},
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "risk_level", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_org_risk_sort")},
		{Keys: bson.D{{Key: "testee_id", Value: 1}, {Key: "model_code", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_testee_model_sort")},
		{Keys: bson.D{{Key: "testee_id", Value: 1}, {Key: "risk_level", Value: 1}, {Key: "sort_at", Value: -1}}, Options: options.Index().SetName("idx_report_catalog_testee_risk_sort")},
	})
	return err
}

func backfillArchive(ctx context.Context, db *mongo.Database, my *sql.DB, c config, s *summary) error {
	source, target := db.Collection("archived_reports"), db.Collection("report_query_catalog")
	after := c.afterID
	for {
		cur, err := source.Find(ctx, bson.M{"domain_id": bson.M{"$gt": after}}, options.Find().SetSort(bson.D{{Key: "domain_id", Value: 1}}).SetLimit(c.pageSize))
		if err != nil {
			return err
		}
		var docs []bson.M
		if err = cur.All(ctx, &docs); err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		orgs, err := loadOrgs(ctx, my, docs)
		if err != nil {
			return err
		}
		for _, d := range docs {
			s.scanned++
			id := asUint64(d["domain_id"])
			after = id
			testee := asUint64(d["testee_id"])
			org, ok := orgs[testee]
			if !ok {
				s.missingTestee++
				continue
			}
			if org == 0 {
				s.missingOrg++
				continue
			}
			entry := bson.M{"assessment_id": id, "org_id": org, "testee_id": testee, "source_kind": "archive", "source_id": id, "model_code": asString(d["scale_code"]), "risk_level": asString(d["risk_level"]), "sort_at": asTime(d["created_at"]), "sort_report_id": uint64(0), "updated_at": time.Now().UTC()}
			if !c.apply {
				continue
			}
			res, err := target.UpdateOne(ctx, bson.M{"assessment_id": id}, bson.M{"$setOnInsert": entry}, options.Update().SetUpsert(true))
			if err != nil {
				s.failed++
				return err
			}
			if res.UpsertedCount > 0 {
				s.inserted++
			} else {
				s.unchanged++
			}
		}
	}
}

func backfillArtifact(ctx context.Context, db *mongo.Database, c config, s *summary) error {
	source, target := db.Collection("interpret_report_artifacts"), db.Collection("report_query_catalog")
	after := c.afterID
	for {
		cur, err := source.Find(ctx, bson.M{"domain_id": bson.M{"$gt": after}}, options.Find().SetSort(bson.D{{Key: "domain_id", Value: 1}}).SetLimit(c.pageSize))
		if err != nil {
			return err
		}
		var docs []bson.M
		if err = cur.All(ctx, &docs); err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		for _, d := range docs {
			s.scanned++
			rid := asUint64(d["domain_id"])
			after = rid
			assessment := asUint64(d["assessment_id"])
			at := asTime(d["generated_at"])
			entry := bson.M{"assessment_id": assessment, "org_id": asInt64(d["org_id"]), "testee_id": asUint64(d["testee_id"]), "source_kind": "artifact", "source_id": rid, "model_code": asString(d["scale_code"]), "risk_level": asString(d["risk_level"]), "sort_at": at, "sort_report_id": rid, "updated_at": time.Now().UTC()}
			if !c.apply {
				continue
			}
			filter := bson.M{"assessment_id": assessment, "$or": bson.A{bson.M{"source_kind": "archive"}, bson.M{"sort_at": bson.M{"$lt": at}}, bson.M{"sort_at": at, "sort_report_id": bson.M{"$lt": rid}}}}
			res, err := target.UpdateOne(ctx, filter, bson.M{"$set": entry}, options.Update().SetUpsert(true))
			if mongo.IsDuplicateKeyError(err) {
				s.unchanged++
				continue
			}
			if err != nil {
				s.failed++
				return err
			}
			if res.UpsertedCount > 0 {
				s.inserted++
			} else if res.ModifiedCount > 0 {
				s.updated++
			} else {
				s.unchanged++
			}
		}
	}
}

func loadOrgs(ctx context.Context, db *sql.DB, docs []bson.M) (map[uint64]int64, error) {
	ids := make([]string, 0, len(docs))
	seen := map[uint64]bool{}
	for _, d := range docs {
		id := asUint64(d["testee_id"])
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, "?")
		}
	}
	args := make([]any, 0, len(seen))
	for id := range seen {
		args = append(args, id)
	}
	out := map[uint64]int64{}
	if len(args) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, "SELECT id, org_id FROM testee WHERE id IN ("+strings.Join(ids, ",")+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uint64
		var org int64
		if err := rows.Scan(&id, &org); err != nil {
			return nil, err
		}
		out[id] = org
	}
	return out, rows.Err()
}

func verify(ctx context.Context, db *mongo.Database) error {
	cat := db.Collection("report_query_catalog")
	total, err := cat.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	missingOrg, err := cat.CountDocuments(ctx, bson.M{"org_id": bson.M{"$lte": 0}})
	if err != nil {
		return err
	}
	missingTestee, err := cat.CountDocuments(ctx, bson.M{"testee_id": bson.M{"$lte": 0}})
	if err != nil {
		return err
	}
	wrongPriority, err := aggregateCount(ctx, db.Collection("interpret_report_artifacts"), mongo.Pipeline{
		{{Key: "$sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}}},
		{{Key: "$group", Value: bson.M{"_id": "$assessment_id", "source_id": bson.M{"$first": "$domain_id"}}}},
		{{Key: "$lookup", Value: bson.M{"from": "report_query_catalog", "localField": "_id", "foreignField": "assessment_id", "as": "catalog"}}},
		{{Key: "$unwind", Value: bson.M{"path": "$catalog", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$match", Value: bson.M{"$expr": bson.M{"$or": bson.A{bson.M{"$ne": bson.A{"$catalog.source_kind", "artifact"}}, bson.M{"$ne": bson.A{"$catalog.source_id", "$source_id"}}}}}}},
	})
	if err != nil {
		return err
	}
	danglingArtifact, err := aggregateCount(ctx, cat, mongo.Pipeline{{{Key: "$match", Value: bson.M{"source_kind": "artifact"}}}, {{Key: "$lookup", Value: bson.M{"from": "interpret_report_artifacts", "localField": "source_id", "foreignField": "domain_id", "as": "source"}}}, {{Key: "$match", Value: bson.M{"source": bson.M{"$size": 0}}}}})
	if err != nil {
		return err
	}
	danglingArchive, err := aggregateCount(ctx, cat, mongo.Pipeline{{{Key: "$match", Value: bson.M{"source_kind": "archive"}}}, {{Key: "$lookup", Value: bson.M{"from": "archived_reports", "localField": "source_id", "foreignField": "domain_id", "as": "source"}}}, {{Key: "$match", Value: bson.M{"source": bson.M{"$size": 0}}}}})
	if err != nil {
		return err
	}
	missingArchive, err := aggregateCount(ctx, db.Collection("archived_reports"), mongo.Pipeline{{{Key: "$lookup", Value: bson.M{"from": "report_query_catalog", "localField": "domain_id", "foreignField": "assessment_id", "as": "catalog"}}}, {{Key: "$match", Value: bson.M{"catalog": bson.M{"$size": 0}}}}})
	if err != nil {
		return err
	}
	fmt.Printf("verify catalog=%d missing_org=%d missing_testee=%d missing_archive=%d wrong_priority=%d dangling_source=%d\n", total, missingOrg, missingTestee, missingArchive, wrongPriority, danglingArtifact+danglingArchive)
	if missingOrg+missingTestee+missingArchive+wrongPriority+danglingArtifact+danglingArchive > 0 {
		return fmt.Errorf("catalog reconciliation failed")
	}
	return nil
}
func aggregateCount(ctx context.Context, c *mongo.Collection, p mongo.Pipeline) (int64, error) {
	p = append(p, bson.D{{Key: "$count", Value: "count"}})
	cur, err := c.Aggregate(ctx, p)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	if !cur.Next(ctx) {
		return 0, cur.Err()
	}
	var row struct {
		Count int64 `bson:"count"`
	}
	if err := cur.Decode(&row); err != nil {
		return 0, err
	}
	return row.Count, nil
}
func asUint64(v any) uint64 {
	switch n := v.(type) {
	case int64:
		return uint64(n)
	case int32:
		return uint64(n)
	case uint64:
		return n
	case float64:
		return uint64(n)
	}
	return 0
}
func asInt64(v any) int64   { return int64(asUint64(v)) }
func asString(v any) string { s, _ := v.(string); return s }
func asTime(v any) time.Time {
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Time{}
}
