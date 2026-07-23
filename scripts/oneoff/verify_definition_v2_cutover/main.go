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
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	evaluationinputport "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	mongoindexes "github.com/FangcunMount/qs-server/internal/pkg/mongodb"
)

const sampleLimit = 20

var identityRefPattern = regexp.MustCompile(`^isn:v2:[0-9a-f]{64}$`)

type config struct {
	mysqlDSN               string
	mongoURI               string
	mongoDB                string
	modelCatalogIdentity   bool
	applyModelCatalogFixes bool
	jsonOut                bool
	timeout                time.Duration
}

type finding struct {
	Severity string   `json:"severity,omitempty"`
	Source   string   `json:"source"`
	Rule     string   `json:"rule"`
	Count    int64    `json:"count"`
	Samples  []string `json:"samples,omitempty"`
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
		var items []finding
		var err error
		if cfg.modelCatalogIdentity {
			items, err = auditModelCatalogIdentityMySQL(ctx, cfg.mysqlDSN)
		} else {
			items, err = auditMySQL(ctx, cfg.mysqlDSN)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "verify cutover failed: mysql:", err)
			os.Exit(1)
		}
		result.Findings = append(result.Findings, items...)
	}
	if cfg.mongoURI != "" {
		var items []finding
		var err error
		if cfg.modelCatalogIdentity {
			items, err = auditModelCatalogIdentityMongo(ctx, cfg.mongoURI, cfg.mongoDB)
		} else {
			items, err = auditMongo(ctx, cfg.mongoURI, cfg.mongoDB)
		}
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
		if item.Severity != "info" {
			result.Total += item.Count
		}
	}
	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	} else {
		printReport(result)
	}
	if result.Total > 0 {
		os.Exit(2)
	}
	if cfg.applyModelCatalogFixes {
		if !cfg.modelCatalogIdentity {
			fmt.Fprintln(os.Stderr, "verify cutover failed: --apply requires --modelcatalog-identity")
			os.Exit(1)
		}
		if cfg.mongoURI == "" || cfg.mongoDB == "" {
			fmt.Fprintln(os.Stderr, "verify cutover failed: --apply requires --mongo-uri and --mongo-db")
			os.Exit(1)
		}
		if err := applyModelCatalogIdentity(ctx, cfg.mongoURI, cfg.mongoDB); err != nil {
			fmt.Fprintln(os.Stderr, "verify cutover failed: apply modelcatalog identity:", err)
			os.Exit(1)
		}
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.mysqlDSN, "mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN")
	flag.StringVar(&cfg.mongoURI, "mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI")
	flag.StringVar(&cfg.mongoDB, "mongo-db", envOr("MONGO_DB", "qs_server"), "MongoDB database")
	flag.BoolVar(&cfg.modelCatalogIdentity, "modelcatalog-identity", false, "audit ModelCatalog canonical identity and legacy runtime recoverability")
	flag.BoolVar(&cfg.applyModelCatalogFixes, "apply", false, "apply the audited ModelCatalog Mongo cleanup; requires --modelcatalog-identity")
	flag.BoolVar(&cfg.jsonOut, "json", false, "emit JSON")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "audit timeout")
	flag.Parse()
	return cfg
}

