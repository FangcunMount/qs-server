package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type projectionRecord struct {
	ModelID                  string         `json:"model_id,omitempty"`
	Code                     string         `json:"code,omitempty"`
	Role                     string         `json:"role,omitempty"`
	Revision                 string         `json:"revision,omitempty"`
	Fingerprint              string         `json:"fingerprint"`
	Status                   string         `json:"status"`
	Historical               bool           `json:"historical"`
	HashMetadataStatus       string         `json:"hash_metadata_status"`
	HashMetadataPaths        []string       `json:"hash_metadata_paths,omitempty"`
	HashMetadataReasonCounts map[string]int `json:"hash_metadata_reason_counts,omitempty"`
	mongomodelcatalog.DefinitionProjectionAudit
}

type projectionReport struct {
	Complete                 bool               `json:"complete"`
	ZeroMismatch             bool               `json:"zero_mismatch"`
	Expected                 int64              `json:"expected"`
	Scanned                  int64              `json:"scanned"`
	Compared                 int64              `json:"compared"`
	Matched                  int64              `json:"matched"`
	Mismatched               int64              `json:"mismatched"`
	MissingDefinition        int64              `json:"missing_definition"`
	UnknownSchema            int64              `json:"unknown_schema"`
	Invalid                  int64              `json:"invalid"`
	RecordErrors             int64              `json:"record_errors"`
	ScanErrors               int64              `json:"scan_errors"`
	Heads                    int64              `json:"heads"`
	Snapshots                int64              `json:"snapshots"`
	Historical               int64              `json:"historical"`
	StoredDecisionFields     int64              `json:"stored_decision_fields"`
	StoredAssetFields        int64              `json:"stored_asset_fields"`
	HashMetadataMatched      int64              `json:"hash_metadata_matched"`
	HashMetadataInvalid      int64              `json:"hash_metadata_invalid"`
	HashMetadataUnverifiable int64              `json:"hash_metadata_unverifiable"`
	Truncated                bool               `json:"truncated"`
	Reason                   string             `json:"reason,omitempty"`
	Records                  []projectionRecord `json:"records"`
}

func projectionConfig(cfg config) (config, error) {
	if cfg.normVersion != "" || cfg.batchSize < 1 || cfg.batchSize > 1000 || cfg.maxRecords < 1 || cfg.maxRecords > 100000 || cfg.timeout <= 0 || cfg.timeout > 5*time.Minute {
		return cfg, errors.New("invalid_audit_bounds_or_mode")
	}
	if cfg.uriFromFlag {
		parsed, err := url.Parse(cfg.mongoURI)
		if err != nil || parsed.User != nil || cfg.privateConfig != "" {
			return cfg, errors.New("credentials_or_private_config_require_non_cli_uri")
		}
		for key := range parsed.Query() {
			if strings.Contains(strings.ToLower(key), "password") || strings.EqualFold(key, "authMechanismProperties") || strings.EqualFold(key, "proxyUsername") {
				return cfg, errors.New("credentials_require_non_cli_uri")
			}
		}
	}
	if cfg.privateConfig != "" {
		info, err := os.Lstat(cfg.privateConfig)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16*1024 {
			return cfg, errors.New("private_config_requires_small_regular_owner_only_file")
		}
		file, err := os.Open(cfg.privateConfig)
		if err != nil {
			return cfg, errors.New("private_config_unreadable")
		}
		defer func() { _ = file.Close() }()
		var connection struct {
			MongoURI string `json:"mongo_uri"`
			MongoDB  string `json:"mongo_db"`
		}
		decoder := json.NewDecoder(io.LimitReader(file, 16*1024+1))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&connection); err != nil {
			return cfg, errors.New("private_config_invalid")
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return cfg, errors.New("private_config_invalid")
		}
		cfg.mongoURI = connection.MongoURI
		if connection.MongoDB != "" {
			cfg.mongoDB = connection.MongoDB
		}
	}
	if strings.TrimSpace(cfg.mongoURI) == "" || strings.TrimSpace(cfg.mongoDB) == "" {
		return cfg, errors.New("mongo_connection_required")
	}
	return cfg, nil
}

