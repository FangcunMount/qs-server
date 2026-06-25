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

	interpretationmodelInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/interpretationmodel"
	mongoInterpretationmodel "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretationmodel"
)

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		log.Fatalf("seed interpretation models failed: %v", err)
	}
}

type config struct {
	mongoURI string
	mongoDB  string
	apply    bool
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.BoolVar(&cfg.apply, "apply", false, "write interpretation models to MongoDB (default dry-run)")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	snapshots, err := interpretationmodelInfra.DefaultEmbeddedSnapshots(context.Background())
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	repo := mongoInterpretationmodel.NewRepository(client.Database(cfg.mongoDB))
	for _, snapshot := range snapshots {
		if err := repo.UpsertPublished(ctx, snapshot); err != nil {
			return fmt.Errorf("upsert %s@%s: %w", snapshot.Definition.Code, snapshot.Definition.Version, err)
		}
	}
	fmt.Printf("seeded %d interpretation model(s)\n", len(snapshots))
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