// applyModelCatalogIdentity is deliberately opt-in. It creates the canonical
// index before removing redundant fields and leaves immutable reports and SQL
// history untouched. The caller must run the dry-run audit to zero findings.
func applyModelCatalogIdentity(ctx context.Context, uri, database string) error {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	models := client.Database(database).Collection("assessment_models")
	_, err = models.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "kind", Value: 1}, {Key: "algorithm", Value: 1}, {Key: "code", Value: 1}, {Key: "release_version", Value: 1}, {Key: "record_role", Value: 1}},
		Options: options.Index().SetName("idx_assessment_models_snapshot_identity_version_v2").SetUnique(true).
			SetPartialFilterExpression(bson.M{"record_role": "published_snapshot", "deleted_at": nil}),
	})
	if err != nil {
		return fmt.Errorf("create canonical snapshot identity index: %w", err)
	}
	_, err = models.UpdateMany(ctx,
		bson.M{"deleted_at": nil, "record_role": bson.M{"$in": bson.A{"head", "published_snapshot"}}},
		bson.M{"$unset": bson.M{"sub_kind": "", "product_channel": "", "algorithm_family": ""}},
	)
	if err != nil {
		return fmt.Errorf("unset redundant assessment model identity fields: %w", err)
	}
	remaining, err := models.CountDocuments(ctx, bson.M{
		"deleted_at": nil, "record_role": "published_snapshot",
		"$or": bson.A{
			bson.M{"kind": ""}, bson.M{"algorithm": ""}, bson.M{"code": ""}, bson.M{"release_version": ""},
		},
	})
	if err != nil {
		return fmt.Errorf("verify canonical active snapshots: %w", err)
	}
	if remaining != 0 {
		return fmt.Errorf("verify canonical active snapshots: %d incomplete records remain", remaining)
	}
	if _, err := models.Indexes().DropOne(ctx, "idx_assessment_models_snapshot_identity_version"); err != nil && !strings.Contains(err.Error(), "index not found") {
		return fmt.Errorf("drop legacy snapshot identity index: %w", err)
	}
	return nil
}

