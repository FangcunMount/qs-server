// govern_interpretation_model_template_routes enriches a route-governance
// manifest with canonical DefinitionV2 hashes. The manifest and transaction
// engine remain in govern.js; this helper deliberately reuses the exact Go
// domain mapper and hash implementation used by the running service.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	governanceSchemaVersion = "interpretation-model-template-route-governance/v1"
	targetTemplateVersion   = "2026-08-v1"
	manifestPathEnv         = "MODEL_TEMPLATE_ROUTE_MANIFEST_PATH"
)

type governanceManifest struct {
	SchemaVersion         string           `json:"schema_version"`
	Database              string           `json:"database"`
	Collection            string           `json:"collection"`
	GeneratedAt           string           `json:"generated_at"`
	TargetTemplateVersion string           `json:"target_template_version"`
	Records               []map[string]any `json:"records"`
	RecordsFingerprint    string           `json:"records_fingerprint"`
}

type mongoConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
}

func main() {
	if err := run(context.Background(), os.Getenv, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "enrich model template route manifest:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, stdout *os.File) error {
	manifestPath := strings.TrimSpace(getenv(manifestPathEnv))
	if manifestPath == "" {
		return fmt.Errorf("%s is required", manifestPathEnv)
	}
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return err
	}
	if err := validateManifest(manifest, false); err != nil {
		return err
	}
	cfg, err := readMongoConfig(getenv)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	clientOptions := options.Client().
		SetHosts([]string{net.JoinHostPort(cfg.Host, cfg.Port)}).
		SetAuth(options.Credential{AuthSource: "admin", Username: cfg.Username, Password: cfg.Password}).
		SetConnectTimeout(30 * time.Second)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("connect MongoDB: %w", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("ping MongoDB primary: %w", err)
	}
	if cfg.Database != manifest.Database || manifest.Collection != "assessment_models" {
		return fmt.Errorf("manifest target mismatch: %s.%s", manifest.Database, manifest.Collection)
	}

	collection := client.Database(cfg.Database).Collection(manifest.Collection)
	for index, record := range manifest.Records {
		if err := enrichRecord(ctx, collection, record); err != nil {
			return fmt.Errorf("records[%d]: %w", index, err)
		}
	}
	manifest.RecordsFingerprint, err = recordsFingerprint(manifest.Records)
	if err != nil {
		return err
	}
	if err := validateManifest(manifest, true); err != nil {
		return err
	}
	if err := writeManifest(manifestPath, manifest); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "MODEL_TEMPLATE_ROUTE_CANONICAL_HASHES_OK records=%d fingerprint=%s\n", len(manifest.Records), manifest.RecordsFingerprint)
	return err
}

func readMongoConfig(getenv func(string) string) (mongoConfig, error) {
	cfg := mongoConfig{
		Host: strings.TrimSpace(getenv("MONGODB_HOST")), Port: strings.TrimSpace(getenv("MONGODB_PORT")),
		Username: getenv("MONGODB_USERNAME"), Password: getenv("MONGODB_PASSWORD"),
		Database: strings.TrimSpace(getenv("MONGODB_DBNAME")),
	}
	if cfg.Port == "" {
		cfg.Port = "27017"
	}
	if cfg.Host == "" || cfg.Username == "" || cfg.Password == "" || cfg.Database == "" {
		return mongoConfig{}, errors.New("MONGODB_HOST, MONGODB_USERNAME, MONGODB_PASSWORD and MONGODB_DBNAME are required")
	}
	return cfg, nil
}

func readManifest(path string) (governanceManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return governanceManifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest governanceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return governanceManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return manifest, nil
}

