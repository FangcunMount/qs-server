package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

const (
	governanceSchemaVersion = "evaluation-outcome-template-route-governance/v2"
	targetLegacyVersion     = "legacy-v1"
	targetCurrentVersion    = "2026-08-v1"
	applyConfirmation       = "materialize-outcome-template-route-legacy-v1"
	rollbackConfirmation    = "rollback-outcome-template-route-legacy-v1"
	transactionBatchSize    = 200
	artifactCollection      = "interpret_report_artifacts"
)

var knownTemplateIDs = map[string]struct{}{
	"standard": {}, "mbti": {}, "sbti": {}, "bigfive": {}, "enneagram": {},
}

var errReportSectionsRequired = errors.New("InterpretationAssets.ReportSpec.Sections is required")

type config struct {
	Operation       string
	ManifestPath    string
	AfterID         uint64
	MaxRecords      int
	RequireComplete bool
	Confirm         string
	MySQL           mysqldriver.Config
	Mongo           mongoConfig
}

type mongoConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
}

type manifest struct {
	SchemaVersion      string   `json:"schema_version"`
	Database           string   `json:"database"`
	Table              string   `json:"table"`
	GeneratedAt        string   `json:"generated_at"`
	TargetLegacy       string   `json:"target_legacy_version"`
	MongoDatabase      string   `json:"mongo_database"`
	ArtifactCollection string   `json:"artifact_collection"`
	Records            []record `json:"records"`
	RecordsFingerprint string   `json:"records_fingerprint"`
}

type record struct {
	OutcomeID          uint64                     `json:"outcome_id"`
	ModelKind          string                     `json:"model_kind"`
	ModelCode          string                     `json:"model_code"`
	ModelVersion       string                     `json:"model_version"`
	EvaluatedAt        string                     `json:"evaluated_at"`
	TemplateID         string                     `json:"template_id"`
	TemplateVersion    string                     `json:"template_version"`
	SourceRawHash      string                     `json:"source_raw_hash"`
	SourceSemanticHash string                     `json:"source_semantic_hash"`
	TargetSemanticHash string                     `json:"target_semantic_hash"`
	Sections           []sectionDelta             `json:"sections"`
	TypologyRouting    *routeDelta                `json:"typology_routing,omitempty"`
	Materialization    *reportSpecMaterialization `json:"report_spec_materialization,omitempty"`
}

type reportSpecMaterialization struct {
	OriginalSectionsState string           `json:"original_sections_state"`
	Section               frozenSection    `json:"section"`
	Artifact              artifactEvidence `json:"artifact"`
}

type frozenSection struct {
	Code            string   `json:"Code"`
	Title           string   `json:"Title,omitempty"`
	SourceRefs      []string `json:"SourceRefs"`
	Kind            string   `json:"Kind"`
	AdapterKey      string   `json:"AdapterKey,omitempty"`
	TemplateID      string   `json:"TemplateID"`
	TemplateVersion string   `json:"TemplateVersion"`
	CategoryLabel   string   `json:"CategoryLabel,omitempty"`
}

type artifactEvidence struct {
	ArtifactID           uint64 `json:"artifact_id"`
	OutcomeID            uint64 `json:"outcome_id"`
	TemplateVersion      string `json:"template_version"`
	BuilderIdentity      string `json:"builder_identity"`
	ContentSchemaVersion string `json:"content_schema_version"`
	ModelKind            string `json:"model_kind"`
	ModelCode            string `json:"model_code"`
	ModelVersion         string `json:"model_version"`
	PresentationSource   string `json:"presentation_source"`
	Fingerprint          string `json:"fingerprint"`
}

