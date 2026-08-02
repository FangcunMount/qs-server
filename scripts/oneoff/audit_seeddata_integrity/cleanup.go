package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type applyResult struct {
	Version              int              `json:"version"`
	OrgID                int64            `json:"org_id"`
	BatchID              string           `json:"batch_id"`
	From                 string           `json:"from"`
	To                   string           `json:"to"`
	PlanHash             string           `json:"plan_hash"`
	BackupSuffix         string           `json:"backup_suffix"`
	StartedAt            time.Time        `json:"started_at"`
	CompletedAt          time.Time        `json:"completed_at"`
	Candidates           int              `json:"candidates"`
	MongoBackedUp        int64            `json:"mongo_backed_up"`
	MongoDeleted         int64            `json:"mongo_deleted"`
	StatisticsBackedUp   int64            `json:"statistics_backed_up"`
	StatisticsDeleted    int64            `json:"statistics_deleted"`
	MySQLOutboxBackedUp  int64            `json:"mysql_outbox_backed_up"`
	MySQLOutboxDeleted   int64            `json:"mysql_outbox_deleted"`
	MongoCollections     map[string]int64 `json:"mongo_collections"`
	StageLedgerPreserved bool             `json:"stage_ledger_preserved"`
	Notes                []string         `json:"notes"`
}

type cleanupIDs struct {
	Reports     []uint64
	Assessments []uint64
	Outcomes    []uint64
	Generations []uint64
	Runs        []uint64
}

type statisticsFactSource struct {
	FactType   string
	SourceType string
	SourceRef  string
}

const statisticsFactSourceJoin = `f FORCE INDEX (idx_statistics_assessment_fact_source)
JOIN tmp_seed_orphan_fact_source x
 ON f.source_type=x.source_type AND f.source_ref=x.source_ref AND f.fact_type=x.fact_type`

type collectionCleanup struct {
	Name   string
	Filter bson.M
}

type candidateStageState struct {
	ReportID     uint64
	GenerationID uint64
	RunID        uint64
}

func applyAuditReport(ctx context.Context, mysqlDB *sql.DB, mongoDB *mongo.Database, cfg config) (applyResult, error) {
	report, err := decodeAuditReport(cfg.AuditReport)
	if err != nil {
		return applyResult{}, err
	}
	if report.OrgID != cfg.OrgID || report.BatchID != cfg.BatchID || report.From != cfg.From || report.To != cfg.To {
		return applyResult{}, errors.New("audit report scope does not match org, batch, or date flags")
	}
	storage, err := loadStorageIdentity(ctx, mysqlDB, mongoDB)
	if err != nil {
		return applyResult{}, err
	}
	if storage != report.Storage {
		return applyResult{}, fmt.Errorf("audit report belongs to a different database deployment: report=%+v current=%+v", report.Storage, storage)
	}
	result := applyResult{
		Version: 1, OrgID: cfg.OrgID, BatchID: cfg.BatchID, From: cfg.From, To: cfg.To,
		PlanHash: report.PlanHash, BackupSuffix: cfg.BackupSuffix, StartedAt: time.Now().UTC(),
		Candidates: len(report.DeletionCandidates), MongoCollections: map[string]int64{}, StageLedgerPreserved: true,
	}
	if len(report.DeletionCandidates) == 0 {
		result.CompletedAt = time.Now().UTC()
		result.Notes = []string{"audit report contained no safe automatic deletion candidates"}
		return result, nil
	}
	if len(report.DeletionCandidates) > cfg.MaxCandidates {
		return applyResult{}, fmt.Errorf("audit report has %d candidates, exceeding current safety ceiling %d", len(report.DeletionCandidates), cfg.MaxCandidates)
	}

	ids, err := validateCandidates(ctx, mysqlDB, mongoDB, cfg, report.DeletionCandidates)
	if err != nil {
		return applyResult{}, err
	}
	documents, err := materializeMongoCleanup(ctx, mongoDB, ids)
	if err != nil {
		return applyResult{}, err
	}
	if err := requireFreshBackups(ctx, mysqlDB, mongoDB, cfg, documents); err != nil {
		return applyResult{}, err
	}
	backedUp, perCollection, err := backupMongoDocuments(ctx, mongoDB, cfg.BackupSuffix, documents)
	if err != nil {
		return applyResult{}, err
	}
	result.MongoBackedUp = backedUp
	for collection, count := range perCollection {
		result.MongoCollections[collection] = count
	}

	statisticsBackedUp, err := backupStatisticsFacts(ctx, mysqlDB, cfg, report.DeletionCandidates)
	if err != nil {
		return applyResult{}, err
	}
	result.StatisticsBackedUp = statisticsBackedUp
	outboxBackedUp, err := backupMySQLOutbox(ctx, mysqlDB, cfg, report.DeletionCandidates)
	if err != nil {
		return applyResult{}, err
	}
	result.MySQLOutboxBackedUp = outboxBackedUp

	deleted, err := deleteMongoDocuments(ctx, mongoDB, documents)
	if err != nil {
		return applyResult{}, err
	}
	result.MongoDeleted = deleted
	outboxDeleted, err := deleteMySQLOutbox(ctx, mysqlDB, cfg, report.DeletionCandidates)
	if err != nil {
		return applyResult{}, err
	}
	result.MySQLOutboxDeleted = outboxDeleted
	statisticsDeleted, err := deleteStatisticsFacts(ctx, mysqlDB, cfg, report.DeletionCandidates)
	if err != nil {
		return applyResult{}, err
	}
	result.StatisticsDeleted = statisticsDeleted
	result.CompletedAt = time.Now().UTC()
	result.Notes = []string{
		"seed_backfill_stage and seed_backfill_stage_attempt were intentionally preserved for audit",
		"rerun the integrity audit before Statistics repair",
		"an active batch needs an explicit stage reset plus seeddata --resume; this cleanup does not reset runner progress",
	}
	return result, nil
}

