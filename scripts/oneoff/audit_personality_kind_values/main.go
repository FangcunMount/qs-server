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

	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
)

func main() {
	mongoURI := flag.String("mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
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
	repo := mongomodelcatalog.NewRepository(db)

	publishedKind, err := repo.Collection().CountDocuments(ctx, bson.M{
		"model_kind": "personality",
		"deleted_at": bson.M{"$exists": false},
	})
	if err != nil {
		log.Fatalf("count published model_kind=personality: %v", err)
	}
	publishedChannel, err := repo.Collection().CountDocuments(ctx, bson.M{
		"model_product_channel": "personality",
		"deleted_at":            bson.M{"$exists": false},
	})
	if err != nil {
		log.Fatalf("count published model_product_channel=personality: %v", err)
	}
	draftKind, err := db.Collection("assessment_model_drafts").CountDocuments(ctx, bson.M{
		"kind": "personality",
	})
	if err != nil {
		log.Fatalf("count draft kind=personality: %v", err)
	}
	draftChannel, err := db.Collection("assessment_model_drafts").CountDocuments(ctx, bson.M{
		"product_channel": "personality",
	})
	if err != nil {
		log.Fatalf("count draft product_channel=personality: %v", err)
	}

	fmt.Printf("published_assessment_models model_kind=personality: %d\n", publishedKind)
	fmt.Printf("published_assessment_models model_product_channel=personality: %d\n", publishedChannel)
	fmt.Printf("assessment_model_drafts kind=personality: %d\n", draftKind)
	fmt.Printf("assessment_model_drafts product_channel=personality: %d\n", draftChannel)
	total := publishedKind + publishedChannel + draftKind + draftChannel
	fmt.Printf("legacy personality persisted values total: %d\n", total)
	if total > 0 {
		os.Exit(2)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