func runDefinitionProjectionAudit(cfg config, output io.Writer) int {
	cfg, err := projectionConfig(cfg)
	if err != nil {
		_ = json.NewEncoder(output).Encode(projectionReport{ScanErrors: 1, Reason: err.Error()})
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.mongoURI))
	if err != nil {
		_ = json.NewEncoder(output).Encode(projectionReport{ScanErrors: 1, Reason: "mongo_connect_failed"})
		return 1
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(cleanup)
	}()
	if err := client.Ping(ctx, nil); err != nil {
		_ = json.NewEncoder(output).Encode(projectionReport{ScanErrors: 1, Reason: "mongo_ping_failed"})
		return 1
	}
	result := auditDefinitionProjectionCollection(ctx, client.Database(cfg.mongoDB).Collection("assessment_models"), cfg)
	if err := json.NewEncoder(output).Encode(result); err != nil {
		return 1
	}
	return result.exitCode()
}

// A snapshot session fixes all batches and the count to one Mongo cluster time.
// The code issues only read commands: no repository/index installer, transaction
// committer, CAS update, delete or rematerialization capability is provided.
func auditDefinitionProjectionCollection(ctx context.Context, collection *mongo.Collection, cfg config) projectionReport {
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	result := projectionReport{Records: make([]projectionRecord, 0)}
	session, err := collection.Database().Client().StartSession(options.Session().SetSnapshot(true))
	if err != nil {
		result.ScanErrors, result.Reason = 1, "snapshot_session_failed"
		return result
	}
	defer session.EndSession(context.Background())
	err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		return scanProjectionRows(sc, collection, cfg, &result)
	})
	if err != nil {
		result.ScanErrors, result.Reason = 1, "snapshot_scan_failed"
		return result
	}
	result.Truncated = result.Expected > cfg.maxRecords
	result.Complete = !result.Truncated && result.Scanned == result.Expected
	if result.Truncated {
		result.Reason = "record_limit"
	} else if !result.Complete {
		result.ScanErrors, result.Reason = 1, "count_mismatch"
	} else if result.Scanned == 0 {
		result.Reason = "empty_collection"
	}
	result.ZeroMismatch = result.Complete && result.Scanned > 0 && result.Matched == result.Scanned && result.HashMetadataMatched == result.Snapshots && result.HashMetadataInvalid == 0 && result.HashMetadataUnverifiable == 0
	return result
}

// The scanner receives only the two read methods; it cannot call a writer.
type projectionReadSource interface {
	CountDocuments(context.Context, interface{}, ...*options.CountOptions) (int64, error)
	Find(context.Context, interface{}, ...*options.FindOptions) (*mongo.Cursor, error)
}

func scanProjectionRows(sc context.Context, collection projectionReadSource, cfg config, result *projectionReport) error {
	count, err := collection.CountDocuments(sc, bson.D{}, options.Count().SetMaxTime(cfg.timeout))
	if err != nil {
		return err
	}
	result.Expected = count
	cursor, err := collection.Find(sc, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetBatchSize(int32(cfg.batchSize)).SetLimit(cfg.maxRecords).SetMaxTime(cfg.timeout))
	if err != nil {
		return err
	}
	defer func() { _ = cursor.Close(sc) }()
	for cursor.Next(sc) {
		raw := cursor.Current
		record := auditProjectionRecord(raw)
		result.Records = append(result.Records, record)
		result.Scanned++
		switch record.HashMetadataStatus {
		case "matched":
			result.HashMetadataMatched++
		case "invalid":
			result.HashMetadataInvalid++
		case "unverifiable":
			result.HashMetadataUnverifiable++
		}
		switch record.Role {
		case "head":
			result.Heads++
		case "published_snapshot":
			result.Snapshots++
		}
		if record.Historical {
			result.Historical++
		}
		if record.CanonicalHash != "" && !record.HashError {
			result.Compared++
		}
		if record.DifferenceCount > 0 {
			result.Mismatched++
		}
		if def, ok := raw.Lookup("definition_v2").DocumentOK(); ok {
			if def.Lookup("decision_spec").Type != 0 {
				result.StoredDecisionFields++
			}
			if def.Lookup("interpretation_assets").Type != 0 {
				result.StoredAssetFields++
			}
		}
		switch record.Status {
		case "matched":
			result.Matched++
		case "missing_definition":
			result.MissingDefinition++
		case "unknown_schema":
			result.UnknownSchema++
		case "invalid":
			result.Invalid++
		case "error":
			result.RecordErrors++
		}
	}
	return cursor.Err()
}