func validateCandidates(ctx context.Context, mysqlDB *sql.DB, mongoDB *mongo.Database, cfg config, candidates []orphanCandidate) (cleanupIDs, error) {
	ids, stageIDs := cleanupIDs{}, make([]uint64, 0, len(candidates))
	for _, candidate := range candidates {
		reportID, assessmentID, outcomeID, generationID, runID, err := parseCandidateIDs(candidate)
		if err != nil {
			return cleanupIDs{}, err
		}
		ids.Reports = append(ids.Reports, reportID)
		ids.Assessments = append(ids.Assessments, assessmentID)
		ids.Outcomes = append(ids.Outcomes, outcomeID)
		ids.Generations = append(ids.Generations, generationID)
		ids.Runs = append(ids.Runs, runID)
		stageIDs = append(stageIDs, candidate.StageID)
	}
	ids.Reports, ids.Assessments, ids.Outcomes = uniqueIDs(ids.Reports), uniqueIDs(ids.Assessments), uniqueIDs(ids.Outcomes)
	ids.Generations, ids.Runs = uniqueIDs(ids.Generations), uniqueIDs(ids.Runs)

	stageResources, err := loadCandidateStageResources(ctx, mysqlDB, cfg, stageIDs)
	if err != nil {
		return cleanupIDs{}, err
	}
	if len(stageResources) != len(uniqueIDs(stageIDs)) {
		return cleanupIDs{}, errors.New("one or more candidate stage rows changed or disappeared after audit")
	}
	assessments, err := loadAssessmentRows(ctx, mysqlDB, cfg.OrgID, ids.Assessments)
	if err != nil {
		return cleanupIDs{}, err
	}
	outcomes, err := loadOutcomeRows(ctx, mysqlDB, cfg.OrgID, ids.Outcomes)
	if err != nil {
		return cleanupIDs{}, err
	}
	artifacts, err := loadArtifacts(ctx, mongoDB, ids.Reports)
	if err != nil {
		return cleanupIDs{}, err
	}
	generations, err := loadGenerations(ctx, mongoDB, ids.Generations)
	if err != nil {
		return cleanupIDs{}, err
	}
	runs, err := loadRuns(ctx, mongoDB, ids.Runs, ids.Generations)
	if err != nil {
		return cleanupIDs{}, err
	}
	catalogs, err := loadCatalogs(ctx, mongoDB, ids.Assessments)
	if err != nil {
		return cleanupIDs{}, err
	}

	for _, candidate := range candidates {
		reportID, assessmentID, outcomeID, generationID, runID, err := parseCandidateIDs(candidate)
		if err != nil {
			return cleanupIDs{}, err
		}
		stageState := stageResources[candidate.StageID]
		if stageState.ReportID != reportID {
			return cleanupIDs{}, fmt.Errorf("candidate stage %d no longer owns report %d", candidate.StageID, reportID)
		}
		if stageState.GenerationID != 0 && stageState.GenerationID != generationID {
			return cleanupIDs{}, fmt.Errorf("candidate stage %d generation identity changed after audit", candidate.StageID)
		}
		if stageState.RunID != 0 && stageState.RunID != runID {
			return cleanupIDs{}, fmt.Errorf("candidate stage %d run identity changed after audit", candidate.StageID)
		}
		if assessments[assessmentID] != nil || outcomes[outcomeID] != nil {
			return cleanupIDs{}, fmt.Errorf("report %d no longer has both MySQL parents missing; refusing cleanup", reportID)
		}
		artifact := artifacts[reportID]
		if artifact == nil {
			return cleanupIDs{}, fmt.Errorf("report %d is absent from the source collection; this one-time apply cannot be resumed", reportID)
		}
		objectID, err := primitive.ObjectIDFromHex(candidate.ArtifactObject)
		if err != nil {
			return cleanupIDs{}, fmt.Errorf("candidate report %d object id: %w", reportID, err)
		}
		if artifact.ObjectID != objectID || artifact.DomainID != reportID || artifact.AssessmentID != assessmentID || artifact.OutcomeID != outcomeID || artifact.GenerationID != generationID || artifact.InterpretationRunID != runID || artifact.OrgID != cfg.OrgID {
			return cleanupIDs{}, fmt.Errorf("report %d identity changed after audit", reportID)
		}
		if generation := generations[generationID]; generation != nil && (generation.OutcomeID != outcomeID || (generation.ReportID != 0 && generation.ReportID != reportID)) {
			return cleanupIDs{}, fmt.Errorf("report %d generation points outside the audited chain", reportID)
		}
		if run := selectRun(artifact, runs); run != nil && run.Generation != generationID {
			return cleanupIDs{}, fmt.Errorf("report %d run points outside the audited chain", reportID)
		}
		if catalog := selectCatalog(artifact, catalogs); catalog != nil && (catalog.AssessmentID != assessmentID || catalog.OutcomeID != outcomeID || catalog.SourceKind != "artifact" || catalog.SourceID != reportID) {
			return cleanupIDs{}, fmt.Errorf("report %d catalog points outside the audited chain", reportID)
		}
	}
	return ids, nil
}