func auditMySQL(ctx context.Context, dsn string) ([]finding, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	items := make([]finding, 0, 7)
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
   OR decision_kind IS NULL OR decision_kind = ''
   OR input_snapshot_ref IS NULL
   OR input_snapshot_ref NOT REGEXP '^isn:v2:[0-9a-f]{64}$'
   OR report_input_json IS NULL OR report_input_json = ''`)
	if err != nil {
		return nil, err
	}
	items = appendFinding(items, outcomes)

	outcomeContracts, err := auditOutcomeContracts(ctx, db)
	if err != nil {
		return nil, err
	}
	items = append(items, outcomeContracts...)
	return items, nil
}

// auditModelCatalogIdentityMySQL is intentionally separate from the
// DefinitionV2 contract audit. It only judges whether outcome runtime identity
// can be restored without trusting the retired algorithm_family column.
func auditModelCatalogIdentityMySQL(ctx context.Context, dsn string) ([]finding, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
SELECT CAST(id AS CHAR), COALESCE(model_kind, ''), COALESCE(model_algorithm, ''), COALESCE(decision_kind, '')
FROM evaluation_outcome`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byRule := map[string]*finding{}
	for rows.Next() {
		var id, kindText, algorithmText, decisionText string
		if err := rows.Scan(&id, &kindText, &algorithmText, &decisionText); err != nil {
			return nil, err
		}
		if _, err := domain.ResolveLegacyRuntime(domain.Kind(kindText), domain.Algorithm(algorithmText), domain.DecisionKind(decisionText)); err != nil {
			addFindingRule(byRule, "evaluation_outcome", "runtime_identity.unrecoverable", id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortedFindings(byRule), nil
}

// auditModelCatalogIdentityMongo deliberately does not call DefinitionV2
// validators. It examines both catalog record roles and verifies that a frozen
// runtime route is either valid or uniquely recoverable from Kind+Algorithm.
func auditModelCatalogIdentityMongo(ctx context.Context, uri, database string) ([]finding, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(database)
	items, err := auditModelCatalogIdentityModels(ctx, db.Collection("assessment_models"))
	if err != nil {
		return nil, err
	}
	indexItems, err := auditModelCatalogIdentityIndexes(ctx, db.Collection("assessment_models"))
	if err != nil {
		return nil, err
	}
	return append(items, indexItems...), nil
}

func auditModelCatalogIdentityModels(ctx context.Context, models *mongo.Collection) ([]finding, error) {
	cursor, err := models.Find(ctx, bson.M{"deleted_at": nil, "record_role": bson.M{"$in": bson.A{"head", "published_snapshot"}}}, options.Find().SetProjection(bson.M{
		"_id": 1, "code": 1, "record_role": 1, "kind": 1, "algorithm": 1, "decision_kind": 1,
		"sub_kind": 1, "product_channel": 1, "algorithm_family": 1,
	}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	byRule := map[string]*finding{}
	for cursor.Next(ctx) {
		var row struct {
			ID              any    `bson:"_id"`
			Code            string `bson:"code"`
			RecordRole      string `bson:"record_role"`
			Kind            string `bson:"kind"`
			Algorithm       string `bson:"algorithm"`
			DecisionKind    string `bson:"decision_kind"`
			SubKind         string `bson:"sub_kind"`
			ProductChannel  string `bson:"product_channel"`
			AlgorithmFamily string `bson:"algorithm_family"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		sample := row.Code
		if sample == "" {
			sample = fmt.Sprint(row.ID)
		}
		if row.Kind == "" || row.Algorithm == "" {
			addFindingRule(byRule, "assessment_models", "canonical_identity.incomplete", sample)
			continue
		}
		kind := domain.Kind(row.Kind)
		if row.SubKind != "" && row.SubKind != string(domain.CanonicalSubKindFor(kind)) {
			addFindingRule(byRule, "assessment_models", "legacy_sub_kind.inconsistent", sample)
		}
		if row.ProductChannel != "" && row.ProductChannel != string(domain.DefaultProductChannelFor(kind)) {
			addFindingRule(byRule, "assessment_models", "legacy_product_channel.inconsistent", sample)
		}
		runtime, runtimeErr := domain.ResolveLegacyRuntime(kind, domain.Algorithm(row.Algorithm), domain.DecisionKind(row.DecisionKind))
		if runtimeErr != nil {
			addFindingRule(byRule, "assessment_models", "runtime_identity.unrecoverable", sample)
			continue
		}
		if row.AlgorithmFamily != "" && row.AlgorithmFamily != string(runtime.AlgorithmFamily) {
			addFindingRule(byRule, "assessment_models", "legacy_algorithm_family.inconsistent", sample)
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return sortedFindings(byRule), nil
}

func auditModelCatalogIdentityIndexes(ctx context.Context, models *mongo.Collection) ([]finding, error) {
	names, err := mongoIndexNames(ctx, models)
	if err != nil {
		return nil, err
	}
	if _, ok := names["idx_assessment_models_snapshot_identity_version_v2"]; !ok {
		return []finding{{Severity: "info", Source: "mongo_indexes", Rule: "canonical_snapshot_identity.missing", Count: 1}}, nil
	}
	return nil, nil
}

type outcomeContractRow struct {
	ID              string
	SchemaVersion   uint
	ModelKind       string
	ModelSubKind    string
	ModelAlgorithm  string
	ModelCode       string
	ModelVersion    string
	DecisionKind    string
	ReportInputJSON string
	PayloadJSON     string
}

func auditOutcomeContracts(ctx context.Context, db *sql.DB) ([]finding, error) {
	rows, err := db.QueryContext(ctx, `
SELECT CAST(id AS CHAR), schema_version, model_kind, COALESCE(model_sub_kind, ''),
       COALESCE(model_algorithm, ''), model_code, model_version,
       COALESCE(decision_kind, ''), COALESCE(report_input_json, ''), payload_json
FROM evaluation_outcome`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	byRule := map[string]*finding{}
	for rows.Next() {
		var row outcomeContractRow
		if err := rows.Scan(
			&row.ID, &row.SchemaVersion, &row.ModelKind, &row.ModelSubKind,
			&row.ModelAlgorithm, &row.ModelCode, &row.ModelVersion,
			&row.DecisionKind, &row.ReportInputJSON, &row.PayloadJSON,
		); err != nil {
			return nil, err
		}
		for _, rule := range auditOutcomeContractRow(row) {
			addFindingRule(byRule, "evaluation_outcome", rule, row.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortedFindings(byRule), nil
}

func auditOutcomeContractRow(row outcomeContractRow) []string {
	rules := make([]string, 0, 3)
	if row.SchemaVersion != domainoutcome.CurrentSchemaVersion {
		rules = append(rules, "schema_version.not_2")
	}
	modelRef := evaluationinputport.ModelRef{
		Kind:      evaluationinputport.EvaluationModelKind(row.ModelKind),
		Algorithm: row.ModelAlgorithm, Code: row.ModelCode, Version: row.ModelVersion,
	}
	if issues := evaluationinputport.AuditReportInput([]byte(row.ReportInputJSON), modelRef); len(issues) > 0 {
		rules = append(rules, "report_input.invalid")
	}
	if domain.DecisionKind(row.DecisionKind) == domain.DecisionKindNormLookup {
		var execution domainoutcome.Execution
		if json.Unmarshal([]byte(row.PayloadJSON), &execution) != nil || !hasNormReference(execution.Dimensions) {
			rules = append(rules, "norm_reference.missing")
		}
	}
	return rules
}

func hasNormReference(dimensions []domainoutcome.DimensionResult) bool {
	for _, dimension := range dimensions {
		if dimension.NormReference != nil && dimension.NormReference.TableVersion != "" {
			return true
		}
	}
	return false
}

func querySamples(ctx context.Context, db *sql.DB, source, rule, query string) (finding, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return finding{}, err
	}
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(database)
	items := make([]finding, 0, 12)
	migrationItems, err := auditMongoMigrationState(ctx, db)
	if err != nil {
		return nil, err
	}
	items = append(items, migrationItems...)
	indexItems, err := auditModelCatalogIndexes(ctx, db)
	if err != nil {
		return nil, err
	}
	items = append(items, indexItems...)
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
			bson.M{"kind": ""}, bson.M{"algorithm": ""}, bson.M{"decision_kind": ""},
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

func auditMongoMigrationState(ctx context.Context, db *mongo.Database) ([]finding, error) {
	var state struct {
		Version int64 `bson:"version"`
		Dirty   bool  `bson:"dirty"`
	}
	err := db.Collection("schema_migrations").FindOne(ctx, bson.M{}).Decode(&state)
	if err == mongo.ErrNoDocuments {
		return []finding{{Source: "schema_migrations", Rule: "state.missing", Count: 1}}, nil
	}
	if err != nil {
		return nil, err
	}
	if !state.Dirty {
		return nil, nil
	}
	return []finding{{
		Source: "schema_migrations", Rule: "state.dirty", Count: 1,
		Samples: []string{fmt.Sprintf("version=%d", state.Version)},
	}}, nil
}

func auditModelCatalogIndexes(ctx context.Context, db *mongo.Database) ([]finding, error) {
	byRule := map[string]*finding{}
	legacyByCollection := mongoindexes.ForbiddenLegacyIndexNames()
	for collection, required := range mongoindexes.RequiredUnifiedIndexNames() {
		names, err := mongoIndexNames(ctx, db.Collection(collection))
		if err != nil {
			return nil, err
		}
		for _, rule := range auditIndexNames(collection, names, required, legacyByCollection[collection]) {
			addFindingRule(byRule, "mongo_indexes", rule.rule, rule.sample)
		}
	}
	return sortedFindings(byRule), nil
}

type indexAuditRule struct {
	rule   string
	sample string
}

func auditIndexNames(collection string, names map[string]struct{}, required, legacy []string) []indexAuditRule {
	issues := make([]indexAuditRule, 0)
	for _, name := range required {
		if _, ok := names[name]; !ok {
			issues = append(issues, indexAuditRule{rule: "required.missing", sample: collection + "/" + name})
		}
	}
	for _, name := range legacy {
		if _, ok := names[name]; ok {
			issues = append(issues, indexAuditRule{rule: "legacy.present", sample: collection + "/" + name})
		}
	}
	return issues
}

func mongoIndexNames(ctx context.Context, collection *mongo.Collection) (map[string]struct{}, error) {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	names := make(map[string]struct{})
	for cursor.Next(ctx) {
		var row struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		if row.Name != "" {
			names[row.Name] = struct{}{}
		}
	}
	return names, cursor.Err()
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
	addFindingRule(items, "published_runtime", rule, sample)
}

func addFindingRule(items map[string]*finding, source, rule, sample string) {
	key := source + "\x00" + rule
	item := items[key]
	if item == nil {
		item = &finding{Source: source, Rule: rule}
		items[key] = item
	}
	item.Count++
	if len(item.Samples) < sampleLimit {
		item.Samples = append(item.Samples, sample)
	}
}

func sortedFindings(byRule map[string]*finding) []finding {
	keys := make([]string, 0, len(byRule))
	for key := range byRule {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]finding, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byRule[key])
	}
	return out
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
	defer func() { _ = cursor.Close(ctx) }()
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
		prefix := "-"
		if item.Severity == "info" {
			prefix = "- INFO"
		}
		fmt.Printf("%s %s %s: %d", prefix, item.Source, item.Rule, item.Count)
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