func validateManifest(manifest governanceManifest, requireHashes bool) error {
	if manifest.SchemaVersion != governanceSchemaVersion || manifest.TargetTemplateVersion != targetTemplateVersion {
		return errors.New("unsupported model template route governance manifest")
	}
	fingerprint, err := recordsFingerprint(manifest.Records)
	if err != nil {
		return err
	}
	if fingerprint != manifest.RecordsFingerprint {
		return errors.New("model template route governance fingerprint mismatch")
	}
	for index, record := range manifest.Records {
		if _, err := recordObjectID(record, "source_id"); err != nil {
			return fmt.Errorf("records[%d]: %w", index, err)
		}
		if _, ok := record["factor_score_section_added"].(bool); !ok {
			return fmt.Errorf("records[%d]: factor_score_section_added is required", index)
		}
		if _, err := recordStrings(record, "factor_source_refs"); err != nil {
			return fmt.Errorf("records[%d]: %w", index, err)
		}
		if requireHashes && (!isSHA256(recordString(record, "source_definition_hash")) || !isSHA256(recordString(record, "target_definition_hash"))) {
			return fmt.Errorf("records[%d]: canonical definition hashes are required", index)
		}
	}
	return nil
}

func recordsFingerprint(records []map[string]any) (string, error) {
	data, err := json.Marshal(records)
	if err != nil {
		return "", fmt.Errorf("encode manifest records: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type snapshotFinder interface {
	FindOne(context.Context, interface{}, ...*options.FindOneOptions) *mongo.SingleResult
}

func enrichRecord(ctx context.Context, collection snapshotFinder, record map[string]any) error {
	id, err := recordObjectID(record, "source_id")
	if err != nil {
		return err
	}
	var po mongomodelcatalog.PublishedAssessmentModelPO
	if err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&po); err != nil {
		return fmt.Errorf("load source snapshot: %w", err)
	}
	return attachCanonicalHashes(record, &po)
}

func attachCanonicalHashes(record map[string]any, po *mongomodelcatalog.PublishedAssessmentModelPO) error {
	if po == nil || po.DefinitionV2 == nil {
		return errors.New("source DefinitionV2 is required")
	}
	if po.Kind != recordString(record, "kind") || po.Code != recordString(record, "code") || po.ReleaseVersion != recordString(record, "source_release_version") {
		return errors.New("source snapshot identity changed")
	}
	model := mongomodelcatalog.NewMapper().ToPublished(po)
	sourceHash, err := modeldefinition.CanonicalContentHash(model.DefinitionV2)
	if err != nil {
		return fmt.Errorf("hash source DefinitionV2: %w", err)
	}
	if stored := modelcatalogport.DefinitionHashFromSource(model.Source); stored == "" || stored != sourceHash {
		return errors.New("stored source definition hash does not match canonical DefinitionV2")
	}
	target, err := cloneDefinition(model.DefinitionV2)
	if err != nil {
		return err
	}
	templateID := recordString(record, "template_id")
	if templateID == "" {
		return errors.New("target template route is incomplete")
	}
	added, sourceRefs, err := materializeFactorScoreSection(target, po.Kind)
	if err != nil {
		return err
	}
	manifestAdded, _ := record["factor_score_section_added"].(bool)
	manifestRefs, err := recordStrings(record, "factor_source_refs")
	if err != nil {
		return err
	}
	if manifestAdded != added || !equalStrings(manifestRefs, sourceRefs) {
		return errors.New("factor score governance plan does not match source DefinitionV2")
	}
	for index := range target.ReportMap.Sections {
		target.ReportMap.Sections[index].TemplateID = templateID
		target.ReportMap.Sections[index].TemplateVersion = targetTemplateVersion
	}
	modeldefinition.MaterializeLayers(target)
	targetHash, err := modeldefinition.CanonicalContentHash(target)
	if err != nil {
		return fmt.Errorf("hash target DefinitionV2: %w", err)
	}
	if sourceHash == targetHash {
		return errors.New("target DefinitionV2 hash did not change")
	}
	record["source_definition_hash"] = sourceHash
	record["target_definition_hash"] = targetHash
	return nil
}

func materializeFactorScoreSection(target *modeldefinition.Definition, kind string) (bool, []string, error) {
	if target == nil {
		return false, nil, errors.New("target DefinitionV2 is required")
	}
	if kind == "typology" {
		if len(target.ReportMap.Sections) == 0 {
			return false, nil, errors.New("typology target report_map.sections are required")
		}
		return false, []string{}, nil
	}
	if kind != "scale" && kind != "behavioral_rating" && kind != "cognitive" {
		return false, nil, fmt.Errorf("unsupported published snapshot kind: %s", kind)
	}
	if len(target.Measure.Factors) == 0 {
		return false, nil, errors.New("factor model measure.factors are required")
	}
	factorCodes := make([]string, 0, len(target.Measure.Factors))
	knownCodes := make(map[string]struct{}, len(target.Measure.Factors))
	for _, factor := range target.Measure.Factors {
		code := factor.Code
		if code == "" || code != strings.TrimSpace(code) {
			return false, nil, errors.New("factor model measure factor code must be non-blank and normalized")
		}
		if _, duplicate := knownCodes[code]; duplicate {
			return false, nil, fmt.Errorf("factor model measure factor code is duplicated: %s", code)
		}
		knownCodes[code] = struct{}{}
		factorCodes = append(factorCodes, code)
	}
	factorSection := -1
	for index := range target.ReportMap.Sections {
		if target.ReportMap.Sections[index].Kind != modeldefinition.ReportSectionKindFactorScores {
			continue
		}
		if factorSection >= 0 {
			return false, nil, errors.New("factor model may contain only one factor_scores report section")
		}
		factorSection = index
	}
	if factorSection < 0 {
		target.ReportMap.Sections = append(target.ReportMap.Sections, modeldefinition.ReportSection{
			Code:       modeldefinition.ReportSectionKindFactorScores,
			Kind:       modeldefinition.ReportSectionKindFactorScores,
			SourceRefs: append([]string(nil), factorCodes...),
		})
		return true, factorCodes, nil
	}
	refs := append([]string(nil), target.ReportMap.Sections[factorSection].SourceRefs...)
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if ref == "" || ref != strings.TrimSpace(ref) {
			return false, nil, errors.New("factor_scores source_ref must be non-blank and normalized")
		}
		if _, duplicate := seen[ref]; duplicate {
			return false, nil, fmt.Errorf("factor_scores source_ref is duplicated: %s", ref)
		}
		if _, exists := knownCodes[ref]; !exists {
			return false, nil, fmt.Errorf("factor_scores source_ref is not a measure factor: %s", ref)
		}
		seen[ref] = struct{}{}
	}
	return false, refs, nil
}