func parseCandidateIDs(candidate orphanCandidate) (uint64, uint64, uint64, uint64, uint64, error) {
	values := []struct {
		name, raw string
	}{
		{"report_id", candidate.ReportID}, {"assessment_id", candidate.AssessmentID}, {"outcome_id", candidate.OutcomeID},
		{"generation_id", candidate.GenerationID}, {"run_id", candidate.RunID},
	}
	parsed := make([]uint64, len(values))
	for index, value := range values {
		if value.name == "run_id" && strings.TrimSpace(value.raw) == "" {
			continue
		}
		id, err := strconv.ParseUint(value.raw, 10, 64)
		if err != nil || id == 0 {
			return 0, 0, 0, 0, 0, fmt.Errorf("candidate %s %q is invalid", value.name, value.raw)
		}
		parsed[index] = id
	}
	return parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], nil
}

func loadCandidateStageResources(ctx context.Context, db *sql.DB, cfg config, stageIDs []uint64) (map[uint64]candidateStageState, error) {
	result := make(map[uint64]candidateStageState)
	stageIDs = uniqueIDs(stageIDs)
	if len(stageIDs) == 0 {
		return result, nil
	}
	from, to, err := cfg.businessWindow()
	if err != nil {
		return nil, err
	}
	args := make([]any, 0, len(stageIDs)+4)
	args = append(args, cfg.OrgID, cfg.BatchID, from, to)
	for _, id := range stageIDs {
		args = append(args, id)
	}
	query := `SELECT id,CAST(resource_id AS UNSIGNED),payload_json FROM seed_backfill_stage
WHERE org_id=? AND batch_id=? AND status='completed' AND business_at>=? AND business_at<? AND stage='report_generated' AND id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(stageIDs)), ",") + `)`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var stageID, resourceID uint64
		var payload []byte
		if err := rows.Scan(&stageID, &resourceID, &payload); err != nil {
			return nil, err
		}
		result[stageID] = candidateStageState{ReportID: resourceID, GenerationID: payloadUint64(payload, "generation_id"), RunID: payloadUint64(payload, "run_id")}
	}
	return result, rows.Err()
}

func mongoCleanupFilters(ids cleanupIDs) []collectionCleanup {
	uint64Or := func(items ...bson.M) bson.M { return bson.M{"$or": items} }
	reportStrings, assessmentStrings := uint64Strings(ids.Reports), uint64Strings(ids.Assessments)
	generationStrings := uint64Strings(ids.Generations)
	return []collectionCleanup{
		{Name: "interpret_report_artifacts", Filter: bson.M{"domain_id": bson.M{"$in": ids.Reports}}},
		{Name: "report_generations", Filter: uint64Or(
			bson.M{"domain_id": bson.M{"$in": ids.Generations}}, bson.M{"outcome_id": bson.M{"$in": ids.Outcomes}}, bson.M{"report_id": bson.M{"$in": ids.Reports}},
		)},
		{Name: "interpretation_runs", Filter: uint64Or(
			bson.M{"domain_id": bson.M{"$in": ids.Runs}}, bson.M{"generation_id": bson.M{"$in": ids.Generations}},
		)},
		{Name: "report_query_catalog", Filter: uint64Or(
			bson.M{"assessment_id": bson.M{"$in": ids.Assessments}}, bson.M{"outcome_id": bson.M{"$in": ids.Outcomes}}, bson.M{"source_id": bson.M{"$in": ids.Reports}},
		)},
		{Name: "archived_reports", Filter: uint64Or(
			bson.M{"domain_id": bson.M{"$in": ids.Assessments}}, bson.M{"outcome_id": bson.M{"$in": ids.Outcomes}},
		)},
		{Name: "interpret_reports", Filter: uint64Or(
			bson.M{"domain_id": bson.M{"$in": ids.Assessments}}, bson.M{"outcome_id": bson.M{"$in": ids.Outcomes}},
		)},
		{Name: "interpretation_admission_failures", Filter: uint64Or(
			bson.M{"assessment_id": bson.M{"$in": ids.Assessments}}, bson.M{"outcome_id": bson.M{"$in": ids.Outcomes}}, bson.M{"generation_id": bson.M{"$in": ids.Generations}},
		)},
		{Name: "interpretation_attention_projections", Filter: uint64Or(
			bson.M{"report_id": bson.M{"$in": reportStrings}}, bson.M{"assessment_id": bson.M{"$in": assessmentStrings}},
		)},
		{Name: "domain_event_outbox", Filter: bson.M{"aggregate_type": "ReportGeneration", "aggregate_id": bson.M{"$in": generationStrings}}},
	}
}

func materializeMongoCleanup(ctx context.Context, db *mongo.Database, ids cleanupIDs) (map[string][]bson.M, error) {
	result := make(map[string][]bson.M)
	for _, item := range mongoCleanupFilters(ids) {
		cursor, err := db.Collection(item.Name).Find(ctx, item.Filter)
		if err != nil {
			return nil, fmt.Errorf("materialize %s cleanup: %w", item.Name, err)
		}
		for cursor.Next(ctx) {
			var document bson.M
			if err := cursor.Decode(&document); err != nil {
				_ = cursor.Close(ctx)
				return nil, err
			}
			if document["_id"] == nil {
				_ = cursor.Close(ctx)
				return nil, fmt.Errorf("%s cleanup document has no _id", item.Name)
			}
			result[item.Name] = append(result[item.Name], document)
		}
		if err := cursor.Close(ctx); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func requireFreshBackups(ctx context.Context, mysqlDB *sql.DB, mongoDB *mongo.Database, cfg config, documents map[string][]bson.M) error {
	tables := []string{statisticsBackupTable(cfg.BackupSuffix), mysqlOutboxBackupTable(cfg.BackupSuffix)}
	var existingTables int
	if err := mysqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema=DATABASE() AND table_name IN (?,?)`, tables[0], tables[1]).Scan(&existingTables); err != nil {
		return fmt.Errorf("check MySQL backup tables: %w", err)
	}
	if existingTables != 0 {
		return fmt.Errorf("backup suffix %q was already used for MySQL; choose a new suffix", cfg.BackupSuffix)
	}

	backupNames := make([]string, 0, len(documents))
	for collection := range documents {
		backupNames = append(backupNames, backupCollectionName(collection, cfg.BackupSuffix))
	}
	if len(backupNames) == 0 {
		return nil
	}
	existingCollections, err := mongoDB.ListCollectionNames(ctx, bson.M{"name": bson.M{"$in": backupNames}})
	if err != nil {
		return fmt.Errorf("check Mongo backup collections: %w", err)
	}
	if len(existingCollections) != 0 {
		sort.Strings(existingCollections)
		return fmt.Errorf("backup suffix %q was already used for Mongo collections: %s", cfg.BackupSuffix, strings.Join(existingCollections, ","))
	}
	return nil
}

