// backfill_typology_algorithm_identity rewrites retained-read typology algorithms
// (mbti/sbti/bigfive) to personality_typology when runtime equivalence is proven
// (MC-R018 batch 3).
//
// Default is dry-run. Does NOT rewrite Assessment/Outcome model_algorithm columns;
// catalog lookup treats retained aliases as equivalent to personality_typology.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

type config struct {
	mongoURI string
	mongoDB  string
	apply    bool
	jsonOut  bool
	limit    int
	timeout  time.Duration
}

type candidate struct {
	Code      string `json:"code"`
	Version   string `json:"version"`
	Algorithm string `json:"algorithm"`
	Eligible  bool   `json:"eligible"`
	Reason    string `json:"reason,omitempty"`
	Target    string `json:"target,omitempty"`
	Applied   bool   `json:"applied,omitempty"`
}

type report struct {
	Scanned    int         `json:"scanned"`
	Eligible   int         `json:"eligible"`
	Skipped    int         `json:"skipped"`
	Applied    int         `json:"applied"`
	Candidates []candidate `json:"candidates"`
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "backfill typology algorithm identity failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	result, err := run(ctx, client.Database(cfg.mongoDB), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "backfill failed:", err)
		os.Exit(1)
	}
	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(result)
		return
	}
	printReport(result, cfg.apply)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database")
	flag.BoolVar(&cfg.apply, "apply", false, "persist eligible algorithm rewrites")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON report")
	flag.IntVar(&cfg.limit, "limit", 0, "max candidates to scan (0 = all)")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "operation timeout")
	flag.Parse()
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run(ctx context.Context, db *mongo.Database, cfg config) (*report, error) {
	snapshots, err := loadRetainedTypologySnapshots(ctx, db, cfg.limit)
	if err != nil {
		return nil, err
	}
	coll := db.Collection((&mongomodelcatalog.PublishedAssessmentModelPO{}).CollectionName())
	out := &report{}
	for _, snapshot := range snapshots {
		out.Scanned++
		item := evaluateSnapshot(snapshot)
		if item.Eligible {
			out.Eligible++
			if cfg.apply {
				if err := applyRewrite(ctx, coll, snapshot, modelcatalog.Algorithm(item.Target)); err != nil {
					return nil, fmt.Errorf("apply %s@%s: %w", snapshot.Code, snapshot.Version, err)
				}
				item.Applied = true
				out.Applied++
			}
		} else {
			out.Skipped++
		}
		out.Candidates = append(out.Candidates, item)
	}
	sort.Slice(out.Candidates, func(i, j int) bool {
		if out.Candidates[i].Eligible != out.Candidates[j].Eligible {
			return out.Candidates[i].Eligible
		}
		return out.Candidates[i].Code < out.Candidates[j].Code
	})
	return out, nil
}

func loadRetainedTypologySnapshots(ctx context.Context, db *mongo.Database, limit int) ([]*modelcatalogport.PublishedModel, error) {
	filter := bson.M{
		"deleted_at":  nil,
		"record_role": "published_snapshot",
		"kind":        string(modelcatalog.KindTypology),
		"algorithm": bson.M{"$in": bson.A{
			string(modelcatalog.AlgorithmMBTI),
			string(modelcatalog.AlgorithmSBTI),
			string(modelcatalog.AlgorithmBigFive),
		}},
	}
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}, {Key: "release_version", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	cursor, err := db.Collection((&mongomodelcatalog.PublishedAssessmentModelPO{}).CollectionName()).Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	mapper := mongomodelcatalog.NewMapper()
	out := make([]*modelcatalogport.PublishedModel, 0)
	for cursor.Next(ctx) {
		var po mongomodelcatalog.PublishedAssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		out = append(out, mapper.ToPublished(&po))
	}
	return out, cursor.Err()
}

func evaluateSnapshot(snapshot *modelcatalogport.PublishedModel) candidate {
	item := candidate{
		Code: snapshot.Code, Version: snapshot.Version, Algorithm: string(snapshot.Algorithm),
	}
	var payload *modeltypology.Payload
	if len(snapshot.Payload) > 0 {
		var parsed modeltypology.Payload
		if err := json.Unmarshal(snapshot.Payload, &parsed); err == nil {
			payload = &parsed
		}
	}
	eligibility := modeltypology.EvaluateAlgorithmBackfill(snapshot.Algorithm, snapshot.DefinitionV2, payload)
	item.Eligible = eligibility.Eligible
	item.Reason = eligibility.Reason
	item.Target = string(eligibility.To)
	return item
}

func applyRewrite(ctx context.Context, coll *mongo.Collection, snapshot *modelcatalogport.PublishedModel, target modelcatalog.Algorithm) error {
	set := bson.M{
		"algorithm":  string(target),
		"updated_at": time.Now().UTC(),
	}
	if len(snapshot.Payload) > 0 {
		var body map[string]any
		if err := json.Unmarshal(snapshot.Payload, &body); err == nil {
			body["algorithm"] = string(target)
			if raw, err := json.Marshal(body); err == nil {
				set["payload"] = raw
			}
		}
	}
	res, err := coll.UpdateOne(ctx, bson.M{
		"deleted_at":      nil,
		"record_role":     "published_snapshot",
		"kind":            string(modelcatalog.KindTypology),
		"code":            snapshot.Code,
		"release_version": snapshot.Version,
		"algorithm":       string(snapshot.Algorithm),
	}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no document matched for update")
	}
	return nil
}

func printReport(r *report, apply bool) {
	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	fmt.Printf("mode=%s scanned=%d eligible=%d skipped=%d applied=%d\n", mode, r.Scanned, r.Eligible, r.Skipped, r.Applied)
	for _, c := range r.Candidates {
		status := "SKIP"
		if c.Eligible {
			status = "OK"
		}
		if c.Applied {
			status = "APPLIED"
		}
		fmt.Printf("  %-8s %-28s %s -> %s %s\n", status, c.Code+"@"+c.Version, c.Algorithm, c.Target, c.Reason)
	}
}
