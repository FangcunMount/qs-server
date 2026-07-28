package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	rollbackPhasePrepared           = "prepared"
	rollbackPhaseValidated          = "validated"
	rollbackPhaseBackedUp           = "backed_up"
	rollbackPhaseMongoDeleted       = "mongo_deleted"
	rollbackPhaseMySQLDeleted       = "mysql_deleted"
	rollbackPhaseStatisticsRepaired = "statistics_repaired"
	rollbackPhaseLedgersDeleted     = "ledgers_deleted"
	rollbackPhaseCompleted          = "completed"
)

var rollbackPhaseOrder = map[string]int{
	rollbackPhasePrepared:           0,
	rollbackPhaseValidated:          1,
	rollbackPhaseBackedUp:           2,
	rollbackPhaseMongoDeleted:       3,
	rollbackPhaseMySQLDeleted:       4,
	rollbackPhaseStatisticsRepaired: 5,
	rollbackPhaseLedgersDeleted:     6,
	rollbackPhaseCompleted:          7,
}

type historicalRollbackOperation struct {
	ID           uint64
	OrgID        int64
	BatchID      string
	ManifestHash string
	ScopeHash    string
	BackupSuffix string
	Phase        string
	Status       string
	LastError    sql.NullString
}

type historicalRollbackResource struct {
	Storage      string     `json:"storage"`
	ResourceType string     `json:"resource_type"`
	ResourceID   string     `json:"resource_id"`
	BusinessDate *time.Time `json:"business_date,omitempty"`
}

type historicalDryRunReceipt struct {
	Version      int       `json:"version"`
	OperationID  uint64    `json:"operation_id"`
	BatchID      string    `json:"batch_id"`
	OrgID        int64     `json:"org_id"`
	ScopeHash    string    `json:"scope_hash"`
	ManifestHash string    `json:"manifest_hash"`
	GeneratedAt  time.Time `json:"generated_at"`
}

type mysqlRollbackResourceSpec struct {
	resourceType string
	query        string
}

type historicalManifestScope struct {
	TesteeIDs      []uint64
	AssessmentIDs  []uint64
	AnswerSheetIDs []uint64
	OutcomeIDs     []uint64
	ReportIDs      []uint64
	GenerationIDs  []uint64
	ReportRunIDs   []uint64
	EnrollmentIDs  []uint64
	TaskIDs        []uint64
}