func backupMongoDocuments(ctx context.Context, db *mongo.Database, suffix string, documents map[string][]bson.M) (int64, map[string]int64, error) {
	collections := make([]string, 0, len(documents))
	for collection := range documents {
		collections = append(collections, collection)
	}
	sort.Strings(collections)
	perCollection := make(map[string]int64)
	var total int64
	for _, collection := range collections {
		docs := documents[collection]
		if len(docs) == 0 {
			continue
		}
		backup := db.Collection(backupCollectionName(collection, suffix))
		for start := 0; start < len(docs); start += 500 {
			end := start + 500
			if end > len(docs) {
				end = len(docs)
			}
			items := make([]any, 0, end-start)
			ids := make([]any, 0, end-start)
			for _, document := range docs[start:end] {
				id := document["_id"]
				ids = append(ids, id)
				items = append(items, document)
			}
			result, err := backup.InsertMany(ctx, items, options.InsertMany().SetOrdered(true))
			if err != nil {
				return total, perCollection, fmt.Errorf("backup %s: %w", collection, err)
			}
			if len(result.InsertedIDs) != len(items) {
				return total, perCollection, fmt.Errorf("backup %s insert count mismatch: got %d want %d", collection, len(result.InsertedIDs), len(items))
			}
			count, err := backup.CountDocuments(ctx, bson.M{"_id": bson.M{"$in": ids}})
			if err != nil {
				return total, perCollection, err
			}
			if count != int64(len(ids)) {
				return total, perCollection, fmt.Errorf("backup verification failed for %s: got %d want %d", collection, count, len(ids))
			}
			total += int64(len(ids))
			perCollection[collection] += int64(len(ids))
		}
	}
	return total, perCollection, nil
}

