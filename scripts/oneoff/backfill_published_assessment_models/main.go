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

	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
	v1envelope "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/v1envelope"
)

func main() {
	mongoURI := flag.String("mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	apply := flag.Bool("apply", false, "write backfill rows (default dry-run)")
	flag.Parse()

	if *mongoURI == "" {
		log.Fatal("mongo-uri is required (or set MONGO_URI)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	db := client.Database(*mongoDB)
	legacy := mongoruleset.NewRepository(db)
	target := mongomodelcatalog.NewRepository(db)

	rows, err := legacy.ListPublished(ctx)
	if err != nil {
		log.Fatalf("list legacy published models: %v", err)
	}
	fmt.Printf("plan: backfill %d legacy row(s) into published_assessment_models\n", len(rows))
	if !*apply {
		fmt.Println("dry-run complete (pass --apply to write)")
		return
	}

	written, err := backfillFromLegacy(ctx, legacy, target)
	if err != nil {
		log.Fatalf("backfill failed after %d row(s): %v", written, err)
	}
	fmt.Printf("backfilled %d published assessment model(s)\n", written)
}

func backfillFromLegacy(ctx context.Context, legacy *mongoruleset.Repository, target *mongomodelcatalog.Repository) (int, error) {
	if legacy == nil || target == nil {
		return 0, fmt.Errorf("legacy and target repositories are required")
	}
	rows, err := legacy.ListPublished(ctx)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, snapshot := range rows {
		if snapshot == nil {
			continue
		}
		published := v1envelope.PublishedFromV1(snapshot)
		if published == nil {
			continue
		}
		if err := target.UpsertPublishedModel(ctx, published); err != nil {
			return written, fmt.Errorf("upsert %s@%s: %w", published.Code, published.Version, err)
		}
		written++
	}
	return written, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
