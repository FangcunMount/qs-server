// verify_definition_v2_cutover performs the read-only maintenance-window audit
// for the DefinitionV2-only ModelCatalog -> Evaluation -> Outcome cutover.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

const sampleLimit = 20

var identityRefPattern = regexp.MustCompile(`^isn:v2:[0-9a-f]{64}$`)

type config struct {
	mysqlDSN string
	mongoURI string
	mongoDB  string
	jsonOut  bool
	timeout  time.Duration
}

type finding struct {
	Source  string   `json:"source"`
	Rule    string   `json:"rule"`
	Count   int64    `json:"count"`
	Samples []string `json:"samples,omitempty"`
}

type report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Findings    []finding `json:"findings"`
	Total       int64     `json:"total"`
}

func main() {
	cfg := parseFlags()
	if cfg.mysqlDSN == "" && cfg.mongoURI == "" {
		fmt.Fprintln(os.Stderr, "verify cutover failed: provide --mysql-dsn and/or --mongo-uri")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	result := report{GeneratedAt: time.Now().UTC()}
	if cfg.mysqlDSN != "" {
		items, err := auditMySQL(ctx, cfg.mysqlDSN)
		if err != nil {
			fmt.Fprintln(os.Stderr, "verify cutover failed: mysql:", err)
			os.Exit(1)
		}
		result.Findings = append(result.Findings, items...)
	}
	if cfg.mongoURI != "" {
		items, err := auditMongo(ctx, cfg.mongoURI, cfg.mongoDB)
		if err != nil {
			fmt.Fprintln(os.Stderr, "verify cutover failed: mongo:", err)
			os.Exit(1)
		}
		result.Findings = append(result.Findings, items...)
	}
	sort.Slice(result.Findings, func(i, j int) bool {
		if result.Findings[i].Source != result.Findings[j].Source {
			return result.Findings[i].Source < result.Findings[j].Source
		}
		return result.Findings[i].Rule < result.Findings[j].Rule
	})
	for _, item := range result.Findings {
		result.Total += item.Count
	}
	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	} else {
		printReport(result)
	}
	if result.Total > 0 {
		os.Exit(2)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs_server"), "MongoDB database")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "audit timeout")
	flag.Parse()
	return cfg
}

func auditMySQL(ctx context.Context, dsn string) ([]finding, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	items := make([]finding, 0, 4)
	assessment, err := querySamples(ctx, db, "assessment", "model_identity.incomplete", `
SELECT CAST(id AS CHAR)
FROM assessment
WHERE deleted_at IS NULL
  AND evaluation_model_code IS NOT NULL
  AND (evaluation_model_kind IS NULL OR evaluation_model_kind = ''
    OR evaluation_model_algorithm IS NULL OR evaluation_model_algorithm = ''
    OR evaluation_model_version IS NULL OR evaluation_model_version = '')`)
	if err != nil {
		return nil, err
	}
	items = appendFinding(items, assessment)

	runs, err := queryInvalidRefSamples(ctx, db, "runtime_checkpoint", "input_snapshot_ref.not_v2", `
SELECT CONCAT(resource_id, ':', attempt_no), COALESCE(input_snapshot_ref, '')
FROM runtime_checkpoint
WHERE deleted_at IS NULL AND scope = 'evaluation_run'`)
	if err != nil {
		return nil, err
	}
	items = appendFinding(items, runs)

	outcomes, err := querySamples(ctx, db, "evaluation_outcome", "contract.incomplete", `
SELECT CAST(id AS CHAR)
FROM evaluation_outcome
WHERE model_kind = '' OR model_algorithm IS NULL OR model_algorithm = ''
   OR model_code = '' OR model_version = ''
   OR algorithm_family IS NULL OR algorithm_family = ''
   OR decision_kind IS NULL OR decision_kind = ''
   OR input_snapshot_ref IS NULL
   OR input_snapshot_ref NOT REGEXP '^isn:v2:[0-9a-f]{64}$'
   OR report_input_json IS NULL OR report_input_json = ''`)
	if err != nil {
		return nil, err
	}
	items = appendFinding(items, outcomes)
	return items, nil
}

func querySamples(ctx context.Context, db *sql.DB, source, rule, query string) (finding, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return finding{}, err
	}
	defer rows.Close()
	result := finding{Source: source, Rule: rule}
	for rows.Next() {
		var sample string
		if err := rows.Scan(&sample); err != nil {
			return finding{}, err
		}
		result.Count++
		if len(result.Samples) < sampleLimit {
			result.Samples = append(result.Samples, sample)
		}
	}
	return result, rows.Err()
}

func queryInvalidRefSamples(ctx context.Context, db *sql.DB, source, rule, query string) (finding, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return finding{}, err
	}
	defer rows.Close()
	result := finding{Source: source, Rule: rule}
	for rows.Next() {
		var id, ref string
		if err := rows.Scan(&id, &ref); err != nil {
			return finding{}, err
		}
		if identityRefPattern.MatchString(ref) {
			continue
		}
		result.Count++
		if len(result.Samples) < sampleLimit {
			result.Samples = append(result.Samples, id)
		}
	}
	return result, rows.Err()
}