func deleteMongoDocuments(ctx context.Context, db *mongo.Database, documents map[string][]bson.M) (int64, error) {
	order := []string{
		"domain_event_outbox", "interpretation_attention_projections", "interpretation_admission_failures",
		"report_query_catalog", "archived_reports", "interpret_reports", "interpret_report_artifacts",
		"interpretation_runs", "report_generations",
	}
	var total int64
	for _, collection := range order {
		docs := documents[collection]
		for start := 0; start < len(docs); start += 500 {
			end := start + 500
			if end > len(docs) {
				end = len(docs)
			}
			ids := make([]any, 0, end-start)
			for _, document := range docs[start:end] {
				ids = append(ids, document["_id"])
			}
			result, err := db.Collection(collection).DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
			if err != nil {
				return total, fmt.Errorf("delete %s: %w", collection, err)
			}
			if result.DeletedCount != int64(len(ids)) {
				return total, fmt.Errorf("delete %s count mismatch: got %d want %d", collection, result.DeletedCount, len(ids))
			}
			total += result.DeletedCount
		}
	}
	return total, nil
}

func backupStatisticsFacts(ctx context.Context, db *sql.DB, cfg config, candidates []orphanCandidate) (int64, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	if err := prepareStatisticsFactSourceTempTable(ctx, conn, candidates); err != nil {
		return 0, err
	}
	backupTable := statisticsBackupTable(cfg.BackupSuffix)
	if _, err := conn.ExecContext(ctx, `CREATE TABLE `+backupTable+` LIKE statistics_assessment_fact`); err != nil {
		return 0, fmt.Errorf("create statistics backup table: %w", err)
	}
	result, err := conn.ExecContext(ctx, `INSERT INTO `+backupTable+`
SELECT f.* FROM statistics_assessment_fact `+statisticsFactSourceJoin+`
WHERE f.org_id=?`, cfg.OrgID)
	if err != nil {
		return 0, fmt.Errorf("backup statistics facts: %w", err)
	}
	return result.RowsAffected()
}