func cloneDefinition(source *modeldefinition.Definition) (*modeldefinition.Definition, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("encode source DefinitionV2: %w", err)
	}
	var target modeldefinition.Definition
	if err := json.Unmarshal(data, &target); err != nil {
		return nil, fmt.Errorf("decode source DefinitionV2: %w", err)
	}
	return &target, nil
}

func recordObjectID(record map[string]any, field string) (primitive.ObjectID, error) {
	value := recordString(record, field)
	if value == "" {
		return primitive.NilObjectID, fmt.Errorf("%s is required", field)
	}
	id, err := primitive.ObjectIDFromHex(value)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("%s is invalid", field)
	}
	return id, nil
}

func recordString(record map[string]any, field string) string {
	value, _ := record[field].(string)
	return strings.TrimSpace(value)
}

func recordStrings(record map[string]any, field string) ([]string, error) {
	values, ok := record[field].([]any)
	if !ok {
		return nil, fmt.Errorf("%s is required", field)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s must contain only strings", field)
		}
		result = append(result, item)
	}
	return result, nil
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

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func writeManifest(path string, manifest governanceManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode enriched manifest: %w", err)
	}
	data = append(data, '\n')
	temporary := filepath.Clean(path) + ".hash.partial"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create enriched manifest: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(temporary)
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write enriched manifest: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync enriched manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close enriched manifest: %w", err)
	}
	closed = true
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace enriched manifest: %w", err)
	}
	return nil
}
