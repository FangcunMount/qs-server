package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	mongoRuleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	mongoScale "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
)

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatalf("seed interpretation models failed: %v", err)
	}
}

type config struct {
	mongoURI     string
	mongoDB      string
	apply        bool
	skipEmbedded bool
	skipScales   bool
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.BoolVar(&cfg.apply, "apply", false, "write interpretation models to MongoDB (default dry-run)")
	flag.BoolVar(&cfg.skipEmbedded, "skip-embedded", false, "skip embedded SBTI/MBTI rules")
	flag.BoolVar(&cfg.skipScales, "skip-scales", false, "skip backfill from published scale snapshots")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	ctx := context.Background()
	snapshots, err := collectSnapshots(ctx, cfg)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		fmt.Printf("plan: upsert %s/%s@%s -> questionnaire %s@%s\n",
			snapshot.Definition.Kind,
			snapshot.Definition.Code,
			snapshot.Definition.Version,
			snapshot.Binding.QuestionnaireCode,
			snapshot.Binding.QuestionnaireVersion,
		)
	}
	if !cfg.apply {
		fmt.Println("dry-run complete (pass --apply to write)")
		return nil
	}
	if cfg.mongoURI == "" {
		return fmt.Errorf("mongo-uri is required (or set MONGO_URI)")
	}

	applyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	client, err := mongo.Connect(applyCtx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	repo := mongoRuleset.NewRepository(client.Database(cfg.mongoDB))
	for _, snapshot := range snapshots {
		if err := repo.UpsertPublished(applyCtx, snapshot); err != nil {
			return fmt.Errorf("upsert %s@%s: %w", snapshot.Definition.Code, snapshot.Definition.Version, err)
		}
	}
	fmt.Printf("seeded %d interpretation model(s)\n", len(snapshots))
	return nil
}

func collectSnapshots(ctx context.Context, cfg config) ([]*domain.RuleSetSnapshot, error) {
	var snapshots []*domain.RuleSetSnapshot
	if !cfg.skipEmbedded {
		embedded, err := rulesetInfra.DefaultEmbeddedSnapshots(ctx)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, embedded...)
	}
	if cfg.skipScales || cfg.mongoURI == "" {
		return snapshots, nil
	}
	scaleSnapshots, err := loadPublishedScaleSnapshots(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return append(snapshots, scaleSnapshots...), nil
}

func loadPublishedScaleSnapshots(ctx context.Context, cfg config) ([]*domain.RuleSetSnapshot, error) {
	applyCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	client, err := mongo.Connect(applyCtx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return nil, fmt.Errorf("connect mongo for scale backfill: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	scaleRepo := mongoScale.NewRepository(client.Database(cfg.mongoDB))
	return rulesetInfra.PublishedScaleRuleSetSnapshots(ctx, scaleRepo)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