func deleteStatisticsFacts(ctx context.Context, db *sql.DB, cfg config, candidates []orphanCandidate) (int64, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	if err := prepareStatisticsFactSourceTempTable(ctx, conn, candidates); err != nil {
		return 0, err
	}
	backupTable := statisticsBackupTable(cfg.BackupSuffix)
	var backedUp int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+backupTable+` `+statisticsFactSourceJoin+`
WHERE f.org_id=?`, cfg.OrgID).Scan(&backedUp); err != nil {
		return 0, fmt.Errorf("verify statistics backup: %w", err)
	}
	var source int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM statistics_assessment_fact `+statisticsFactSourceJoin+`
WHERE f.org_id=?`, cfg.OrgID).Scan(&source); err != nil {
		return 0, err
	}
	if backedUp != source {
		return 0, fmt.Errorf("statistics backup does not exactly match source: backed_up=%d source=%d", backedUp, source)
	}
	result, err := conn.ExecContext(ctx, `DELETE f FROM statistics_assessment_fact `+statisticsFactSourceJoin+`
WHERE f.org_id=?`, cfg.OrgID)
	if err != nil {
		return 0, fmt.Errorf("delete orphan statistics facts: %w", err)
	}
	return result.RowsAffected()
}

func prepareStatisticsFactSourceTempTable(ctx context.Context, conn *sql.Conn, candidates []orphanCandidate) error {
	sources, err := statisticsFactSources(candidates)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS tmp_seed_orphan_fact_source`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `CREATE TEMPORARY TABLE tmp_seed_orphan_fact_source (
 fact_type VARCHAR(64) NOT NULL,
 source_type VARCHAR(64) NOT NULL,
 source_ref VARCHAR(128) NOT NULL,
 PRIMARY KEY (source_type,source_ref,fact_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	for start := 0; start < len(sources); start += 500 {
		end := start + 500
		if end > len(sources) {
			end = len(sources)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*3)
		for _, source := range sources[start:end] {
			values = append(values, "(?,?,?)")
			args = append(args, source.FactType, source.SourceType, source.SourceRef)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO tmp_seed_orphan_fact_source (fact_type,source_type,source_ref) VALUES `+strings.Join(values, ","), args...); err != nil {
			return err
		}
	}
	return nil
}

func statisticsFactSources(candidates []orphanCandidate) ([]statisticsFactSource, error) {
	seen := make(map[statisticsFactSource]struct{}, len(candidates)*3)
	for _, candidate := range candidates {
		reportID, assessmentID, outcomeID, _, _, err := parseCandidateIDs(candidate)
		if err != nil {
			return nil, err
		}
		for _, source := range []statisticsFactSource{
			{FactType: "assessment_created", SourceType: "assessment", SourceRef: strconv.FormatUint(assessmentID, 10)},
			{FactType: "outcome_committed", SourceType: "evaluation_outcome", SourceRef: strconv.FormatUint(outcomeID, 10)},
			{FactType: "report_generated", SourceType: "interpret_report", SourceRef: strconv.FormatUint(reportID, 10)},
		} {
			seen[source] = struct{}{}
		}
	}
	result := make([]statisticsFactSource, 0, len(seen))
	for source := range seen {
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FactType != result[j].FactType {
			return result[i].FactType < result[j].FactType
		}
		if result[i].SourceType != result[j].SourceType {
			return result[i].SourceType < result[j].SourceType
		}
		return result[i].SourceRef < result[j].SourceRef
	})
	return result, nil
}

func backupMySQLOutbox(ctx context.Context, db *sql.DB, cfg config, candidates []orphanCandidate) (int64, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	if err := prepareCandidateTempTable(ctx, conn, candidates); err != nil {
		return 0, err
	}
	backupTable := mysqlOutboxBackupTable(cfg.BackupSuffix)
	if _, err := conn.ExecContext(ctx, `CREATE TABLE `+backupTable+` LIKE domain_event_outbox`); err != nil {
		return 0, fmt.Errorf("create MySQL outbox backup table: %w", err)
	}
	result, err := conn.ExecContext(ctx, `INSERT INTO `+backupTable+`
SELECT o.* FROM domain_event_outbox o FORCE INDEX (idx_outbox_aggregate_event_latest) JOIN tmp_seed_orphan_candidate x
 ON o.aggregate_type='Evaluation' AND o.aggregate_id=CAST(x.assessment_id AS CHAR)
WHERE o.event_type='evaluation.outcome.committed'
  AND JSON_UNQUOTE(JSON_EXTRACT(o.payload_json,'$.data.outcome_id'))=CAST(x.outcome_id AS CHAR)`)
	if err != nil {
		return 0, fmt.Errorf("backup MySQL outcome outbox: %w", err)
	}
	return result.RowsAffected()
}