func (r projectionReport) exitCode() int {
	if r.ScanErrors > 0 {
		return 1
	}
	if !r.Complete {
		return 3
	}
	if !r.ZeroMismatch {
		return 2
	}
	return 0
}

func auditProjectionRecord(raw bson.Raw) (record projectionRecord) {
	fingerprint := sha256.Sum256(raw)
	record = projectionRecord{Fingerprint: hex.EncodeToString(fingerprint[:]), Status: "invalid", HashMetadataStatus: "not_applicable"}
	var metadata struct {
		ID               primitive.ObjectID `bson:"_id"`
		Code             string             `bson:"code"`
		Kind             string             `bson:"kind"`
		Algorithm        string             `bson:"algorithm"`
		DecisionKind     string             `bson:"decision_kind"`
		ReleaseStatus    string             `bson:"release_status"`
		Role             string             `bson:"record_role"`
		Revision         int64              `bson:"revision"`
		ReleaseVersion   string             `bson:"release_version"`
		Schema           string             `bson:"schema_version"`
		DefinitionSchema string             `bson:"definition_schema_version"`
	}
	if err := bson.Unmarshal(raw, &metadata); err != nil {
		record.invalid("metadata", "metadata.decode_error")
		return record
	}
	if !metadata.ID.IsZero() {
		record.ModelID = metadata.ID.Hex()
	}
	record.Code, record.Role = metadata.Code, metadata.Role
	if record.Role == "published_snapshot" {
		defer auditPublishedDefinitionHash(raw, &record)
	}
	if record.Role == "head" {
		record.Revision = strconv.FormatInt(metadata.Revision, 10)
	} else {
		record.Revision = metadata.ReleaseVersion
	}
	// Metadata decoding is last-wins for duplicate BSON keys. A fingerprint
	// cannot make an ambiguous identity/revision safe to classify as matching.
	elements, err := raw.Elements()
	if err != nil {
		record.invalid("metadata", "raw.invalid_bson")
		return record
	}
	seen := map[string]bool{}
	for _, element := range elements {
		if seen[element.Key()] {
			record.invalid("metadata.duplicate_field", "raw.duplicate_key")
			return record
		}
		seen[element.Key()] = true
	}
	for _, field := range []string{"deleted_at", "archived_at", "release_archived_at"} {
		value := raw.Lookup(field)
		if value.Type != 0 && value.Type != bson.TypeNull {
			record.Historical = true
		}
	}
	if metadata.ReleaseStatus == "archived" {
		record.Historical = true
	}
	if metadata.ID.IsZero() || metadata.Code == "" || strings.TrimSpace(metadata.Code) != metadata.Code || record.Revision == "" || (record.Role == "head" && metadata.Revision <= 0) {
		record.invalid("metadata.identity_revision", "metadata.identity_revision_invalid")
		return record
	}
	definition := raw.Lookup("definition_v2")
	if definition.Type == 0 || definition.Type == bson.TypeNull {
		record.Status = "missing_definition"
		return record
	}
	if metadata.DefinitionSchema != domain.SchemaVersionV2 || (metadata.Schema != "" && metadata.Schema != domain.SchemaVersionV2) || (record.Role == "published_snapshot" && metadata.Schema != domain.SchemaVersionV2) || (record.Role != "head" && record.Role != "published_snapshot") {
		record.Status = "unknown_schema"
		record.invalid("metadata.schema_role", "metadata.schema_role_unsupported")
		return record
	}
	switch metadata.Kind {
	case "scale", "typology", "behavioral_rating", "cognitive":
	default:
		record.Status = "unknown_schema"
		record.invalid("metadata.kind", "metadata.kind_unsupported")
		return record
	}
	def, ok := definition.DocumentOK()
	if !ok {
		record.invalid("definition_v2", "raw.type_mismatch")
		return record
	}
	record.DefinitionProjectionAudit = mongomodelcatalog.AuditDefinitionProjections(def)
	if record.InvalidCount == 0 && !record.HashError {
		var po mongomodelcatalog.DefinitionPO
		if err := bson.Unmarshal(def, &po); err != nil {
			record.invalid("definition_v2", "raw.decode_error")
		} else {
			// This mapper only decodes the canonical Definition; routing is derived
			// by the existing pure domain rule, independently of stored layers.
			mapped := mongomodelcatalog.NewMapper().ToPublished(&mongomodelcatalog.PublishedAssessmentModelPO{DefinitionV2: &po})
			model := &domain.AssessmentModel{Kind: domain.Kind(metadata.Kind), DefinitionV2: mapped.DefinitionV2}
			decision, err := model.DecisionKindForDefinition()
			if err != nil || (record.Role == "published_snapshot" && metadata.DecisionKind != string(decision)) {
				record.invalid("metadata.decision_kind", "metadata.decision_kind_invalid")
			}
			policy := domain.ClassifyAlgorithmWritePolicy(domain.Kind(metadata.Kind), domain.Algorithm(metadata.Algorithm))
			if policy == domain.AlgorithmWriteUnknown || (record.Role == "published_snapshot" && !domain.IsCanonicalPublishAlgorithm(domain.Kind(metadata.Kind), domain.Algorithm(metadata.Algorithm))) {
				record.invalid("metadata.algorithm", "metadata.algorithm_invalid")
			}
		}
	}
	switch {
	case record.InvalidCount > 0:
		record.Status = "invalid"
	case record.HashError:
		record.Status = "error"
	case record.DifferenceCount > 0:
		record.Status = "mismatched"
	default:
		record.Status = "matched"
	}
	return record
}

