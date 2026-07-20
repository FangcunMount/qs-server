// backfill_published_projection_hashes fills missing projection hashes on published
// snapshots when replay audit passes (MC-R017 batch 4).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type config struct {
	mongoURI string
	mongoDB  string
	codes    string
	apply    bool
	json     bool
	timeout  time.Duration
}

type result struct {
	Scanned  int `json:"scanned"`
	Eligible int `json:"eligible"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "backfill projection hashes failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "backfill projection hashes failed: connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	db := client.Database(cfg.mongoDB)
	repo := mongomodelcatalog.NewRepository(db)
	normRepo := mongomodelcatalog.NewNormRepository(db)
	registry := appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{},
		appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepo},
		appdefinition.CognitiveDefinitionHandler{NormRepo: normRepo},
		appdefinition.TypologyDefinitionHandler{},
	)
	stats, err := run(ctx, db, repo, registry, splitCodes(cfg.codes), cfg.apply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "backfill projection hashes failed:", err)
		os.Exit(1)
	}
	if cfg.json {
		_ = json.NewEncoder(os.Stdout).Encode(stats)
	} else {
		fmt.Printf("scanned=%d eligible=%d updated=%d skipped=%d apply=%v\n", stats.Scanned, stats.Eligible, stats.Updated, stats.Skipped, cfg.apply)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.StringVar(&cfg.codes, "codes", "", "optional comma-separated model codes")
	flag.BoolVar(&cfg.apply, "apply", false, "persist missing projection hashes")
	flag.BoolVar(&cfg.json, "json", false, "write JSON summary")
	flag.DurationVar(&cfg.timeout, "timeout", 10*time.Minute, "operation timeout")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, db *mongo.Database, repo *mongomodelcatalog.Repository, registry appdefinition.Registry, codes []string, apply bool) (*result, error) {
	snapshots, err := loadPublished(ctx, db, codes)
	if err != nil {
		return nil, err
	}
	stats := &result{Scanned: len(snapshots)}
	for _, snapshot := range snapshots {
		existingDef, existingPayload := modelcatalogport.ProjectionHashesFromSource(snapshot.Source)
		if existingDef != "" && existingPayload != "" {
			stats.Skipped++
			continue
		}
		if snapshot.DefinitionV2 == nil {
			stats.Skipped++
			continue
		}
		handler, err := registry.MustResolveBinding(appdefinition.AlgorithmBindingFromModel(publication.ModelFromPublishedSnapshot(snapshot)))
		if err != nil {
			stats.Skipped++
			continue
		}
		if issues := publication.AuditPublishedSnapshotInventory(ctx, snapshot, handler); len(issues) > 0 {
			stats.Skipped++
			continue
		}
		defHash, err := modeldefinition.CanonicalContentHash(snapshot.DefinitionV2)
		if err != nil {
			return nil, err
		}
		payloadHash := modeldefinition.PayloadProjectionHash(snapshot.Payload)
		stats.Eligible++
		if !apply {
			continue
		}
		if err := repo.BackfillPublishedProjectionHashes(ctx, snapshot, defHash, payloadHash); err != nil {
			return nil, fmt.Errorf("backfill %s@%s: %w", snapshot.Code, snapshot.Version, err)
		}
		stats.Updated++
	}
	return stats, nil
}

func loadPublished(ctx context.Context, db *mongo.Database, codes []string) ([]*modelcatalogport.PublishedModel, error) {
	filter := bson.M{"deleted_at": nil, "record_role": "published_snapshot", "status": "published", "$or": bson.A{
		bson.M{"release_status": "active"},
		bson.M{"release_status": bson.M{"$exists": false}, "is_active_published": true},
	}}
	if len(codes) > 0 {
		filter["code"] = bson.M{"$in": codes}
	}
	cursor, err := db.Collection("assessment_models").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "code", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("find published snapshots: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	mapper := mongomodelcatalog.NewMapper()
	out := make([]*modelcatalogport.PublishedModel, 0)
	for cursor.Next(ctx) {
		var po mongomodelcatalog.PublishedAssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, fmt.Errorf("decode published snapshot: %w", err)
		}
		out = append(out, mapper.ToPublished(&po))
	}
	return out, cursor.Err()
}

func splitCodes(raw string) []string {
	seen := map[string]struct{}{}
	for _, code := range strings.Split(raw, ",") {
		if code = strings.TrimSpace(code); code != "" {
			seen[code] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