func deleteMySQLOutbox(ctx context.Context, db *sql.DB, cfg config, candidates []orphanCandidate) (int64, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	if err := prepareCandidateTempTable(ctx, conn, candidates); err != nil {
		return 0, err
	}
	backupTable := mysqlOutboxBackupTable(cfg.BackupSuffix)
	countQuery := func(table string) string {
		return `SELECT COUNT(*) FROM ` + table + ` o FORCE INDEX (idx_outbox_aggregate_event_latest) JOIN tmp_seed_orphan_candidate x
 ON o.aggregate_type='Evaluation' AND o.aggregate_id=CAST(x.assessment_id AS CHAR)
WHERE o.event_type='evaluation.outcome.committed'
  AND JSON_UNQUOTE(JSON_EXTRACT(o.payload_json,'$.data.outcome_id'))=CAST(x.outcome_id AS CHAR)`
	}
	var backedUp, source int64
	if err := conn.QueryRowContext(ctx, countQuery(backupTable)).Scan(&backedUp); err != nil {
		return 0, fmt.Errorf("verify MySQL outbox backup: %w", err)
	}
	if err := conn.QueryRowContext(ctx, countQuery("domain_event_outbox")).Scan(&source); err != nil {
		return 0, fmt.Errorf("count MySQL outcome outbox source: %w", err)
	}
	if backedUp != source {
		return 0, fmt.Errorf("MySQL outbox backup does not exactly match source: backed_up=%d source=%d", backedUp, source)
	}
	result, err := conn.ExecContext(ctx, `DELETE o FROM domain_event_outbox o FORCE INDEX (idx_outbox_aggregate_event_latest) JOIN tmp_seed_orphan_candidate x
 ON o.aggregate_type='Evaluation' AND o.aggregate_id=CAST(x.assessment_id AS CHAR)
WHERE o.event_type='evaluation.outcome.committed'
  AND JSON_UNQUOTE(JSON_EXTRACT(o.payload_json,'$.data.outcome_id'))=CAST(x.outcome_id AS CHAR)`)
	if err != nil {
		return 0, fmt.Errorf("delete MySQL outcome outbox: %w", err)
	}
	return result.RowsAffected()
}

func prepareCandidateTempTable(ctx context.Context, conn *sql.Conn, candidates []orphanCandidate) error {
	if _, err := conn.ExecContext(ctx, `DROP TEMPORARY TABLE IF EXISTS tmp_seed_orphan_candidate`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `CREATE TEMPORARY TABLE tmp_seed_orphan_candidate (
 report_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
 assessment_id BIGINT UNSIGNED NOT NULL,
 outcome_id BIGINT UNSIGNED NOT NULL
) ENGINE=InnoDB`); err != nil {
		return err
	}
	for start := 0; start < len(candidates); start += 500 {
		end := start + 500
		if end > len(candidates) {
			end = len(candidates)
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*3)
		for _, candidate := range candidates[start:end] {
			reportID, assessmentID, outcomeID, _, _, err := parseCandidateIDs(candidate)
			if err != nil {
				return err
			}
			values = append(values, "(?,?,?)")
			args = append(args, reportID, assessmentID, outcomeID)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO tmp_seed_orphan_candidate (report_id,assessment_id,outcome_id) VALUES `+strings.Join(values, ","), args...); err != nil {
			return err
		}
	}
	return nil
}

func backupCollectionName(collection, suffix string) string {
	return collection + "__seed_orphan_backup_" + suffix
}

func statisticsBackupTable(suffix string) string {
	return "seed_orphan_stats_bak_" + suffix
}

func mysqlOutboxBackupTable(suffix string) string {
	return "seed_orphan_outbox_bak_" + suffix
}

func uint64Strings(values []uint64) []string {
	result := make([]string, 0, len(values))
	for _, value := range uniqueIDs(values) {
		result = append(result, strconv.FormatUint(value, 10))
	}
	return result
}