func loadHistoricalManifestScope(path, batchID string) (historicalManifestScope, error) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return historicalManifestScope{}, err
	}
	var manifest struct {
		BatchID   string `json:"batch_id"`
		Scenarios map[string]struct {
			TesteeID       string   `json:"testee_id"`
			TesteeCreated  bool     `json:"testee_created"`
			EnrollmentID   string   `json:"enrollment_id"`
			TaskIDs        []string `json:"task_ids"`
			AnswerSheetID  string   `json:"answersheet_id"`
			AnswerSheetIDs []string `json:"answersheet_ids"`
			AssessmentID   string   `json:"assessment_id"`
			AssessmentIDs  []string `json:"assessment_ids"`
			OutcomeID      string   `json:"outcome_id"`
			OutcomeIDs     []string `json:"outcome_ids"`
			ReportID       string   `json:"report_id"`
			ReportIDs      []string `json:"report_ids"`
			GenerationID   string   `json:"report_generation_id"`
			ReportRunID    string   `json:"report_run_id"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return historicalManifestScope{}, err
	}
	if strings.TrimSpace(manifest.BatchID) != batchID {
		return historicalManifestScope{}, fmt.Errorf("manifest batch_id %q does not match %q", manifest.BatchID, batchID)
	}
	var scope historicalManifestScope
	add := func(field, scenario string, destination *[]uint64, values ...string) error {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			id, err := strconv.ParseUint(value, 10, 64)
			if err != nil || id == 0 {
				return fmt.Errorf("scenario %s has invalid %s %q", scenario, field, value)
			}
			*destination = append(*destination, id)
		}
		return nil
	}
	for scenarioID, scenario := range manifest.Scenarios {
		if scenario.TesteeCreated {
			if err := add("testee_id", scenarioID, &scope.TesteeIDs, scenario.TesteeID); err != nil {
				return historicalManifestScope{}, err
			}
		}
		fields := []struct {
			name   string
			dest   *[]uint64
			values []string
		}{
			{"enrollment_id", &scope.EnrollmentIDs, []string{scenario.EnrollmentID}},
			{"task_ids", &scope.TaskIDs, scenario.TaskIDs},
			{"answersheet_ids", &scope.AnswerSheetIDs, append([]string{scenario.AnswerSheetID}, scenario.AnswerSheetIDs...)},
			{"assessment_ids", &scope.AssessmentIDs, append([]string{scenario.AssessmentID}, scenario.AssessmentIDs...)},
			{"outcome_ids", &scope.OutcomeIDs, append([]string{scenario.OutcomeID}, scenario.OutcomeIDs...)},
			{"report_ids", &scope.ReportIDs, append([]string{scenario.ReportID}, scenario.ReportIDs...)},
			{"report_generation_id", &scope.GenerationIDs, []string{scenario.GenerationID}},
			{"report_run_id", &scope.ReportRunIDs, []string{scenario.ReportRunID}},
		}
		for _, field := range fields {
			if err := add(field.name, scenarioID, field.dest, field.values...); err != nil {
				return historicalManifestScope{}, err
			}
		}
	}
	scope.TesteeIDs = uniqueUint64(scope.TesteeIDs)
	scope.AssessmentIDs = uniqueUint64(scope.AssessmentIDs)
	scope.AnswerSheetIDs = uniqueUint64(scope.AnswerSheetIDs)
	scope.OutcomeIDs = uniqueUint64(scope.OutcomeIDs)
	scope.ReportIDs = uniqueUint64(scope.ReportIDs)
	scope.GenerationIDs = uniqueUint64(scope.GenerationIDs)
	scope.ReportRunIDs = uniqueUint64(scope.ReportRunIDs)
	scope.EnrollmentIDs = uniqueUint64(scope.EnrollmentIDs)
	scope.TaskIDs = uniqueUint64(scope.TaskIDs)
	return scope, nil
}

func historicalMySQLResourceSpecs() []mysqlRollbackResourceSpec {
	return []mysqlRollbackResourceSpec{
		{"testee", `SELECT CAST(t.id AS CHAR), NULL FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id=t.id`},
		{"assessment", `SELECT CAST(a.id AS CHAR), NULL FROM assessment a JOIN tmp_cleanup_assessment_ids x ON x.id=a.id`},
		{"evaluation_outcome", `SELECT CAST(o.id AS CHAR), NULL FROM evaluation_outcome o JOIN tmp_cleanup_outcome_ids x ON x.id=o.id`},
		{"assessment_score", `SELECT CAST(s.id AS CHAR), NULL FROM assessment_score s JOIN tmp_cleanup_assessment_score_ids x ON x.id=s.id`},
		{"assessment_task", `SELECT CAST(t.id AS CHAR), NULL FROM assessment_task t JOIN tmp_cleanup_assessment_task_ids x ON x.id=t.id`},
		{"plan_enrollment", `SELECT CAST(e.id AS CHAR), NULL FROM plan_enrollment e JOIN tmp_cleanup_plan_enrollment_ids x ON x.id=e.id`},
		{"clinician_relation", `SELECT CAST(r.id AS CHAR), NULL FROM clinician_relation r JOIN tmp_cleanup_relation_ids x ON x.id=r.id`},
		{"assessment_entry_resolve_log", `SELECT CAST(l.id AS CHAR), NULL FROM assessment_entry_resolve_log l JOIN tmp_cleanup_resolve_log_ids x ON x.id=l.id`},
		{"assessment_entry_intake_log", `SELECT CAST(l.id AS CHAR), NULL FROM assessment_entry_intake_log l JOIN tmp_cleanup_intake_log_ids x ON x.id=l.id`},
		{"statistics_access_fact", `SELECT CAST(f.id AS CHAR), f.stat_date FROM statistics_access_fact f LEFT JOIN tmp_cleanup_testee_ids t ON t.id=f.testee_id LEFT JOIN tmp_cleanup_resolve_log_ids r ON f.source_type='entry_resolve' AND BINARY f.source_ref=BINARY CAST(r.id AS CHAR) LEFT JOIN tmp_cleanup_intake_log_ids i ON f.source_type='entry_intake' AND BINARY f.source_ref=BINARY CAST(i.id AS CHAR) WHERE t.id IS NOT NULL OR r.id IS NOT NULL OR i.id IS NOT NULL`},
		{"statistics_assessment_fact", `SELECT CAST(f.id AS CHAR), f.stat_date FROM statistics_assessment_fact f LEFT JOIN tmp_cleanup_testee_ids t ON t.id=f.testee_id LEFT JOIN tmp_cleanup_answersheet_ids s ON s.id=f.answersheet_id LEFT JOIN tmp_cleanup_assessment_ids a ON a.id=f.assessment_id LEFT JOIN tmp_cleanup_report_ids r ON r.id=f.report_id WHERE t.id IS NOT NULL OR s.id IS NOT NULL OR a.id IS NOT NULL OR r.id IS NOT NULL`},
		{"statistics_plan_fact", `SELECT CAST(f.id AS CHAR), f.stat_date FROM statistics_plan_fact f LEFT JOIN tmp_cleanup_testee_ids t ON t.id=f.testee_id LEFT JOIN tmp_cleanup_plan_enrollment_ids e ON e.id=f.enrollment_id LEFT JOIN tmp_cleanup_assessment_task_ids task ON task.id=f.task_id WHERE t.id IS NOT NULL OR e.id IS NOT NULL OR task.id IS NOT NULL`},
		{"statistics_access_daily", `SELECT CAST(d.id AS CHAR), d.stat_date FROM statistics_access_daily d JOIN tmp_cleanup_statistics_dates x ON x.org_id=d.org_id AND x.stat_date=d.stat_date`},
		{"statistics_assessment_daily", `SELECT CAST(d.id AS CHAR), d.stat_date FROM statistics_assessment_daily d JOIN tmp_cleanup_statistics_dates x ON x.org_id=d.org_id AND x.stat_date=d.stat_date`},
		{"statistics_plan_activity_daily", `SELECT CAST(d.id AS CHAR), d.stat_date FROM statistics_plan_activity_daily d JOIN tmp_cleanup_statistics_dates x ON x.org_id=d.org_id AND x.stat_date=d.stat_date`},
		{"statistics_plan_fulfillment_daily", `SELECT CAST(d.id AS CHAR), d.cohort_date FROM statistics_plan_fulfillment_daily d JOIN tmp_cleanup_statistics_dates x ON x.org_id=d.org_id AND x.stat_date=d.cohort_date`},
		{"statistics_org_snapshot", `SELECT CAST(s.org_id AS CHAR), MAX(x.stat_date) FROM statistics_org_snapshot s JOIN tmp_cleanup_statistics_dates x ON x.org_id=s.org_id GROUP BY s.org_id`},
		{"domain_event_outbox", `SELECT CAST(o.id AS CHAR), NULL FROM domain_event_outbox o JOIN tmp_cleanup_mysql_outbox_ids x ON x.id=o.id`},
		{"runtime_checkpoint", `SELECT CAST(r.id AS CHAR), NULL FROM runtime_checkpoint r JOIN tmp_cleanup_assessment_ids a ON a.id=r.assessment_id`},
		{"retry_event_hold", `SELECT CAST(h.id AS CHAR), NULL FROM retry_event_hold h JOIN tmp_cleanup_event_ids e ON BINARY e.event_id=BINARY h.event_id`},
		{"seed_backfill_stage_attempt", `SELECT CAST(a.id AS CHAR), DATE(a.business_at) FROM seed_backfill_stage_attempt a JOIN tmp_cleanup_seed_attempt_ids x ON x.id=a.id`},
		{"seed_backfill_stage", `SELECT CAST(s.id AS CHAR), DATE(s.business_at) FROM seed_backfill_stage s JOIN tmp_cleanup_seed_stage_ids x ON x.id=s.id`},
	}
}

func runHistoricalRollback(ctx context.Context, conn *sql.Conn, mongoDB *mongo.Database, cfg config) error {
	if err := validateBackupSuffix(cfg.backupSuffix); err != nil {
		return fmt.Errorf("invalid backup suffix: %w", err)
	}
	manifestHash, err := hashFile(cfg.seedManifest)
	if err != nil {
		return fmt.Errorf("hash seed manifest: %w", err)
	}
	if !cfg.apply {
		return prepareHistoricalRollback(ctx, conn, mongoDB, cfg, manifestHash)
	}
	receipt, err := readHistoricalDryRunReceipt(cfg.dryRunReceipt)
	if err != nil {
		return err
	}
	if receipt.Version != 2 {
		return fmt.Errorf("dry-run receipt v%d cannot be applied; run historical dry-run again to create receipt v2", receipt.Version)
	}
	op, err := loadHistoricalRollbackOperation(ctx, conn, receipt.OperationID)
	if err != nil {
		return err
	}
	if err := validateHistoricalReceipt(receipt, op, cfg.seedBatchID, manifestHash); err != nil {
		return err
	}
	if op.Phase == rollbackPhaseCompleted && op.Status == "completed" {
		return nil
	}
	if rollbackPhaseOrder[op.Phase] < rollbackPhaseOrder[rollbackPhaseBackedUp] {
		resources, orgID, err := buildHistoricalRollbackScope(ctx, conn, mongoDB, cfg)
		if err != nil {
			return err
		}
		if orgID != op.OrgID {
			return fmt.Errorf("rollback org drift: operation=%d current=%d", op.OrgID, orgID)
		}
		currentHash, err := historicalResourceScopeHash(cfg.seedBatchID, manifestHash, resources)
		if err != nil {
			return err
		}
		if currentHash != op.ScopeHash {
			return fmt.Errorf("rollback resource scope drift: operation=%s current=%s; run dry-run again", op.ScopeHash, currentHash)
		}
		if err := validateHistoricalMongoForeignReferences(ctx, mongoDB, currentHistoricalTesteeIDs(resources), resources); err != nil {
			return err
		}
	}

	if err := runHistoricalRollbackPhase(ctx, conn, &op, rollbackPhaseBackedUp, func() error {
		if err := backupHistoricalMySQLResources(ctx, conn, op.ID, op.BackupSuffix); err != nil {
			return err
		}
		return backupHistoricalMongoResources(ctx, conn, mongoDB, op.ID, op.BackupSuffix)
	}); err != nil {
		return err
	}
	if err := runHistoricalRollbackPhase(ctx, conn, &op, rollbackPhaseMongoDeleted, func() error {
		return deleteHistoricalMongoResources(ctx, conn, mongoDB, op.ID)
	}); err != nil {
		return err
	}
	if err := runHistoricalRollbackPhase(ctx, conn, &op, rollbackPhaseMySQLDeleted, func() error {
		return deleteHistoricalMySQLBusinessResources(ctx, conn, op.ID)
	}); err != nil {
		return err
	}
	if err := runHistoricalRollbackPhase(ctx, conn, &op, rollbackPhaseStatisticsRepaired, func() error {
		if err := prepareHistoricalStatisticsDates(ctx, conn, op.ID, op.OrgID); err != nil {
			return err
		}
		return repairHistoricalRollbackStatistics(ctx, conn, cfg)
	}); err != nil {
		return err
	}
	if err := runHistoricalRollbackPhase(ctx, conn, &op, rollbackPhaseLedgersDeleted, func() error {
		return deleteHistoricalLedgerResources(ctx, conn, op.ID)
	}); err != nil {
		return err
	}
	return runHistoricalRollbackPhase(ctx, conn, &op, rollbackPhaseCompleted, func() error { return nil })
}

func prepareHistoricalRollback(ctx context.Context, conn *sql.Conn, mongoDB *mongo.Database, cfg config, manifestHash string) error {
	resources, orgID, err := buildHistoricalRollbackScope(ctx, conn, mongoDB, cfg)
	if err != nil {
		return err
	}
	if err := validateHistoricalMongoForeignReferences(ctx, mongoDB, currentHistoricalTesteeIDs(resources), resources); err != nil {
		return err
	}
	scopeHash, err := historicalResourceScopeHash(cfg.seedBatchID, manifestHash, resources)
	if err != nil {
		return err
	}
	op, err := persistHistoricalRollbackOperation(ctx, conn, historicalRollbackOperation{
		OrgID: orgID, BatchID: cfg.seedBatchID, ManifestHash: manifestHash, ScopeHash: scopeHash,
		BackupSuffix: cfg.backupSuffix, Phase: rollbackPhasePrepared, Status: "running",
	}, resources)
	if err != nil {
		return err
	}
	if err := runHistoricalRollbackPhase(ctx, conn, &op, rollbackPhaseValidated, func() error { return nil }); err != nil {
		return err
	}
	receipt := historicalDryRunReceipt{
		Version: 2, OperationID: op.ID, BatchID: op.BatchID, OrgID: op.OrgID,
		ScopeHash: op.ScopeHash, ManifestHash: op.ManifestHash, GeneratedAt: time.Now().UTC(),
	}
	if err := writeHistoricalDryRunReceipt(cfg.dryRunReceipt, receipt); err != nil {
		return err
	}
	counts := map[string]int{}
	for _, resource := range resources {
		counts[resource.Storage+"/"+resource.ResourceType]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("historical_scope %s=%d\n", key, counts[key])
	}
	fmt.Printf("historical rollback dry-run prepared: operation_id=%d org=%d resources=%d scope_hash=%s\n", op.ID, op.OrgID, len(resources), op.ScopeHash)
	return nil
}

func buildHistoricalRollbackScope(ctx context.Context, conn *sql.Conn, mongoDB *mongo.Database, cfg config) ([]historicalRollbackResource, int64, error) {
	ledgerIDs, err := loadHistoricalBatchTesteeIDs(ctx, conn, cfg.seedBatchID)
	if err != nil {
		return nil, 0, fmt.Errorf("load historical batch testee ids: %w", err)
	}
	manifestIDs, err := loadHistoricalManifestTesteeIDs(cfg.seedManifest, cfg.seedBatchID)
	if err != nil {
		return nil, 0, fmt.Errorf("load historical manifest testee ids: %w", err)
	}
	testeeIDs := uniqueUint64(append(ledgerIDs, manifestIDs...))
	if len(testeeIDs) == 0 {
		return nil, 0, fmt.Errorf("historical batch %s resolved zero testee ids", cfg.seedBatchID)
	}
	if err := prepareMySQLScope(ctx, conn, cfg, testeeIDs); err != nil {
		return nil, 0, fmt.Errorf("prepare mysql scope: %w", err)
	}
	ids, err := loadScopeIDs(ctx, conn, cfg)
	if err != nil {
		return nil, 0, err
	}
	ids, err = enrichScopeIDsFromMongo(ctx, mongoDB, ids, cfg.workers, true)
	if err != nil {
		return nil, 0, err
	}
	if err := storeScopeIDs(ctx, conn, ids); err != nil {
		return nil, 0, err
	}
	if err := addMySQLOutboxIDsToScope(ctx, conn, cfg); err != nil {
		return nil, 0, err
	}
	if !cfg.skipMongoOutboxEventScope {
		if err := addMongoOutboxEventIDsToMySQLScope(ctx, conn, mongoDB, ids, cfg.workers); err != nil {
			return nil, 0, err
		}
	}
	if err := validateHistoricalForeignReferences(ctx, conn); err != nil {
		return nil, 0, err
	}
	orgID, err := historicalScopeOrgID(ctx, conn)
	if err != nil {
		return nil, 0, err
	}
	resources, err := collectHistoricalMySQLResources(ctx, conn)
	if err != nil {
		return nil, 0, err
	}
	mongoResources, err := collectHistoricalMongoResources(ctx, mongoDB, mongoScopeIDs(ids, cfg))
	if err != nil {
		return nil, 0, err
	}
	return normalizeHistoricalRollbackResources(append(resources, mongoResources...)), orgID, nil
}

func historicalScopeOrgID(ctx context.Context, conn *sql.Conn) (int64, error) {
	rows, err := conn.QueryContext(ctx, `SELECT DISTINCT t.org_id FROM testee t JOIN tmp_cleanup_testee_ids x ON x.id=t.id ORDER BY t.org_id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var orgs []int64
	for rows.Next() {
		var org int64
		if err := rows.Scan(&org); err != nil {
			return 0, err
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(orgs) != 1 {
		return 0, fmt.Errorf("historical rollback requires exactly one org, got %v", orgs)
	}
	return orgs[0], nil
}

func collectHistoricalMySQLResources(ctx context.Context, conn *sql.Conn) ([]historicalRollbackResource, error) {
	var resources []historicalRollbackResource
	for _, spec := range historicalMySQLResourceSpecs() {
		rows, err := conn.QueryContext(ctx, spec.query)
		if err != nil {
			return nil, fmt.Errorf("collect mysql %s: %w", spec.resourceType, err)
		}
		for rows.Next() {
			var id string
			var businessDate sql.NullTime
			if err := rows.Scan(&id, &businessDate); err != nil {
				_ = rows.Close()
				return nil, err
			}
			resource := historicalRollbackResource{Storage: "mysql", ResourceType: spec.resourceType, ResourceID: id}
			if businessDate.Valid {
				date := businessDate.Time
				resource.BusinessDate = &date
			}
			resources = append(resources, resource)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return resources, nil
}

func collectHistoricalMongoResources(ctx context.Context, db *mongo.Database, ids scopeIDs) ([]historicalRollbackResource, error) {
	var resources []historicalRollbackResource
	for _, item := range mongoCollectionScopes(ids) {
		seen := map[string]struct{}{}
		for _, filter := range item.filters {
			cur, err := db.Collection(item.coll).Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
			if err != nil {
				return nil, fmt.Errorf("collect mongo %s: %w", item.coll, err)
			}
			for cur.Next(ctx) {
				var row bson.M
				if err := cur.Decode(&row); err != nil {
					_ = cur.Close(ctx)
					return nil, err
				}
				key := mongoDocumentIDKey(row["_id"])
				if key == "" {
					continue
				}
				seen[key] = struct{}{}
			}
			if err := cur.Close(ctx); err != nil {
				return nil, err
			}
		}
		for key := range seen {
			resources = append(resources, historicalRollbackResource{Storage: "mongo", ResourceType: item.coll, ResourceID: key})
		}
	}
	return resources, nil
}

func normalizeHistoricalRollbackResources(resources []historicalRollbackResource) []historicalRollbackResource {
	byKey := make(map[string]historicalRollbackResource, len(resources))
	for _, resource := range resources {
		key := resource.Storage + "\x00" + resource.ResourceType + "\x00" + resource.ResourceID
		byKey[key] = resource
	}
	out := make([]historicalRollbackResource, 0, len(byKey))
	for _, resource := range byKey {
		out = append(out, resource)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Storage != out[j].Storage {
			return out[i].Storage < out[j].Storage
		}
		if out[i].ResourceType != out[j].ResourceType {
			return out[i].ResourceType < out[j].ResourceType
		}
		return out[i].ResourceID < out[j].ResourceID
	})
	return out
}

func historicalResourceScopeHash(batchID, manifestHash string, resources []historicalRollbackResource) (string, error) {
	resources = normalizeHistoricalRollbackResources(resources)
	payload, err := json.Marshal(struct {
		BatchID      string                       `json:"batch_id"`
		ManifestHash string                       `json:"manifest_hash"`
		Resources    []historicalRollbackResource `json:"resources"`
	}{strings.TrimSpace(batchID), manifestHash, resources})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func persistHistoricalRollbackOperation(ctx context.Context, conn *sql.Conn, wanted historicalRollbackOperation, resources []historicalRollbackResource) (historicalRollbackOperation, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return historicalRollbackOperation{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO seed_backfill_rollback_operation
(org_id,batch_id,manifest_hash,scope_hash,backup_suffix,phase,status,started_at)
VALUES (?,?,?,?,?,?,?,UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`, wanted.OrgID, wanted.BatchID, wanted.ManifestHash, wanted.ScopeHash, wanted.BackupSuffix, wanted.Phase, wanted.Status)
	if err != nil {
		return historicalRollbackOperation{}, err
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		return historicalRollbackOperation{}, fmt.Errorf("resolve rollback operation id: id=%d err=%w", id, err)
	}
	op, err := loadHistoricalRollbackOperationQuery(ctx, tx, uint64(id))
	if err != nil {
		return historicalRollbackOperation{}, err
	}
	if op.ManifestHash != wanted.ManifestHash || op.ScopeHash != wanted.ScopeHash {
		return historicalRollbackOperation{}, fmt.Errorf("existing rollback operation %d differs from dry-run inputs", op.ID)
	}
	for _, resource := range resources {
		var date any
		if resource.BusinessDate != nil {
			date = resource.BusinessDate.Format("2006-01-02")
		}
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO seed_backfill_rollback_resource
(operation_id,storage,resource_type,resource_id,business_date) VALUES (?,?,?,?,?)`, op.ID, resource.Storage, resource.ResourceType, resource.ResourceID, date); err != nil {
			return historicalRollbackOperation{}, err
		}
	}
	var persisted int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM seed_backfill_rollback_resource WHERE operation_id=?`, op.ID).Scan(&persisted); err != nil {
		return historicalRollbackOperation{}, err
	}
	if persisted != len(resources) {
		return historicalRollbackOperation{}, fmt.Errorf("rollback resource ledger count=%d want=%d", persisted, len(resources))
	}
	if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO seed_backfill_rollback_phase_attempt
(operation_id,phase,attempt_no,status,started_at,finished_at)
VALUES (?,'prepared',1,'completed',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, op.ID); err != nil {
		return historicalRollbackOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return historicalRollbackOperation{}, err
	}
	return op, nil
}

type sqlQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadHistoricalRollbackOperationQuery(ctx context.Context, queryer sqlQueryer, id uint64) (historicalRollbackOperation, error) {
	var op historicalRollbackOperation
	err := queryer.QueryRowContext(ctx, `SELECT id,org_id,batch_id,manifest_hash,scope_hash,backup_suffix,phase,status,last_error
FROM seed_backfill_rollback_operation WHERE id=?`, id).Scan(&op.ID, &op.OrgID, &op.BatchID, &op.ManifestHash, &op.ScopeHash, &op.BackupSuffix, &op.Phase, &op.Status, &op.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return op, fmt.Errorf("rollback operation %d does not exist", id)
	}
	return op, err
}

func loadHistoricalRollbackOperation(ctx context.Context, conn *sql.Conn, id uint64) (historicalRollbackOperation, error) {
	return loadHistoricalRollbackOperationQuery(ctx, conn, id)
}

func runHistoricalRollbackPhase(ctx context.Context, conn *sql.Conn, op *historicalRollbackOperation, target string, action func() error) error {
	currentRank, ok := rollbackPhaseOrder[op.Phase]
	if !ok {
		return fmt.Errorf("rollback operation %d has unknown phase %q", op.ID, op.Phase)
	}
	targetRank, ok := rollbackPhaseOrder[target]
	if !ok {
		return fmt.Errorf("unknown rollback phase %q", target)
	}
	if currentRank >= targetRank {
		return nil
	}
	if targetRank != currentRank+1 {
		return fmt.Errorf("rollback phase transition %s -> %s is not monotonic", op.Phase, target)
	}
	var attemptNo uint64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt_no),0)+1 FROM seed_backfill_rollback_phase_attempt WHERE operation_id=? AND phase=?`, op.ID, target).Scan(&attemptNo); err != nil {
		return err
	}
	result, err := conn.ExecContext(ctx, `INSERT INTO seed_backfill_rollback_phase_attempt
(operation_id,phase,attempt_no,status,started_at) VALUES (?,?,?,'running',UTC_TIMESTAMP(6))`, op.ID, target, attemptNo)
	if err != nil {
		return err
	}
	attemptID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `UPDATE seed_backfill_rollback_operation SET status='running',last_error=NULL WHERE id=?`, op.ID); err != nil {
		return err
	}
	if err := action(); err != nil {
		message := truncateRollbackError(err.Error())
		_, _ = conn.ExecContext(ctx, `UPDATE seed_backfill_rollback_phase_attempt SET status='failed',error_text=?,finished_at=UTC_TIMESTAMP(6) WHERE id=?`, message, attemptID)
		_, _ = conn.ExecContext(ctx, `UPDATE seed_backfill_rollback_operation SET status='failed',last_error=? WHERE id=?`, message, op.ID)
		return fmt.Errorf("rollback phase %s failed: %w", target, err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE seed_backfill_rollback_phase_attempt SET status='completed',finished_at=UTC_TIMESTAMP(6) WHERE id=? AND status='running'`, attemptID); err != nil {
		return err
	}
	status := "running"
	completedAt := "NULL"
	if target == rollbackPhaseCompleted {
		status = "completed"
		completedAt = "UTC_TIMESTAMP(6)"
	}
	result, err = tx.ExecContext(ctx, `UPDATE seed_backfill_rollback_operation SET phase=?,status=?,last_error=NULL,completed_at=`+completedAt+` WHERE id=? AND phase=?`, target, status, op.ID, op.Phase)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("advance rollback phase %s -> %s affected=%d err=%v", op.Phase, target, affected, err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	op.Phase = target
	op.Status = status
	return nil
}

func truncateRollbackError(message string) string {
	const max = 2000
	if len(message) <= max {
		return message
	}
	return message[:max]
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func writeHistoricalDryRunReceipt(path string, receipt historicalDryRunReceipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func readHistoricalDryRunReceipt(path string) (historicalDryRunReceipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return historicalDryRunReceipt{}, err
	}
	var version struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return historicalDryRunReceipt{}, err
	}
	if version.Version != 2 {
		return historicalDryRunReceipt{}, fmt.Errorf("dry-run receipt v%d cannot be applied; run historical dry-run again to create receipt v2", version.Version)
	}
	var receipt historicalDryRunReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return historicalDryRunReceipt{}, err
	}
	if receipt.OperationID == 0 || receipt.BatchID == "" || receipt.OrgID == 0 || receipt.ScopeHash == "" || receipt.ManifestHash == "" || receipt.GeneratedAt.IsZero() {
		return historicalDryRunReceipt{}, fmt.Errorf("dry-run receipt v2 is incomplete; run historical dry-run again")
	}
	return receipt, nil
}

func validateHistoricalReceipt(receipt historicalDryRunReceipt, op historicalRollbackOperation, batchID, manifestHash string) error {
	if receipt.OperationID != op.ID || receipt.BatchID != batchID || receipt.BatchID != op.BatchID || receipt.OrgID != op.OrgID ||
		receipt.ScopeHash != op.ScopeHash || receipt.ManifestHash != manifestHash || receipt.ManifestHash != op.ManifestHash {
		return fmt.Errorf("dry-run receipt does not match the persisted rollback operation, manifest, or batch; run dry-run again")
	}
	return nil
}

func currentHistoricalTesteeIDs(resources []historicalRollbackResource) []uint64 {
	var ids []uint64
	for _, resource := range resources {
		if resource.Storage != "mysql" || resource.ResourceType != "testee" {
			continue
		}
		if id, err := strconv.ParseUint(resource.ResourceID, 10, 64); err == nil && id != 0 {
			ids = append(ids, id)
		}
	}
	return uniqueUint64(ids)
}

func validateHistoricalMongoForeignReferences(ctx context.Context, db *mongo.Database, testeeIDs []uint64, resources []historicalRollbackResource) error {
	if len(testeeIDs) == 0 {
		return fmt.Errorf("historical rollback has no testee resources")
	}
	owned := map[string]struct{}{}
	for _, resource := range resources {
		if resource.Storage == "mongo" {
			owned[resource.ResourceType+"\x00"+resource.ResourceID] = struct{}{}
		}
	}
	collections := []string{
		"answersheets", "answersheet_submit_idempotency", "interpret_reports", "interpret_report_artifacts",
		"report_query_catalog", "archived_reports", "interpretation_admission_failures", "interpretation_attention_projections",
	}
	var conflicts []string
	for _, collection := range collections {
		seen := map[string]struct{}{}
		for _, filter := range inUint64Filters("testee_id", testeeIDs) {
			cur, err := db.Collection(collection).Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1}))
			if err != nil {
				return fmt.Errorf("mongo foreign-reference check %s: %w", collection, err)
			}
			for cur.Next(ctx) {
				var row bson.M
				if err := cur.Decode(&row); err != nil {
					_ = cur.Close(ctx)
					return err
				}
				key := mongoDocumentIDKey(row["_id"])
				if key != "" {
					seen[key] = struct{}{}
				}
			}
			if err := cur.Close(ctx); err != nil {
				return err
			}
		}
		var foreign int
		for key := range seen {
			if _, ok := owned[collection+"\x00"+key]; !ok {
				foreign++
			}
		}
		if foreign > 0 {
			conflicts = append(conflicts, fmt.Sprintf("%s=%d", collection, foreign))
		}
	}
	if len(conflicts) != 0 {
		return fmt.Errorf("historical batch testee has non-batch Mongo references; preserve testee and abort cleanup: %s", strings.Join(conflicts, ", "))
	}
	return nil
}

func historicalMySQLResourceKeyColumn(resourceType string) string {
	if resourceType == "statistics_org_snapshot" {
		return "org_id"
	}
	return "id"
}

func historicalMySQLBusinessDeleteOrder() []string {
	return []string{
		"statistics_org_snapshot", "statistics_plan_fulfillment_daily", "statistics_plan_activity_daily", "statistics_assessment_daily", "statistics_access_daily",
		"statistics_plan_fact", "statistics_assessment_fact", "statistics_access_fact", "retry_event_hold", "runtime_checkpoint", "domain_event_outbox",
		"assessment_entry_resolve_log", "assessment_entry_intake_log", "clinician_relation", "assessment_task", "plan_enrollment", "assessment_score",
		"evaluation_outcome", "assessment", "testee",
	}
}

func backupHistoricalMySQLResources(ctx context.Context, conn *sql.Conn, operationID uint64, suffix string) error {
	types := historicalMySQLBusinessDeleteOrder()
	types = append(types, "seed_backfill_stage_attempt", "seed_backfill_stage")
	for _, resourceType := range types {
		column := historicalMySQLResourceKeyColumn(resourceType)
		backupTable := fmt.Sprintf("cleanup_bak_perf_testee_%s_%s", resourceType, suffix)
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` LIKE `%s`", backupTable, resourceType)); err != nil {
			return fmt.Errorf("create backup table %s: %w", backupTable, err)
		}
		query := fmt.Sprintf(`INSERT IGNORE INTO %s SELECT t.* FROM %s t JOIN seed_backfill_rollback_resource r
ON r.operation_id=? AND r.storage='mysql' AND r.resource_type=? AND r.resource_id=CAST(t.%s AS CHAR)`, "`"+backupTable+"`", "`"+resourceType+"`", column)
		if _, err := conn.ExecContext(ctx, query, operationID, resourceType); err != nil {
			return fmt.Errorf("backup mysql %s: %w", resourceType, err)
		}
	}
	return nil
}

func backupHistoricalMongoResources(ctx context.Context, conn *sql.Conn, db *mongo.Database, operationID uint64, suffix string) error {
	return forEachHistoricalMongoResourceType(ctx, conn, operationID, func(collection string, ids []any) error {
		backup := db.Collection("cleanup_bak_perf_testee_" + collection + "_" + suffix)
		cur, err := db.Collection(collection).Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err != nil {
			return err
		}
		defer cur.Close(ctx)
		var docs []interface{}
		for cur.Next(ctx) {
			var doc bson.M
			if err := cur.Decode(&doc); err != nil {
				return err
			}
			docs = append(docs, doc)
			if len(docs) == mongoIDChunkSize {
				if _, err := backup.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil && !mongo.IsDuplicateKeyError(err) {
					return err
				}
				docs = nil
			}
		}
		if err := cur.Err(); err != nil {
			return err
		}
		if len(docs) != 0 {
			if _, err := backup.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil && !mongo.IsDuplicateKeyError(err) {
				return err
			}
		}
		return nil
	})
}

func deleteHistoricalMongoResources(ctx context.Context, conn *sql.Conn, db *mongo.Database, operationID uint64) error {
	return forEachHistoricalMongoResourceType(ctx, conn, operationID, func(collection string, ids []any) error {
		_, err := db.Collection(collection).DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
		return err
	})
}

func forEachHistoricalMongoResourceType(ctx context.Context, conn *sql.Conn, operationID uint64, fn func(string, []any) error) error {
	rows, err := conn.QueryContext(ctx, `SELECT resource_type,resource_id FROM seed_backfill_rollback_resource WHERE operation_id=? AND storage='mongo' ORDER BY resource_type,resource_id`, operationID)
	if err != nil {
		return err
	}
	defer rows.Close()
	byCollection := map[string][]any{}
	for rows.Next() {
		var collection, raw string
		if err := rows.Scan(&collection, &raw); err != nil {
			return err
		}
		id, err := parseMongoDocumentIDKey(raw)
		if err != nil {
			return fmt.Errorf("parse mongo resource %s/%s: %w", collection, raw, err)
		}
		byCollection[collection] = append(byCollection[collection], id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	collections := make([]string, 0, len(byCollection))
	for collection := range byCollection {
		collections = append(collections, collection)
	}
	sort.Strings(collections)
	for _, collection := range collections {
		ids := byCollection[collection]
		for start := 0; start < len(ids); start += mongoIDChunkSize {
			end := start + mongoIDChunkSize
			if end > len(ids) {
				end = len(ids)
			}
			if err := fn(collection, ids[start:end]); err != nil {
				return fmt.Errorf("mongo %s: %w", collection, err)
			}
		}
	}
	return nil
}

func parseMongoDocumentIDKey(raw string) (any, error) {
	prefix, value, ok := strings.Cut(raw, ":")
	if !ok {
		return nil, fmt.Errorf("missing type prefix")
	}
	switch prefix {
	case "oid":
		return primitive.ObjectIDFromHex(value)
	case "str":
		return value, nil
	case "i32":
		parsed, err := strconv.ParseInt(value, 10, 32)
		return int32(parsed), err
	case "i64":
		parsed, err := strconv.ParseInt(value, 10, 64)
		return parsed, err
	case "i":
		parsed, err := strconv.ParseInt(value, 10, 0)
		return int(parsed), err
	default:
		return nil, fmt.Errorf("unsupported type prefix %q", prefix)
	}
}

func deleteHistoricalMySQLBusinessResources(ctx context.Context, conn *sql.Conn, operationID uint64) error {
	for _, resourceType := range historicalMySQLBusinessDeleteOrder() {
		column := historicalMySQLResourceKeyColumn(resourceType)
		query := fmt.Sprintf(`DELETE t FROM %s t JOIN seed_backfill_rollback_resource r
ON r.operation_id=? AND r.storage='mysql' AND r.resource_type=? AND r.resource_id=CAST(t.%s AS CHAR)`, "`"+resourceType+"`", column)
		if _, err := conn.ExecContext(ctx, query, operationID, resourceType); err != nil {
			return fmt.Errorf("delete mysql %s: %w", resourceType, err)
		}
	}
	return nil
}

func deleteHistoricalLedgerResources(ctx context.Context, conn *sql.Conn, operationID uint64) error {
	for _, resourceType := range []string{"seed_backfill_stage_attempt", "seed_backfill_stage"} {
		query := fmt.Sprintf(`DELETE t FROM %s t JOIN seed_backfill_rollback_resource r
ON r.operation_id=? AND r.storage='mysql' AND r.resource_type=? AND r.resource_id=CAST(t.id AS CHAR)`, "`"+resourceType+"`")
		if _, err := conn.ExecContext(ctx, query, operationID, resourceType); err != nil {
			return fmt.Errorf("delete mysql %s: %w", resourceType, err)
		}
	}
	return nil
}

func prepareHistoricalStatisticsDates(ctx context.Context, conn *sql.Conn, operationID uint64, orgID int64) error {
	if _, err := conn.ExecContext(ctx, `CREATE TEMPORARY TABLE IF NOT EXISTS tmp_cleanup_statistics_dates
(org_id BIGINT NOT NULL,stat_date DATE NOT NULL,PRIMARY KEY(org_id,stat_date))`); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, `INSERT IGNORE INTO tmp_cleanup_statistics_dates(org_id,stat_date)
SELECT ?,business_date FROM seed_backfill_rollback_resource WHERE operation_id=? AND business_date IS NOT NULL`, orgID, operationID)
	return err
}
