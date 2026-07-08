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
)

func main() {
	mongoURI := flag.String("mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	apply := flag.Bool("apply", false, "write migrated values (default dry-run)")
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
	now := time.Now()

	plans := []struct {
		collection string
		field      string
		filter     bson.M
	}{
		{
			collection: "published_assessment_models",
			field:      "model_kind",
			filter:     bson.M{"model_kind": "personality", "deleted_at": bson.M{"$exists": false}},
		},
		{
			collection: "published_assessment_models",
			field:      "model_product_channel",
			filter:     bson.M{"model_product_channel": "personality", "deleted_at": bson.M{"$exists": false}},
		},
		{
			collection: "assessment_model_drafts",
			field:      "kind",
			filter:     bson.M{"kind": "personality"},
		},
		{
			collection: "assessment_model_drafts",
			field:      "product_channel",
			filter:     bson.M{"product_channel": "personality"},
		},
	}

	for _, plan := range plans {
		count, err := db.Collection(plan.collection).CountDocuments(ctx, plan.filter)
		if err != nil {
			log.Fatalf("count %s.%s: %v", plan.collection, plan.field, err)
		}
		fmt.Printf("plan: migrate %d row(s) in %s.%s personality -> typology\n", count, plan.collection, plan.field)
		if count == 0 || !*apply {
			continue
		}
		set := bson.M{"updated_at": now}
		switch plan.field {
		case "model_kind", "kind":
			set[plan.field] = "typology"
		case "model_product_channel", "product_channel":
			set[plan.field] = "typology"
		}
		result, err := db.Collection(plan.collection).UpdateMany(ctx, plan.filter, bson.M{"$set": set})
		if err != nil {
			log.Fatalf("migrate %s.%s: %v", plan.collection, plan.field, err)
		}
		fmt.Printf("migrated %d/%d row(s) in %s.%s\n", result.ModifiedCount, count, plan.collection, plan.field)
	}

	if !*apply {
		fmt.Println("dry-run complete (pass --apply to write)")
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
