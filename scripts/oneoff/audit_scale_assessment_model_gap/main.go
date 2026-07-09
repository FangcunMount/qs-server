package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	mongoURI := flag.String("mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	mongoDB := flag.String("mongo-db", envOr("MONGO_DB", "qs"), "MongoDB database name")
	flag.Parse()
	if *mongoURI == "" {
		log.Fatal("mongo-uri is required (or set MONGO_URI)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*mongoURI))
	if err != nil {
		log.Fatalf("connect mongo: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	db := client.Database(*mongoDB)
	report, err := audit(ctx, db)
	if err != nil {
		log.Fatalf("audit: %v", err)
	}
	printReport(report)
	if len(report.missingInAssessmentModels) > 0 {
		os.Exit(2)
	}
}

type auditReport struct {
	scalesTotal                int64
	scalesHead                 int64
	scalesPublishedSnapshot    int64
	scalesDeleted              int64
	assessmentModelsTotal      int64
	assessmentModelsScaleKind  int64
	publishedAssessmentTotal   int64
	publishedAssessmentScale   int64
	missingInAssessmentModels  []string
	extraInAssessmentModels    []string
}

func audit(ctx context.Context, db *mongo.Database) (*auditReport, error) {
	scales := db.Collection("scales")
	models := db.Collection("assessment_models")
	published := db.Collection("published_assessment_models")

	report := &auditReport{}

	var err error
	report.scalesTotal, err = scales.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	report.scalesDeleted, err = scales.CountDocuments(ctx, bson.M{"deleted_at": bson.M{"$ne": nil}})
	if err != nil {
		return nil, err
	}
	report.scalesHead, err = scales.CountDocuments(ctx, headScalesFilter())
	if err != nil {
		return nil, err
	}
	report.scalesPublishedSnapshot, err = scales.CountDocuments(ctx, bson.M{
		"deleted_at":   nil,
		"record_role":  "published_snapshot",
	})
	if err != nil {
		return nil, err
	}

	report.assessmentModelsTotal, err = models.CountDocuments(ctx, bson.M{"deleted_at": nil})
	if err != nil {
		return nil, err
	}
	report.assessmentModelsScaleKind, err = models.CountDocuments(ctx, bson.M{
		"deleted_at": nil,
		"kind":       "scale",
	})
	if err != nil {
		return nil, err
	}

	report.publishedAssessmentTotal, err = published.CountDocuments(ctx, bson.M{"deleted_at": bson.M{"$exists": false}})
	if err != nil {
		return nil, err
	}
	report.publishedAssessmentScale, err = published.CountDocuments(ctx, bson.M{
		"deleted_at": bson.M{"$exists": false},
		"kind":       "scale",
	})
	if err != nil {
		return nil, err
	}

	headCodes, err := distinctCodes(ctx, scales, headScalesFilter())
	if err != nil {
		return nil, err
	}
	modelCodes, err := distinctCodes(ctx, models, bson.M{"deleted_at": nil, "kind": "scale"})
	if err != nil {
		return nil, err
	}

	headSet := toSet(headCodes)
	modelSet := toSet(modelCodes)
	for code := range headSet {
		if _, ok := modelSet[code]; !ok {
			report.missingInAssessmentModels = append(report.missingInAssessmentModels, code)
		}
	}
	for code := range modelSet {
		if _, ok := headSet[code]; !ok {
			report.extraInAssessmentModels = append(report.extraInAssessmentModels, code)
		}
	}
	sort.Strings(report.missingInAssessmentModels)
	sort.Strings(report.extraInAssessmentModels)
	return report, nil
}

func headScalesFilter() bson.M {
	return bson.M{
		"deleted_at": nil,
		"$or": bson.A{
			bson.M{"record_role": "head"},
			bson.M{"record_role": bson.M{"$exists": false}},
			bson.M{"record_role": ""},
		},
	}
}

func distinctCodes(ctx context.Context, coll *mongo.Collection, filter bson.M) ([]string, error) {
	values, err := coll.Distinct(ctx, "code", filter)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(values))
	for _, value := range values {
		code, ok := value.(string)
		if !ok || code == "" {
			continue
		}
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes, nil
}

func toSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

func printReport(r *auditReport) {
	fmt.Println("=== scales vs assessment_models audit ===")
	fmt.Printf("scales total: %d (head=%d, published_snapshot=%d, deleted=%d)\n",
		r.scalesTotal, r.scalesHead, r.scalesPublishedSnapshot, r.scalesDeleted)
	fmt.Printf("assessment_models: total=%d kind=scale %d\n", r.assessmentModelsTotal, r.assessmentModelsScaleKind)
	fmt.Printf("published_assessment_models: total=%d kind=scale %d\n", r.publishedAssessmentTotal, r.publishedAssessmentScale)
	fmt.Printf("head codes missing in assessment_models: %d\n", len(r.missingInAssessmentModels))
	for _, code := range r.missingInAssessmentModels {
		fmt.Printf("  - %s\n", code)
	}
	if len(r.extraInAssessmentModels) > 0 {
		fmt.Printf("assessment_models scale codes without scales head: %d\n", len(r.extraInAssessmentModels))
		for _, code := range r.extraInAssessmentModels {
			fmt.Printf("  - %s\n", code)
		}
	}
	if len(r.missingInAssessmentModels) == 0 {
		fmt.Println("draft head migration looks complete (compare head count, not scales total)")
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
