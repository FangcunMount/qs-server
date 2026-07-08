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

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
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

	formats := publishing.LegacyDecodeOnlyPayloadFormats()
	total := int64(0)
	for _, format := range formats {
		count, err := repo.Collection().CountDocuments(ctx, bson.M{
			"payload_format": format,
			"deleted_at":     bson.M{"$exists": false},
		})
		if err != nil {
			log.Fatalf("count payload_format=%s: %v", format, err)
		}
		fmt.Printf("%s: %d\n", format, count)
		total += count
	}
	fmt.Printf("legacy payload_format total: %d\n", total)
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