// Published hashes are frozen metadata, not a request to repair or rehash a
// record. Heads do not carry this snapshot-only Source contract.
func auditPublishedDefinitionHash(raw bson.Raw, record *projectionRecord) {
	record.HashMetadataStatus = "invalid"
	source, ok := raw.Lookup("source").DocumentOK()
	if !ok {
		record.hashMetadataIssue("source", "metadata.definition_hash_source_invalid")
		return
	}
	elements, err := source.Elements()
	if err != nil {
		record.hashMetadataIssue("source", "raw.invalid_bson")
		return
	}
	seen := make(map[string]bool, len(elements))
	for _, element := range elements {
		if seen[element.Key()] {
			record.hashMetadataIssue("source.duplicate_field", "raw.duplicate_key")
			return
		}
		seen[element.Key()] = true
	}
	schema, schemaOK := source.Lookup(modelcatalogport.SourceDefinitionHashSchema).StringValueOK()
	if !schemaOK || schema != "definition-v2/v1" {
		record.hashMetadataIssue("source.definition_hash_schema", "metadata.definition_hash_schema_invalid")
	}
	hash, hashOK := source.Lookup(modelcatalogport.SourceDefinitionContentHash).StringValueOK()
	decoded, decodeErr := hex.DecodeString(hash)
	if !hashOK || decodeErr != nil || len(decoded) != sha256.Size || strings.ToLower(hash) != hash {
		record.hashMetadataIssue("source.definition_content_hash", "metadata.definition_content_hash_invalid")
	} else if record.CanonicalHash != "" && hash != record.CanonicalHash {
		record.hashMetadataIssue("source.definition_content_hash", "metadata.definition_content_hash_mismatch")
	}
	if len(record.HashMetadataReasonCounts) == 0 {
		if record.CanonicalHash == "" || record.HashError {
			record.HashMetadataStatus = "unverifiable"
			record.hashMetadataIssue("source.definition_content_hash", "metadata.definition_hash_canonical_unavailable")
		} else {
			record.HashMetadataStatus = "matched"
		}
	}
}

func (r *projectionRecord) hashMetadataIssue(path, reason string) {
	if r.HashMetadataReasonCounts == nil {
		r.HashMetadataReasonCounts = make(map[string]int)
	}
	r.HashMetadataReasonCounts[reason]++
	r.HashMetadataPaths = append(r.HashMetadataPaths, path)
}

// All callers provide fixed diagnostic enums; no metadata value becomes a label.
func (r *projectionRecord) invalid(path, reason string) {
	r.InvalidCount++
	r.InvalidPaths = append(r.InvalidPaths, path)
	if r.InvalidReasonCounts == nil {
		r.InvalidReasonCounts = make(map[string]int)
	}
	r.InvalidReasonCounts[reason]++
}
