// audit_published_projections is a read-only inventory checker for published
// snapshot projection hashes and deterministic payload replay (MC-R017 batch 3).
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
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type config struct {
	mongoURI string
	mongoDB  string
	codes    string
	json     bool
	timeout  time.Duration
}

type report struct {
	PublishedCount int                               `json:"published_count"`
	ErrorCount     int                               `json:"error_count"`
	Issues         []publication.InventoryAuditIssue `json:"issues"`
}

func main() {
	cfg := parseFlags()
	if cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "audit published projections failed: --mongo-uri is required (or set MONGO_URI)")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit published projections failed: connect mongo:", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "audit published projections failed: ping mongo:", err)
		os.Exit(1)
	}
	db := client.Database(cfg.mongoDB)
	normRepo := mongomodelcatalog.NewNormRepository(db)
	registry := appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{},
		appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepo},
		appdefinition.CognitiveDefinitionHandler{NormRepo: normRepo},
		appdefinition.TypologyDefinitionHandler{},
	)
	result, err := audit(ctx, db, registry, splitCodes(cfg.codes))
	if err != nil {
		fmt.Fprintln(os.Stderr, "audit published projections failed:", err)
		os.Exit(1)
	}
	if cfg.json {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	} else {
		printReport(result)
	}
	if result.ErrorCount > 0 {
		os.Exit(2)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.StringVar(&cfg.codes, "codes", "", "optional comma-separated model codes")
	flag.BoolVar(&cfg.json, "json", false, "write JSON report")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "MongoDB audit timeout")
	flag.Parse()
	return cfg
}

func audit(ctx context.Context, db *mongo.Database, registry appdefinition.Registry, codes []string) (*report, error) {
	snapshots, err := loadPublished(ctx, db, codes)
	if err != nil {
		return nil, err
	}
	result := &report{PublishedCount: len(snapshots)}
	for _, snapshot := range snapshots {
		binding := appdefinition.AlgorithmBindingFromModel(publication.ModelFromPublishedSnapshot(snapshot))
		handler, err := registry.MustResolveBinding(binding)
		if err != nil {
			result.Issues = append(result.Issues, publication.InventoryAuditIssue{
				Scope: "published", Code: snapshot.Code, Field: "binding", Rule: "handler.unsupported",
				Message: err.Error(),
			})
			continue
		}
		result.Issues = append(result.Issues, publication.AuditPublishedSnapshotInventory(ctx, snapshot, handler)...)
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		if result.Issues[i].Code != result.Issues[j].Code {
			return result.Issues[i].Code < result.Issues[j].Code
		}
		return result.Issues[i].Rule < result.Issues[j].Rule
	})
	result.ErrorCount = len(result.Issues)
	return result, nil
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

func printReport(result *report) {
	fmt.Printf("published snapshots: %d\n", result.PublishedCount)
	fmt.Printf("integrity issues: %d\n", result.ErrorCount)
	for _, item := range result.Issues {
		fmt.Printf("- [%s] %s %s (%s): %s\n", item.Scope, item.Code, item.Field, item.Rule, item.Message)
	}
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
