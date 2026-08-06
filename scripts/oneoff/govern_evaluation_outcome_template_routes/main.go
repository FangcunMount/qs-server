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

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

const (
	governanceSchemaVersion = "evaluation-outcome-template-route-governance/v1"
	targetLegacyVersion     = "legacy-v1"
	targetCurrentVersion    = "2026-08-v1"
	applyConfirmation       = "materialize-outcome-template-route-legacy-v1"
	rollbackConfirmation    = "rollback-outcome-template-route-legacy-v1"
	transactionBatchSize    = 200
)

var knownTemplateIDs = map[string]struct{}{
	"standard": {}, "mbti": {}, "sbti": {}, "bigfive": {}, "enneagram": {},
}

type config struct {
	Operation       string
	ManifestPath    string
	AfterID         uint64
	MaxRecords      int
	RequireComplete bool
	Confirm         string
	MySQL           mysqldriver.Config
}

type manifest struct {
	SchemaVersion      string   `json:"schema_version"`
	Database           string   `json:"database"`
	Table              string   `json:"table"`
	GeneratedAt        string   `json:"generated_at"`
	TargetLegacy       string   `json:"target_legacy_version"`
	Records            []record `json:"records"`
	RecordsFingerprint string   `json:"records_fingerprint"`
}

type record struct {
	OutcomeID          uint64         `json:"outcome_id"`
	ModelKind          string         `json:"model_kind"`
	ModelCode          string         `json:"model_code"`
	ModelVersion       string         `json:"model_version"`
	EvaluatedAt        string         `json:"evaluated_at"`
	TemplateID         string         `json:"template_id"`
	TemplateVersion    string         `json:"template_version"`
	SourceRawHash      string         `json:"source_raw_hash"`
	SourceSemanticHash string         `json:"source_semantic_hash"`
	TargetSemanticHash string         `json:"target_semantic_hash"`
	Sections           []sectionDelta `json:"sections"`
	TypologyRouting    *routeDelta    `json:"typology_routing,omitempty"`
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
}

type inventory struct {
	Total      int64 `json:"total"`
	Explicit   int64 `json:"explicit"`
	Candidates int64 `json:"candidates"`
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
	defer db.Close()

	switch cfg.Operation {
	case "audit":
		return audit(ctx, db, cfg, stdout)
	case "apply", "rollback", "verify":
		governance, err := readManifest(cfg.ManifestPath)
		if err != nil {
			return err
		}
		if err := validateManifest(governance, cfg.MySQL.DBName); err != nil {
			return err
		}
		records := selectedRecords(governance.Records, cfg)
		if cfg.Operation == "verify" {
			return verify(ctx, db, records, cfg.RequireComplete, stdout)
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
		db.Close()
		return nil, fmt.Errorf("ping MySQL: %w", err)
	}
	return db, nil
}

func audit(ctx context.Context, db *sql.DB, cfg config, stdout io.Writer) error {
	generatedAt := time.Now().UTC()
	records := make([]record, 0)
	state, err := scanOutcomes(ctx, db, func(row outcomeRow, plan reportPlan) error {
		if !plan.Changed {
			return nil
		}
		records = append(records, record{
			OutcomeID: row.ID, ModelKind: row.ModelKind, ModelCode: row.ModelCode, ModelVersion: row.ModelVersion,
			EvaluatedAt: row.EvaluatedAt.UTC().Format(time.RFC3339Nano), TemplateID: plan.TemplateID, TemplateVersion: plan.TemplateVersion,
			SourceRawHash: hashString(row.ReportInput), SourceSemanticHash: plan.SourceSemanticHash,
			TargetSemanticHash: plan.TargetSemanticHash, Sections: plan.Sections, TypologyRouting: plan.TypologyRouting,
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
		Records: records, RecordsFingerprint: fingerprint,
	}
	if err := writeManifest(cfg.ManifestPath, governance); err != nil {
		return err
	}
	return writeResult(stdout, map[string]any{
		"operation": "audit", "inventory": state, "records": len(records),
		"manifest_path": cfg.ManifestPath, "records_fingerprint": fingerprint,
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

func scanOutcomes(ctx context.Context, db *sql.DB, visit func(outcomeRow, reportPlan) error) (inventory, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, model_kind, model_code, model_version, evaluated_at, COALESCE(report_input_json, '')
FROM evaluation_outcome
ORDER BY id`)
	if err != nil {
		return inventory{}, fmt.Errorf("query evaluation outcomes: %w", err)
	}
	defer rows.Close()
	state := inventory{}
	for rows.Next() {
		var row outcomeRow
		if err := rows.Scan(&row.ID, &row.ModelKind, &row.ModelCode, &row.ModelVersion, &row.EvaluatedAt, &row.ReportInput); err != nil {
			return state, fmt.Errorf("scan evaluation outcome: %w", err)
		}
		state.Total++
		plan, err := planReportInput(row.ReportInput, row.ModelKind)
		if err != nil {
			return state, fmt.Errorf("outcome %d %s@%s: %w", row.ID, row.ModelCode, row.ModelVersion, err)
		}
		if plan.Changed {
			state.Candidates++
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
		return reportPlan{}, errors.New("InterpretationAssets.ReportSpec.Sections is required")
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
	changed := false
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
		Sections: deltas, TypologyRouting: typologyDelta,
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
	defer selectStmt.Close()
	updateStmt, err := tx.PrepareContext(ctx, `UPDATE evaluation_outcome SET report_input_json = ? WHERE id = ? AND report_input_json = ?`)
	if err != nil {
		return 0, 0, err
	}
	defer updateStmt.Close()
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
			plan, err := planReportInput(raw, item.ModelKind)
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

func verify(ctx context.Context, db *sql.DB, records []record, requireComplete bool, stdout io.Writer) error {
	for _, item := range records {
		var raw string
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(report_input_json, '') FROM evaluation_outcome WHERE id = ?`, item.OutcomeID).Scan(&raw); err != nil {
			return fmt.Errorf("load outcome %d: %w", item.OutcomeID, err)
		}
		canonical, err := canonicalJSON(raw)
		if err != nil || hashBytes(canonical) != item.TargetSemanticHash {
			return fmt.Errorf("outcome %d target verification failed", item.OutcomeID)
		}
	}
	state, err := scanOutcomes(ctx, db, nil)
	if err != nil {
		return err
	}
	if requireComplete && state.Candidates != 0 {
		return fmt.Errorf("outcome template routes are incomplete: %d candidates remain", state.Candidates)
	}
	return writeResult(stdout, map[string]any{
		"operation": "verify", "verified": len(records), "inventory": state, "require_complete": requireComplete,
	})
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

func validateManifest(value manifest, database string) error {
	if value.SchemaVersion != governanceSchemaVersion || value.Database != database || value.Table != "evaluation_outcome" || value.TargetLegacy != targetLegacyVersion {
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

func writeResult(output io.Writer, value any) error {
	if _, err := io.WriteString(output, "EVALUATION_OUTCOME_TEMPLATE_ROUTE_GOVERNANCE_RESULT\n"); err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
