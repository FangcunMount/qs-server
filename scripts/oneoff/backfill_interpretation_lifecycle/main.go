// backfill_interpretation_lifecycle converts legacy generated reports into the
// Generation/Run/Artifact model. Default mode is read-only; pass --apply only
// after reviewing the reconciliation summary.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type config struct {
	uri, db   string
	apply     bool
	archiveV0 bool
	pageSize  int
}
type summary struct{ scanned, candidates, existing, backfilled, skipped, failed int }

func main() {
	var cfg config
	flag.StringVar(&cfg.uri, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI (or MONGO_URI)")
	flag.StringVar(&cfg.db, "mongo-db", os.Getenv("MONGO_DB"), "MongoDB database (or MONGO_DB)")
	flag.BoolVar(&cfg.apply, "apply", false, "write three-object records (default dry-run)")
	flag.BoolVar(&cfg.archiveV0, "archive-v0", false, "copy reports without outcome_id into archived_reports")
	flag.IntVar(&cfg.pageSize, "page-size", 500, "Mongo cursor batch size")
	flag.Parse()
	if cfg.uri == "" || cfg.db == "" || cfg.pageSize < 1 {
		log.Fatal("mongo-uri, mongo-db and positive page-size are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.uri))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := client.Database(cfg.db)
	if cfg.archiveV0 {
		if err := archiveV0(ctx, db, cfg); err != nil {
			log.Fatal(err)
		}
		return
	}
	s, err := run(ctx, db, cfg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("mode=%s scanned=%d candidates=%d existing=%d backfilled=%d skipped=%d failed=%d\n", map[bool]string{true: "apply", false: "dry-run"}[cfg.apply], s.scanned, s.candidates, s.existing, s.backfilled, s.skipped, s.failed)
}

func archiveV0(ctx context.Context, db *mongo.Database, cfg config) error {
	legacy := db.Collection("interpret_reports")
	filter := bson.M{"$or": bson.A{bson.M{"outcome_id": bson.M{"$exists": false}}, bson.M{"outcome_id": 0}}}
	count, err := legacy.CountDocuments(ctx, filter)
	if err != nil {
		return err
	}
	if !cfg.apply {
		fmt.Printf("mode=dry-run archive_v0_candidates=%d\n", count)
		return nil
	}
	archive := db.Collection("archived_reports")
	if _, err := archive.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uk_archived_report_assessment")}); err != nil {
		return err
	}
	_, err = legacy.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		// $merge replaces one archived row per domain_id. Ascending source time
		// leaves the newest legacy report as the final deterministic value.
		{{Key: "$sort", Value: bson.D{{Key: "domain_id", Value: 1}, {Key: "created_at", Value: 1}, {Key: "updated_at", Value: 1}, {Key: "_id", Value: 1}}}},
		{{Key: "$set", Value: bson.M{"archive_source": "legacy_v0", "archived_at": time.Now().UTC()}}},
		// archived_reports has its own Mongo _id. Excluding the legacy _id lets
		// $merge replace matching domain_id rows without altering that immutable
		// target identity on a rerun.
		{{Key: "$project", Value: bson.M{"_id": 0}}},
		{{Key: "$merge", Value: bson.M{"into": "archived_reports", "on": "domain_id", "whenMatched": "replace", "whenNotMatched": "insert"}}},
	})
	if err == nil {
		fmt.Printf("mode=apply archive_v0_candidates=%d\n", count)
	}
	return err
}

func run(ctx context.Context, db *mongo.Database, cfg config) (summary, error) {
	legacy, generations, runs, artifacts := db.Collection("interpret_reports"), db.Collection("report_generations"), db.Collection("interpretation_runs"), db.Collection("interpret_report_artifacts")
	filter := bson.M{"status": "generated", "outcome_id": bson.M{"$gt": 0}}
	cur, err := legacy.Find(ctx, filter, options.Find().SetBatchSize(int32(cfg.pageSize)).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return summary{}, err
	}
	defer func() { _ = cur.Close(ctx) }()
	var s summary
	for cur.Next(ctx) {
		s.scanned++
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return s, err
		}
		outcome, _ := asUint64(doc["outcome_id"])
		assessment, ok := asUint64(doc["domain_id"])
		if !ok || outcome == 0 {
			s.skipped++
			continue
		}
		s.candidates++
		key := bson.M{"outcome_id": outcome, "report_type": "standard", "template_version": "legacy-v1"}
		n, err := generations.CountDocuments(ctx, key)
		if err != nil {
			return s, err
		}
		if n > 0 {
			s.existing++
			continue
		}
		if !cfg.apply {
			continue
		}
		at := time.Now().UTC()
		if value, ok := doc["generated_at"].(time.Time); ok {
			at = value
		}
		gid, rid, aid := meta.New().Uint64(), meta.New().Uint64(), meta.New().Uint64()
		gen := bson.M{"domain_id": gid, "outcome_id": outcome, "report_type": "standard", "template_version": "legacy-v1", "status": "generated", "latest_run_id": rid, "report_id": aid, "version": uint64(3), "created_at": at, "updated_at": at}
		run := bson.M{"domain_id": rid, "generation_id": gid, "attempt": 1, "status": "succeeded", "started_at": at, "finished_at": at, "created_at": at, "updated_at": at}
		artifact := bson.M{"domain_id": aid, "generation_id": gid, "outcome_id": outcome, "interpretation_run_id": rid, "report_type": "standard", "template_version": "legacy-v1", "generated_at": at, "assessment_id": assessment, "testee_id": doc["testee_id"], "scale_name": doc["scale_name"], "scale_code": doc["scale_code"], "model": doc["model"], "primary_score": doc["primary_score"], "level": doc["level"], "total_score": doc["total_score"], "risk_level": doc["risk_level"], "conclusion": doc["conclusion"], "dimensions": doc["dimensions"], "suggestions": doc["suggestions"], "model_extra": doc["model_extra"], "created_at": at, "updated_at": at}
		sess, err := db.Client().StartSession()
		if err != nil {
			return s, err
		}
		_, err = sess.WithTransaction(ctx, func(tx mongo.SessionContext) (interface{}, error) {
			if _, e := generations.InsertOne(tx, gen); e != nil {
				return nil, e
			}
			if _, e := runs.InsertOne(tx, run); e != nil {
				return nil, e
			}
			_, e := artifacts.InsertOne(tx, artifact)
			return nil, e
		})
		sess.EndSession(ctx)
		if err != nil {
			s.failed++
			log.Printf("legacy outcome=%d assessment=%d: %v", outcome, assessment, err)
			continue
		}
		s.backfilled++
	}
	return s, cur.Err()
}

func asUint64(v any) (uint64, bool) {
	switch x := v.(type) {
	case uint64:
		return x, true
	case int64:
		return uint64(x), x > 0
	case int:
		return uint64(x), x > 0
	case int32:
		return uint64(x), x > 0
	default:
		return 0, false
	}
}