func auditMongo(ctx context.Context, uri, database string) ([]finding, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.Background())
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(database)
	items := make([]finding, 0, 8)
	models := db.Collection("assessment_models")
	for _, check := range []struct {
		rule   string
		filter bson.M
	}{
		{"legacy_fields.present", bson.M{"deleted_at": nil, "$or": bson.A{
			bson.M{"payload": bson.M{"$exists": true}}, bson.M{"payload_format": bson.M{"$exists": true}},
			bson.M{"definition_payload": bson.M{"$exists": true}}, bson.M{"definition_payload_format": bson.M{"$exists": true}},
			bson.M{"is_active_published": bson.M{"$exists": true}},
		}}},
		{"definition_v2.missing", bson.M{"deleted_at": nil, "record_role": bson.M{"$in": bson.A{"head", "published_snapshot"}}, "$or": bson.A{
			bson.M{"definition_v2": bson.M{"$exists": false}}, bson.M{"definition_v2": nil},
		}}},
		{"published_identity.incomplete", bson.M{"deleted_at": nil, "record_role": "published_snapshot", "$or": bson.A{
			bson.M{"kind": ""}, bson.M{"algorithm": ""}, bson.M{"algorithm_family": ""}, bson.M{"decision_kind": ""},
			bson.M{"source.definition_content_hash": bson.M{"$exists": false}},
		}}},
	} {
		item, err := mongoSamples(ctx, models, "assessment_models", check.rule, check.filter)
		if err != nil {
			return nil, err
		}
		items = appendFinding(items, item)
	}

	rulesetCount, err := db.Collection("evaluation_rule_sets").CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	items = appendFinding(items, finding{Source: "evaluation_rule_sets", Rule: "collection.not_empty", Count: rulesetCount})

	normRepo := mongomodelcatalog.NewNormRepository(db)
	normItems, err := auditNorms(ctx, normRepo)
	if err != nil {
		return nil, err
	}
	items = append(items, normItems...)

	publishedRepo := mongomodelcatalog.NewRepository(db)
	registry := appdefinition.NewRegistry(
		appdefinition.ScaleDefinitionHandler{},
		appdefinition.TypologyDefinitionHandler{},
		appdefinition.BehavioralRatingDefinitionHandler{NormRepo: normRepo},
		appdefinition.CognitiveDefinitionHandler{NormRepo: normRepo},
	)
	runtimeItems, err := auditPublishedRuntime(ctx, publishedRepo, registry)
	if err != nil {
		return nil, err
	}
	items = append(items, runtimeItems...)
	return items, nil
}

func auditNorms(ctx context.Context, repo *mongomodelcatalog.NormRepository) ([]finding, error) {
	invalid := finding{Source: "assessment_norms", Rule: "validate_import.failed"}
	for page := 1; ; page++ {
		rows, total, err := repo.ListNorms(ctx, modelcatalogport.NormListFilter{Page: page, PageSize: 100})
		if err != nil {
			return nil, err
		}
		for _, table := range rows {
			if err := modelnorm.ValidateImport(table); err != nil {
				invalid.Count++
				if len(invalid.Samples) < sampleLimit {
					invalid.Samples = append(invalid.Samples, table.TableVersion)
				}
			}
		}
		if int64(page*100) >= total {
			break
		}
	}
	return appendFinding(nil, invalid), nil
}

func auditPublishedRuntime(ctx context.Context, repo *mongomodelcatalog.Repository, registry appdefinition.Registry) ([]finding, error) {
	byRule := map[string]*finding{}
	for page := 1; ; page++ {
		rows, total, err := repo.ListPublishedModels(ctx, modelcatalogport.ListPublishedFilter{Page: page, PageSize: 100})
		if err != nil {
			return nil, err
		}
		for _, snapshot := range rows {
			handler, ok := registry.Resolve(domain.NewIdentity(snapshot.Kind, snapshot.SubKind, snapshot.Algorithm))
			if !ok {
				addRule(byRule, "handler.missing", snapshot.Code)
				continue
			}
			for _, issue := range publication.AuditPublishedSnapshotInventory(ctx, snapshot, handler) {
				addRule(byRule, issue.Rule, snapshot.Code+"@"+snapshot.Version)
			}
		}
		if int64(page*100) >= total {
			break
		}
	}
	keys := make([]string, 0, len(byRule))
	for key := range byRule {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]finding, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byRule[key])
	}
	return out, nil
}

func addRule(items map[string]*finding, rule, sample string) {
	item := items[rule]
	if item == nil {
		item = &finding{Source: "published_runtime", Rule: rule}
		items[rule] = item
	}
	item.Count++
	if len(item.Samples) < sampleLimit {
		item.Samples = append(item.Samples, sample)
	}
}

func mongoSamples(ctx context.Context, collection *mongo.Collection, source, rule string, filter bson.M) (finding, error) {
	count, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return finding{}, err
	}
	result := finding{Source: source, Rule: rule, Count: count}
	if count == 0 {
		return result, nil
	}
	cursor, err := collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1, "code": 1, "release_version": 1}).SetLimit(sampleLimit))
	if err != nil {
		return finding{}, err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var row struct {
			ID             any    `bson:"_id"`
			Code           string `bson:"code"`
			ReleaseVersion string `bson:"release_version"`
		}
		if err := cursor.Decode(&row); err != nil {
			return finding{}, err
		}
		sample := fmt.Sprint(row.ID)
		if row.Code != "" {
			sample = row.Code
			if row.ReleaseVersion != "" {
				sample += "@" + row.ReleaseVersion
			}
		}
		result.Samples = append(result.Samples, sample)
	}
	return result, cursor.Err()
}

func appendFinding(items []finding, item finding) []finding {
	if item.Count == 0 {
		return items
	}
	return append(items, item)
}

func printReport(result report) {
	fmt.Printf("DefinitionV2 cutover audit: findings=%d generated_at=%s\n", result.Total, result.GeneratedAt.Format(time.RFC3339))
	if result.Total == 0 {
		fmt.Println("PASS: no incompatible data found")
		return
	}
	for _, item := range result.Findings {
		fmt.Printf("- %s %s: %d", item.Source, item.Rule, item.Count)
		if len(item.Samples) > 0 {
			fmt.Printf(" samples=%s", strings.Join(item.Samples, ","))
		}
		fmt.Println()
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