type fieldState struct {
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

type sectionDelta struct {
	Index                   int        `json:"index"`
	OriginalTemplateID      fieldState `json:"original_template_id"`
	OriginalTemplateVersion fieldState `json:"original_template_version"`
}

type routeDelta struct {
	OriginalTemplateID      fieldState `json:"original_template_id"`
	OriginalTemplateVersion fieldState `json:"original_template_version"`
}

type reportPlan struct {
	Changed            bool
	TemplateID         string
	TemplateVersion    string
	SourceSemanticHash string
	TargetSemanticHash string
	TargetJSON         string
	Sections           []sectionDelta
	TypologyRouting    *routeDelta
	Materialization    *reportSpecMaterialization
}

type inventory struct {
	Total                int64 `json:"total"`
	Explicit             int64 `json:"explicit"`
	Candidates           int64 `json:"candidates"`
	MaterializedSections int64 `json:"materialized_sections"`
}

func main() {
	if err := run(context.Background(), os.Getenv, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "govern evaluation outcome template routes:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	cfg, err := readConfig(getenv)
	if err != nil {
		return err
	}
	db, err := openDatabase(ctx, cfg.MySQL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	mongoClient, mongoDatabase, err := openMongoDatabase(ctx, cfg.Mongo)
	if err != nil {
		return err
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()

	switch cfg.Operation {
	case "audit":
		return audit(ctx, db, mongoDatabase, cfg, stdout)
	case "apply", "rollback", "verify":
		governance, err := readManifest(cfg.ManifestPath)
		if err != nil {
			return err
		}
		if err := validateManifest(governance, cfg.MySQL.DBName, cfg.Mongo.Database); err != nil {
			return err
		}
		records := selectedRecords(governance.Records, cfg)
		var artifacts map[uint64]artifactCandidate
		if cfg.Operation != "rollback" {
			if hasReportSpecMaterialization(governance.Records) {
				artifacts, _, err = loadArtifactIndex(ctx, mongoDatabase)
				if err != nil {
					return err
				}
				if err := validateArtifactEvidence(records, artifacts); err != nil {
					return err
				}
			}
		}
		if cfg.Operation == "verify" {
			return verify(ctx, db, records, artifacts, cfg.RequireComplete, stdout)
		}
		return mutate(ctx, db, records, cfg.Operation, stdout)
	default:
		return fmt.Errorf("unsupported operation %q", cfg.Operation)
	}
}

func readConfig(getenv func(string) string) (config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := config{
		Operation:    strings.TrimSpace(getenv("OUTCOME_TEMPLATE_ROUTE_OPERATION")),
		ManifestPath: strings.TrimSpace(getenv("OUTCOME_TEMPLATE_ROUTE_MANIFEST_PATH")),
		Confirm:      strings.TrimSpace(getenv("OUTCOME_TEMPLATE_ROUTE_CONFIRM")),
	}
	if cfg.Operation == "" {
		cfg.Operation = "audit"
	}
	if cfg.ManifestPath == "" {
		return config{}, errors.New("OUTCOME_TEMPLATE_ROUTE_MANIFEST_PATH is required")
	}
	if !oneOf(cfg.Operation, "audit", "apply", "verify", "rollback") {
		return config{}, errors.New("OUTCOME_TEMPLATE_ROUTE_OPERATION must be audit, apply, verify or rollback")
	}
	var err error
	if value := strings.TrimSpace(getenv("OUTCOME_TEMPLATE_ROUTE_AFTER_ID")); value != "" && value != "0" {
		cfg.AfterID, err = strconv.ParseUint(value, 10, 64)
		if err != nil {
			return config{}, errors.New("OUTCOME_TEMPLATE_ROUTE_AFTER_ID must be an unsigned integer")
		}
	}
	if value := strings.TrimSpace(getenv("OUTCOME_TEMPLATE_ROUTE_MAX_RECORDS")); value != "" {
		cfg.MaxRecords, err = strconv.Atoi(value)
		if err != nil || cfg.MaxRecords < 0 {
			return config{}, errors.New("OUTCOME_TEMPLATE_ROUTE_MAX_RECORDS must be a non-negative integer")
		}
	}
	cfg.RequireComplete = strings.EqualFold(strings.TrimSpace(getenv("OUTCOME_TEMPLATE_ROUTE_REQUIRE_COMPLETE")), "true")
	if cfg.Operation == "apply" && cfg.Confirm != applyConfirmation {
		return config{}, fmt.Errorf("apply requires OUTCOME_TEMPLATE_ROUTE_CONFIRM=%s", applyConfirmation)
	}
	if cfg.Operation == "rollback" && cfg.Confirm != rollbackConfirmation {
		return config{}, fmt.Errorf("rollback requires OUTCOME_TEMPLATE_ROUTE_CONFIRM=%s", rollbackConfirmation)
	}
	host, port := strings.TrimSpace(getenv("MYSQL_HOST")), strings.TrimSpace(getenv("MYSQL_PORT"))
	if port == "" {
		port = "3306"
	}
	cfg.MySQL = mysqldriver.Config{
		User: strings.TrimSpace(getenv("MYSQL_USERNAME")), Passwd: getenv("MYSQL_PASSWORD"),
		Net: "tcp", Addr: net.JoinHostPort(host, port), DBName: strings.TrimSpace(getenv("MYSQL_DATABASE")),
		ParseTime: true, Loc: time.UTC, Timeout: 30 * time.Second, ReadTimeout: 15 * time.Minute, WriteTimeout: 15 * time.Minute,
		AllowNativePasswords: true,
	}
	if host == "" || cfg.MySQL.User == "" || cfg.MySQL.Passwd == "" || cfg.MySQL.DBName == "" {
		return config{}, errors.New("MYSQL_HOST, MYSQL_USERNAME, MYSQL_PASSWORD and MYSQL_DATABASE are required")
	}
	cfg.Mongo = mongoConfig{
		Host: strings.TrimSpace(getenv("MONGODB_HOST")), Port: strings.TrimSpace(getenv("MONGODB_PORT")),
		Username: strings.TrimSpace(getenv("MONGODB_USERNAME")), Password: getenv("MONGODB_PASSWORD"),
		Database: strings.TrimSpace(getenv("MONGODB_DBNAME")),
	}
	if cfg.Mongo.Port == "" {
		cfg.Mongo.Port = "27017"
	}
	if cfg.Mongo.Host == "" || cfg.Mongo.Username == "" || cfg.Mongo.Password == "" || cfg.Mongo.Database == "" {
		return config{}, errors.New("MONGODB_HOST, MONGODB_USERNAME, MONGODB_PASSWORD and MONGODB_DBNAME are required")
	}
	return cfg, nil
}

func openDatabase(ctx context.Context, cfg mysqldriver.Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open MySQL: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping MySQL: %w", err)
	}
	return db, nil
}

func openMongoDatabase(ctx context.Context, cfg mongoConfig) (*mongo.Client, *mongo.Database, error) {
	uri := "mongodb://" + net.JoinHostPort(cfg.Host, cfg.Port)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetAuth(options.Credential{
		AuthSource: "admin", Username: cfg.Username, Password: cfg.Password,
	}).SetConnectTimeout(30*time.Second).SetServerSelectionTimeout(30*time.Second))
	if err != nil {
		return nil, nil, fmt.Errorf("connect MongoDB: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("ping MongoDB: %w", err)
	}
	return client, client.Database(cfg.Database), nil
}

func audit(ctx context.Context, db *sql.DB, mongoDatabase *mongo.Database, cfg config, stdout io.Writer) error {
	generatedAt := time.Now().UTC()
	artifacts, artifactInventory, err := loadArtifactIndex(ctx, mongoDatabase)
	if err != nil {
		return err
	}
	records := make([]record, 0)
	state, err := scanOutcomesWithPlanner(ctx, db, func(row outcomeRow) (reportPlan, error) {
		plan, planErr := planReportInput(row.ReportInput, row.ModelKind)
		if planErr == nil {
			return plan, nil
		}
		if !errors.Is(planErr, errReportSectionsRequired) {
			return reportPlan{}, planErr
		}
		materialization, materializeErr := materializationFromArtifact(row, artifacts[row.ID])
		if materializeErr != nil {
			return reportPlan{}, materializeErr
		}
		return planReportInputWithMaterialization(row.ReportInput, row.ModelKind, materialization)
	}, func(row outcomeRow, plan reportPlan) error {
		if !plan.Changed {
			return nil
		}
		records = append(records, record{
			OutcomeID: row.ID, ModelKind: row.ModelKind, ModelCode: row.ModelCode, ModelVersion: row.ModelVersion,
			EvaluatedAt: row.EvaluatedAt.UTC().Format(time.RFC3339Nano), TemplateID: plan.TemplateID, TemplateVersion: plan.TemplateVersion,
			SourceRawHash: hashString(row.ReportInput), SourceSemanticHash: plan.SourceSemanticHash,
			TargetSemanticHash: plan.TargetSemanticHash, Sections: plan.Sections, TypologyRouting: plan.TypologyRouting,
			Materialization: plan.Materialization,
		})
		return nil
	})
	if err != nil {
		return err
	}
	fingerprint, err := recordsFingerprint(records)
	if err != nil {
		return err
	}
	governance := manifest{
		SchemaVersion: governanceSchemaVersion, Database: cfg.MySQL.DBName, Table: "evaluation_outcome",
		GeneratedAt: generatedAt.Format(time.RFC3339Nano), TargetLegacy: targetLegacyVersion,
		MongoDatabase: cfg.Mongo.Database, ArtifactCollection: artifactCollection,
		Records: records, RecordsFingerprint: fingerprint,
	}
	if err := writeManifest(cfg.ManifestPath, governance); err != nil {
		return err
	}
	return writeResult(stdout, map[string]any{
		"operation": "audit", "inventory": state, "records": len(records),
		"artifact_inventory": artifactInventory,
		"manifest_path":      cfg.ManifestPath, "records_fingerprint": fingerprint,
	})
}

type outcomeRow struct {
	ID           uint64
	ModelKind    string
	ModelCode    string
	ModelVersion string
	EvaluatedAt  time.Time
	ReportInput  string
}

type artifactDocument struct {
	DomainID             int64                 `bson:"domain_id"`
	OutcomeID            int64                 `bson:"outcome_id"`
	TemplateVersion      string                `bson:"template_version"`
	BuilderIdentity      string                `bson:"builder_identity"`
	ContentSchemaVersion string                `bson:"content_schema_version"`
	Model                *artifactModel        `bson:"model"`
	PresentationProfile  *artifactPresentation `bson:"presentation_profile"`
}

type artifactModel struct {
	Kind    string `bson:"kind"`
	Code    string `bson:"code"`
	Version string `bson:"version"`
}

type artifactPresentation struct {
	VisibleFactorCodes []string `bson:"visible_factor_codes"`
	Source             string   `bson:"source"`
}

type artifactCandidate struct {
	Document artifactDocument
	Count    int
}

type artifactInventory struct {
	Scanned           int64 `json:"scanned"`
	Indexed           int64 `json:"indexed"`
	DuplicateOutcomes int64 `json:"duplicate_outcomes"`
}

func loadArtifactIndex(ctx context.Context, database *mongo.Database) (map[uint64]artifactCandidate, artifactInventory, error) {
	if database == nil {
		return nil, artifactInventory{}, errors.New("MongoDB artifact database is required")
	}
	cursor, err := database.Collection(artifactCollection).Find(ctx, bson.M{
		"deleted_at": nil, "template_version": targetLegacyVersion,
	}, options.Find().SetProjection(bson.M{
		"_id": 0, "domain_id": 1, "outcome_id": 1, "template_version": 1,
		"builder_identity": 1, "content_schema_version": 1, "model": 1, "presentation_profile": 1,
	}).SetBatchSize(2000))
	if err != nil {
		return nil, artifactInventory{}, fmt.Errorf("query legacy Interpretation artifacts: %w", err)
	}
	defer func() { _ = cursor.Close(context.Background()) }()
	index := make(map[uint64]artifactCandidate)
	state := artifactInventory{}
	for cursor.Next(ctx) {
		var document artifactDocument
		if err := cursor.Decode(&document); err != nil {
			return nil, state, fmt.Errorf("decode legacy Interpretation artifact: %w", err)
		}
		state.Scanned++
		if document.DomainID <= 0 || document.OutcomeID <= 0 {
			return nil, state, errors.New("legacy Interpretation artifact identity is invalid")
		}
		outcomeID := uint64(document.OutcomeID)
		candidate, exists := index[outcomeID]
		if !exists {
			index[outcomeID] = artifactCandidate{Document: document, Count: 1}
			state.Indexed++
			continue
		}
		if candidate.Count == 1 {
			state.DuplicateOutcomes++
		}
		candidate.Count++
		index[outcomeID] = candidate
	}
	if err := cursor.Err(); err != nil {
		return nil, state, fmt.Errorf("scan legacy Interpretation artifacts: %w", err)
	}
	return index, state, nil
}

func materializationFromArtifact(row outcomeRow, candidate artifactCandidate) (*reportSpecMaterialization, error) {
	evidence, codes, err := artifactEvidenceFor(row, candidate)
	if err != nil {
		return nil, err
	}
	root, _, err := decodeCanonicalJSON(row.ReportInput)
	if err != nil {
		return nil, err
	}
	if err := validateArtifactCodes(root, codes); err != nil {
		return nil, err
	}
	assets, err := objectField(root, "InterpretationAssets")
	if err != nil {
		return nil, err
	}
	reportSpec, err := objectField(assets, "ReportSpec")
	if err != nil {
		return nil, err
	}
	state, err := emptySectionsState(reportSpec)
	if err != nil {
		return nil, err
	}
	return &reportSpecMaterialization{
		OriginalSectionsState: state,
		Section: frozenSection{
			Code: "factor_scores", SourceRefs: append([]string(nil), codes...), Kind: "factor_scores",
			TemplateID: "standard", TemplateVersion: targetLegacyVersion,
		},
		Artifact: evidence,
	}, nil
}

func artifactEvidenceFor(row outcomeRow, candidate artifactCandidate) (artifactEvidence, []string, error) {
	if candidate.Count != 1 {
		return artifactEvidence{}, nil, fmt.Errorf("outcome %d must have exactly one active legacy artifact, found %d", row.ID, candidate.Count)
	}
	document := candidate.Document
	if document.OutcomeID <= 0 || uint64(document.OutcomeID) != row.ID || document.DomainID <= 0 {
		return artifactEvidence{}, nil, fmt.Errorf("outcome %d artifact association is invalid", row.ID)
	}
	if document.TemplateVersion != targetLegacyVersion || document.ContentSchemaVersion != "report-content/v1" {
		return artifactEvidence{}, nil, fmt.Errorf("outcome %d artifact release identity is invalid", row.ID)
	}
	wantBuilder, ok := map[string]string{"scale": "factor-scoring", "behavioral_rating": "norm-profile", "cognitive": "task-performance"}[row.ModelKind]
	if !ok || document.BuilderIdentity != wantBuilder {
		return artifactEvidence{}, nil, fmt.Errorf("outcome %d artifact builder %q is incompatible with model kind %q", row.ID, document.BuilderIdentity, row.ModelKind)
	}
	if document.Model == nil || document.Model.Kind != row.ModelKind || document.Model.Code != row.ModelCode || document.Model.Version != row.ModelVersion {
		return artifactEvidence{}, nil, fmt.Errorf("outcome %d artifact model identity does not match frozen outcome", row.ID)
	}
	if document.PresentationProfile == nil || !oneOf(document.PresentationProfile.Source, "frozen", "legacy_artifact_dimensions/v1") {
		return artifactEvidence{}, nil, fmt.Errorf("outcome %d artifact presentation profile is not frozen", row.ID)
	}
	codes := append([]string(nil), document.PresentationProfile.VisibleFactorCodes...)
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if code == "" || code != strings.TrimSpace(code) {
			return artifactEvidence{}, nil, fmt.Errorf("outcome %d artifact presentation contains an invalid factor code", row.ID)
		}
		if _, duplicate := seen[code]; duplicate {
			return artifactEvidence{}, nil, fmt.Errorf("outcome %d artifact presentation contains duplicate factor code %s", row.ID, code)
		}
		seen[code] = struct{}{}
	}
	evidence := artifactEvidence{
		ArtifactID: uint64(document.DomainID), OutcomeID: row.ID, TemplateVersion: document.TemplateVersion,
		BuilderIdentity: document.BuilderIdentity, ContentSchemaVersion: document.ContentSchemaVersion,
		ModelKind: document.Model.Kind, ModelCode: document.Model.Code, ModelVersion: document.Model.Version,
		PresentationSource: document.PresentationProfile.Source,
	}
	payload, err := json.Marshal(struct {
		Evidence artifactEvidence `json:"evidence"`
		Codes    []string         `json:"visible_factor_codes"`
	}{Evidence: evidence, Codes: codes})
	if err != nil {
		return artifactEvidence{}, nil, err
	}
	evidence.Fingerprint = hashBytes(payload)
	return evidence, codes, nil
}

func validateArtifactCodes(root map[string]any, codes []string) error {
	catalog, err := arrayField(root, "factor_catalog")
	if err != nil || len(catalog) == 0 {
		return errors.New("frozen factor_catalog is required to validate artifact presentation")
	}
	known := make(map[string]struct{}, len(catalog))
	for index, item := range catalog {
		entry, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("factor_catalog[%d] must be an object", index)
		}
		code, ok := entry["code"].(string)
		if !ok || code == "" || code != strings.TrimSpace(code) {
			return fmt.Errorf("factor_catalog[%d].code is invalid", index)
		}
		known[code] = struct{}{}
	}
	for _, code := range codes {
		if _, ok := known[code]; !ok {
			return fmt.Errorf("artifact presentation factor %s is absent from frozen factor_catalog", code)
		}
	}
	return nil
}

func emptySectionsState(reportSpec map[string]any) (string, error) {
	value, present := reportSpec["Sections"]
	if !present {
		return "missing", nil
	}
	if value == nil {
		return "null", nil
	}
	sections, ok := value.([]any)
	if !ok {
		return "", errors.New("InterpretationAssets.ReportSpec.Sections must be an array, null or missing")
	}
	if len(sections) != 0 {
		return "", errors.New("InterpretationAssets.ReportSpec.Sections is not empty")
	}
	return "empty_array", nil
}

func scanOutcomesWithPlanner(ctx context.Context, db *sql.DB, planner func(outcomeRow) (reportPlan, error), visit func(outcomeRow, reportPlan) error) (inventory, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, model_kind, model_code, model_version, evaluated_at, COALESCE(report_input_json, '')
FROM evaluation_outcome
ORDER BY id`)
	if err != nil {
		return inventory{}, fmt.Errorf("query evaluation outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	state := inventory{}
	for rows.Next() {
		var row outcomeRow
		if err := rows.Scan(&row.ID, &row.ModelKind, &row.ModelCode, &row.ModelVersion, &row.EvaluatedAt, &row.ReportInput); err != nil {
			return state, fmt.Errorf("scan evaluation outcome: %w", err)
		}
		state.Total++
		plan, err := planner(row)
		if err != nil {
			return state, fmt.Errorf("outcome %d %s@%s: %w", row.ID, row.ModelCode, row.ModelVersion, err)
		}
		if plan.Changed {
			state.Candidates++
			if plan.Materialization != nil {
				state.MaterializedSections++
			}
		} else {
			state.Explicit++
		}
		if visit != nil {
			if err := visit(row, plan); err != nil {
				return state, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return state, fmt.Errorf("iterate evaluation outcomes: %w", err)
	}
	return state, nil
}

func planReportInput(raw, modelKind string) (reportPlan, error) {
	return planReportInputWithMaterialization(raw, modelKind, nil)
}

func planReportInputWithMaterialization(raw, modelKind string, materialization *reportSpecMaterialization) (reportPlan, error) {
	root, canonical, err := decodeCanonicalJSON(raw)
	if err != nil {
		return reportPlan{}, err
	}
	if _, err := evaluationinput.SnapshotFromReportInput([]byte(raw), evaluationinput.ModelRef{}); err != nil {
		return reportPlan{}, fmt.Errorf("invalid frozen report input: %w", err)
	}
	assets, err := objectField(root, "InterpretationAssets")
	if err != nil {
		return reportPlan{}, err
	}
	reportSpec, err := objectField(assets, "ReportSpec")
	if err != nil {
		return reportPlan{}, err
	}
	sections, err := arrayField(reportSpec, "Sections")
	if err != nil || len(sections) == 0 {
		if materialization == nil {
			return reportPlan{}, errReportSectionsRequired
		}
		state, stateErr := emptySectionsState(reportSpec)
		if stateErr != nil {
			return reportPlan{}, stateErr
		}
		if state != materialization.OriginalSectionsState {
			return reportPlan{}, errors.New("InterpretationAssets.ReportSpec.Sections state changed")
		}
		encodedSection, marshalErr := json.Marshal(materialization.Section)
		if marshalErr != nil {
			return reportPlan{}, marshalErr
		}
		var section map[string]any
		if unmarshalErr := json.Unmarshal(encodedSection, &section); unmarshalErr != nil {
			return reportPlan{}, unmarshalErr
		}
		sections = []any{section}
		reportSpec["Sections"] = sections
	} else if materialization != nil {
		return reportPlan{}, errors.New("report section materialization is no longer applicable")
	}

	templateIDs, versions := map[string]struct{}{}, map[string]struct{}{}
	deltas := make([]sectionDelta, 0, len(sections))
	sectionObjects := make([]map[string]any, 0, len(sections))
	for index, item := range sections {
		section, ok := item.(map[string]any)
		if !ok {
			return reportPlan{}, fmt.Errorf("ReportSpec.Sections[%d] must be an object", index)
		}
		id, err := stringFieldState(section, "TemplateID")
		if err != nil {
			return reportPlan{}, fmt.Errorf("ReportSpec.Sections[%d]: %w", index, err)
		}
		version, err := stringFieldState(section, "TemplateVersion")
		if err != nil {
			return reportPlan{}, fmt.Errorf("ReportSpec.Sections[%d]: %w", index, err)
		}
		if id.Value != "" {
			templateIDs[id.Value] = struct{}{}
		}
		if version.Value != "" {
			versions[version.Value] = struct{}{}
		}
		deltas = append(deltas, sectionDelta{Index: index, OriginalTemplateID: id, OriginalTemplateVersion: version})
		sectionObjects = append(sectionObjects, section)
	}

	var typology map[string]any
	var typologyDelta *routeDelta
	if modelKind == "typology" {
		typology, err = objectField(root, "typology_routing")
		if err != nil {
			return reportPlan{}, errors.New("typology_routing is required for typology outcome")
		}
		id, err := stringFieldState(typology, "template_id")
		if err != nil {
			return reportPlan{}, err
		}
		version, err := stringFieldState(typology, "template_version")
		if err != nil {
			return reportPlan{}, err
		}
		if id.Value != "" {
			templateIDs[id.Value] = struct{}{}
		}
		if version.Value != "" {
			versions[version.Value] = struct{}{}
		}
		typologyDelta = &routeDelta{OriginalTemplateID: id, OriginalTemplateVersion: version}
	}

	templateID, err := resolveTemplateID(modelKind, templateIDs)
	if err != nil {
		return reportPlan{}, err
	}
	templateVersion, err := resolveTemplateVersion(versions)
	if err != nil {
		return reportPlan{}, err
	}
	changed := materialization != nil
	for _, section := range sectionObjects {
		if value, _ := section["TemplateID"].(string); value != templateID {
			section["TemplateID"] = templateID
			changed = true
		}
		if value, _ := section["TemplateVersion"].(string); value != templateVersion {
			section["TemplateVersion"] = templateVersion
			changed = true
		}
	}
	if typology != nil {
		if value, _ := typology["template_id"].(string); value != templateID {
			typology["template_id"] = templateID
			changed = true
		}
		if value, _ := typology["template_version"].(string); value != templateVersion {
			typology["template_version"] = templateVersion
			changed = true
		}
	}
	targetJSON, err := json.Marshal(root)
	if err != nil {
		return reportPlan{}, fmt.Errorf("encode governed report input: %w", err)
	}
	if _, err := evaluationinput.SnapshotFromReportInput(targetJSON, evaluationinput.ModelRef{}); err != nil {
		return reportPlan{}, fmt.Errorf("governed report input is invalid: %w", err)
	}
	return reportPlan{
		Changed: changed, TemplateID: templateID, TemplateVersion: templateVersion,
		SourceSemanticHash: hashBytes(canonical), TargetSemanticHash: hashBytes(targetJSON), TargetJSON: string(targetJSON),
		Sections: deltas, TypologyRouting: typologyDelta, Materialization: materialization,
	}, nil
}

func resolveTemplateID(modelKind string, values map[string]struct{}) (string, error) {
	if modelKind != "typology" && len(values) == 0 {
		return "standard", nil
	}
	if len(values) != 1 {
		return "", fmt.Errorf("report template_id must resolve exactly once, found %d", len(values))
	}
	var value string
	for item := range values {
		value = item
	}
	if _, ok := knownTemplateIDs[value]; !ok {
		return "", fmt.Errorf("unknown report template_id %q", value)
	}
	if modelKind == "typology" && value == "standard" {
		return "", errors.New("typology outcome cannot use standard template")
	}
	if modelKind != "typology" && value != "standard" {
		return "", fmt.Errorf("non-typology outcome cannot use template %q", value)
	}
	return value, nil
}

func resolveTemplateVersion(values map[string]struct{}) (string, error) {
	if len(values) == 0 {
		return targetLegacyVersion, nil
	}
	if len(values) != 1 {
		return "", fmt.Errorf("report template_version must resolve exactly once, found %d", len(values))
	}
	for value := range values {
		if !oneOf(value, targetLegacyVersion, targetCurrentVersion) {
			return "", fmt.Errorf("unknown report template_version %q", value)
		}
		return value, nil
	}
	panic("unreachable")
}

func mutate(ctx context.Context, db *sql.DB, records []record, operation string, stdout io.Writer) error {
	ordered := append([]record(nil), records...)
	if operation == "rollback" {
		for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
			ordered[left], ordered[right] = ordered[right], ordered[left]
		}
	}
	updated, already := 0, 0
	for start := 0; start < len(ordered); start += transactionBatchSize {
		end := start + transactionBatchSize
		if end > len(ordered) {
			end = len(ordered)
		}
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return err
		}
		batchUpdated, batchAlready, err := mutateBatch(ctx, tx, ordered[start:end], operation)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s batch: %w", operation, err)
		}
		updated += batchUpdated
		already += batchAlready
	}
	return writeResult(stdout, map[string]any{"operation": operation, "selected": len(records), "updated": updated, "already": already})
}

func mutateBatch(ctx context.Context, tx *sql.Tx, records []record, operation string) (int, int, error) {
	selectStmt, err := tx.PrepareContext(ctx, `SELECT COALESCE(report_input_json, '') FROM evaluation_outcome WHERE id = ? FOR UPDATE`)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = selectStmt.Close() }()
	updateStmt, err := tx.PrepareContext(ctx, `UPDATE evaluation_outcome SET report_input_json = ? WHERE id = ? AND report_input_json = ?`)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = updateStmt.Close() }()
	updated, already := 0, 0
	for _, item := range records {
		var raw string
		if err := selectStmt.QueryRowContext(ctx, item.OutcomeID).Scan(&raw); err != nil {
			return updated, already, fmt.Errorf("load outcome %d: %w", item.OutcomeID, err)
		}
		canonical, err := canonicalJSON(raw)
		if err != nil {
			return updated, already, fmt.Errorf("outcome %d: %w", item.OutcomeID, err)
		}
		currentHash := hashBytes(canonical)
		var desired string
		if operation == "apply" {
			if currentHash == item.TargetSemanticHash {
				already++
				continue
			}
			if currentHash != item.SourceSemanticHash {
				return updated, already, fmt.Errorf("outcome %d source changed", item.OutcomeID)
			}
			plan, err := planReportInputWithMaterialization(raw, item.ModelKind, item.Materialization)
			if err != nil || !plan.Changed || plan.TargetSemanticHash != item.TargetSemanticHash || plan.TemplateID != item.TemplateID || plan.TemplateVersion != item.TemplateVersion {
				return updated, already, fmt.Errorf("outcome %d no longer matches governance manifest", item.OutcomeID)
			}
			desired = plan.TargetJSON
		} else {
			if currentHash == item.SourceSemanticHash {
				already++
				continue
			}
			if currentHash != item.TargetSemanticHash {
				return updated, already, fmt.Errorf("outcome %d target changed", item.OutcomeID)
			}
			desired, err = restoreReportInput(raw, item)
			if err != nil {
				return updated, already, fmt.Errorf("outcome %d: %w", item.OutcomeID, err)
			}
		}
		result, err := updateStmt.ExecContext(ctx, desired, item.OutcomeID, raw)
		if err != nil {
			return updated, already, fmt.Errorf("update outcome %d: %w", item.OutcomeID, err)
		}
		matched, err := result.RowsAffected()
		if err != nil || matched != 1 {
			return updated, already, fmt.Errorf("outcome %d lost report_input CAS", item.OutcomeID)
		}
		updated++
	}
	return updated, already, nil
}

func restoreReportInput(raw string, item record) (string, error) {
	root, _, err := decodeCanonicalJSON(raw)
	if err != nil {
		return "", err
	}
	assets, err := objectField(root, "InterpretationAssets")
	if err != nil {
		return "", err
	}
	reportSpec, err := objectField(assets, "ReportSpec")
	if err != nil {
		return "", err
	}
	sections, err := arrayField(reportSpec, "Sections")
	if err != nil {
		return "", err
	}
	for _, delta := range item.Sections {
		if delta.Index < 0 || delta.Index >= len(sections) {
			return "", errors.New("section layout changed")
		}
		section, ok := sections[delta.Index].(map[string]any)
		if !ok {
			return "", errors.New("section layout changed")
		}
		restoreStringField(section, "TemplateID", delta.OriginalTemplateID)
		restoreStringField(section, "TemplateVersion", delta.OriginalTemplateVersion)
	}
	if item.Materialization != nil {
		switch item.Materialization.OriginalSectionsState {
		case "missing":
			delete(reportSpec, "Sections")
		case "null":
			reportSpec["Sections"] = nil
		case "empty_array":
			reportSpec["Sections"] = []any{}
		default:
			return "", errors.New("unsupported original report sections state")
		}
	}
	if item.TypologyRouting != nil {
		route, err := objectField(root, "typology_routing")
		if err != nil {
			return "", err
		}
		restoreStringField(route, "template_id", item.TypologyRouting.OriginalTemplateID)
		restoreStringField(route, "template_version", item.TypologyRouting.OriginalTemplateVersion)
	}
	desired, err := json.Marshal(root)
	if err != nil {
		return "", err
	}
	if hashBytes(desired) != item.SourceSemanticHash {
		return "", errors.New("rollback did not restore source semantics")
	}
	return string(desired), nil
}

func verify(ctx context.Context, db *sql.DB, records []record, artifacts map[uint64]artifactCandidate, requireComplete bool, stdout io.Writer) error {
	expected := make(map[uint64]string, len(records))
	for _, item := range records {
		expected[item.OutcomeID] = item.TargetSemanticHash
	}
	verified := 0
	state, err := scanOutcomesWithPlanner(ctx, db, func(row outcomeRow) (reportPlan, error) {
		plan, planErr := planReportInput(row.ReportInput, row.ModelKind)
		if planErr == nil {
			return plan, nil
		}
		if !errors.Is(planErr, errReportSectionsRequired) || artifacts == nil {
			return reportPlan{}, planErr
		}
		materialization, materializeErr := materializationFromArtifact(row, artifacts[row.ID])
		if materializeErr != nil {
			return reportPlan{}, materializeErr
		}
		return planReportInputWithMaterialization(row.ReportInput, row.ModelKind, materialization)
	}, func(row outcomeRow, _ reportPlan) error {
		want, ok := expected[row.ID]
		if !ok {
			return nil
		}
		canonical, err := canonicalJSON(row.ReportInput)
		if err != nil || hashBytes(canonical) != want {
			return fmt.Errorf("outcome %d target verification failed", row.ID)
		}
		verified++
		return nil
	})
	if err != nil {
		return err
	}
	if verified != len(records) {
		return fmt.Errorf("verified %d of %d selected outcomes", verified, len(records))
	}
	if requireComplete && state.Candidates != 0 {
		return fmt.Errorf("outcome template routes are incomplete: %d candidates remain", state.Candidates)
	}
	return writeResult(stdout, map[string]any{
		"operation": "verify", "verified": verified, "inventory": state, "require_complete": requireComplete,
	})
}

func validateArtifactEvidence(records []record, artifacts map[uint64]artifactCandidate) error {
	for _, item := range records {
		if item.Materialization == nil {
			continue
		}
		row := outcomeRow{ID: item.OutcomeID, ModelKind: item.ModelKind, ModelCode: item.ModelCode, ModelVersion: item.ModelVersion}
		evidence, codes, err := artifactEvidenceFor(row, artifacts[item.OutcomeID])
		if err != nil {
			return err
		}
		if evidence != item.Materialization.Artifact || !equalStrings(codes, item.Materialization.Section.SourceRefs) {
			return fmt.Errorf("outcome %d artifact evidence changed after audit", item.OutcomeID)
		}
	}
	return nil
}

func hasReportSpecMaterialization(records []record) bool {
	for _, item := range records {
		if item.Materialization != nil {
			return true
		}
	}
	return false
}

func selectedRecords(records []record, cfg config) []record {
	selected := make([]record, 0, len(records))
	for _, item := range records {
		if item.OutcomeID <= cfg.AfterID {
			continue
		}
		selected = append(selected, item)
		if cfg.MaxRecords > 0 && len(selected) == cfg.MaxRecords {
			break
		}
	}
	return selected
}

func validateManifest(value manifest, database, mongoDatabase string) error {
	if value.SchemaVersion != governanceSchemaVersion || value.Database != database || value.Table != "evaluation_outcome" || value.TargetLegacy != targetLegacyVersion ||
		value.MongoDatabase != mongoDatabase || value.ArtifactCollection != artifactCollection {
		return errors.New("outcome template route governance target mismatch")
	}
	fingerprint, err := recordsFingerprint(value.Records)
	if err != nil {
		return err
	}
	if value.RecordsFingerprint != fingerprint {
		return errors.New("outcome template route governance fingerprint mismatch")
	}
	var previous uint64
	for index, item := range value.Records {
		if item.OutcomeID == 0 || (index > 0 && item.OutcomeID <= previous) || !isSHA256(item.SourceRawHash) ||
			!isSHA256(item.SourceSemanticHash) || !isSHA256(item.TargetSemanticHash) ||
			item.SourceSemanticHash == item.TargetSemanticHash || len(item.Sections) == 0 {
			return fmt.Errorf("invalid governance record at index %d", index)
		}
		if _, ok := knownTemplateIDs[item.TemplateID]; !ok || !oneOf(item.TemplateVersion, targetLegacyVersion, targetCurrentVersion) {
			return fmt.Errorf("invalid governance route at index %d", index)
		}
		if item.ModelKind == "typology" && (item.TemplateID == "standard" || item.TypologyRouting == nil) {
			return fmt.Errorf("invalid typology governance route at index %d", index)
		}
		if item.ModelKind != "typology" && (item.TemplateID != "standard" || item.TypologyRouting != nil) {
			return fmt.Errorf("invalid non-typology governance route at index %d", index)
		}
		if item.Materialization != nil {
			materialization := item.Materialization
			if !oneOf(materialization.OriginalSectionsState, "missing", "null", "empty_array") ||
				materialization.Section.Code != "factor_scores" || materialization.Section.Kind != "factor_scores" ||
				materialization.Section.TemplateID != item.TemplateID || materialization.Section.TemplateVersion != item.TemplateVersion ||
				materialization.Artifact.OutcomeID != item.OutcomeID || materialization.Artifact.ArtifactID == 0 ||
				!isSHA256(materialization.Artifact.Fingerprint) || materialization.Artifact.TemplateVersion != targetLegacyVersion ||
				materialization.Artifact.ModelKind != item.ModelKind || materialization.Artifact.ModelCode != item.ModelCode ||
				materialization.Artifact.ModelVersion != item.ModelVersion {
				return fmt.Errorf("invalid report section materialization at index %d", index)
			}
		}
		previousSection := -1
		for _, section := range item.Sections {
			if section.Index <= previousSection {
				return fmt.Errorf("invalid section delta at record index %d", index)
			}
			previousSection = section.Index
		}
		previous = item.OutcomeID
	}
	return nil
}

func recordsFingerprint(records []record) (string, error) {
	data, err := json.Marshal(records)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func readManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, err
	}
	var value manifest
	if err := json.Unmarshal(data, &value); err != nil {
		return manifest{}, err
	}
	return value, nil
}

func writeManifest(path string, value manifest) error {
	if _, err := os.Stat(path); err == nil {
		return errors.New("refusing to overwrite existing governance manifest")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := filepath.Clean(path) + ".partial"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(temporary)
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(temporary, path)
}

func decodeCanonicalJSON(raw string) (map[string]any, []byte, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil, errors.New("report_input_json is required")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, nil, fmt.Errorf("invalid report_input_json: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, nil, errors.New("report_input_json contains trailing data")
		}
		return nil, nil, fmt.Errorf("invalid trailing report_input_json: %w", err)
	}
	canonical, err := json.Marshal(root)
	if err != nil {
		return nil, nil, err
	}
	return root, canonical, nil
}

func canonicalJSON(raw string) ([]byte, error) {
	_, canonical, err := decodeCanonicalJSON(raw)
	return canonical, err
}

func objectField(parent map[string]any, key string) (map[string]any, error) {
	value, ok := parent[key].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return value, nil
}

func arrayField(parent map[string]any, key string) ([]any, error) {
	value, ok := parent[key].([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	return value, nil
}

func stringFieldState(parent map[string]any, key string) (fieldState, error) {
	value, present := parent[key]
	if !present {
		return fieldState{}, nil
	}
	stringValue, ok := value.(string)
	if !ok {
		return fieldState{}, fmt.Errorf("%s must be a string", key)
	}
	if stringValue != strings.TrimSpace(stringValue) {
		return fieldState{}, fmt.Errorf("%s contains surrounding whitespace", key)
	}
	return fieldState{Present: true, Value: stringValue}, nil
}

func restoreStringField(parent map[string]any, key string, state fieldState) {
	if !state.Present {
		delete(parent, key)
		return
	}
	parent[key] = state.Value
}

func hashString(value string) string { return hashBytes([]byte(value)) }

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func writeResult(output io.Writer, value any) error {
	if _, err := io.WriteString(output, "EVALUATION_OUTCOME_TEMPLATE_ROUTE_GOVERNANCE_RESULT\n"); err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
