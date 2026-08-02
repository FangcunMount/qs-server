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

const auditReportVersion = 1

type auditReport struct {
	Version            int               `json:"version"`
	OrgID              int64             `json:"org_id"`
	BatchID            string            `json:"batch_id"`
	From               string            `json:"from"`
	To                 string            `json:"to"`
	GeneratedAt        time.Time         `json:"generated_at"`
	Storage            storageIdentity   `json:"storage"`
	Valid              bool              `json:"valid"`
	ProblemCount       int64             `json:"problem_count"`
	WarningCount       int64             `json:"warning_count"`
	FindingDetailsCut  bool              `json:"finding_details_truncated,omitempty"`
	StageCounts        map[string]int64  `json:"stage_counts"`
	Checks             []integrityCheck  `json:"checks"`
	Findings           []finding         `json:"findings,omitempty"`
	ReportStages       int64             `json:"report_stages"`
	AnswerSheetStages  int64             `json:"answersheet_stages"`
	DeletionCandidates []orphanCandidate `json:"deletion_candidates,omitempty"`
	PlanHash           string            `json:"plan_hash"`
}

type integrityCheck struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Problems int64  `json:"problems"`
	Meaning  string `json:"meaning"`
}

type finding struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	ScenarioID   string `json:"scenario_id,omitempty"`
	Stage        string `json:"stage,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Message      string `json:"message"`
}

type orphanCandidate struct {
	ScenarioID     string `json:"scenario_id"`
	StageID        uint64 `json:"stage_id"`
	ArtifactObject string `json:"artifact_object_id"`
	ReportID       string `json:"report_id"`
	AssessmentID   string `json:"assessment_id"`
	OutcomeID      string `json:"outcome_id"`
	GenerationID   string `json:"generation_id"`
	RunID          string `json:"run_id,omitempty"`
	Reason         string `json:"reason"`
	GeneratedAt    string `json:"generated_at"`
}

type reportStage struct {
	ID          uint64
	ScenarioID  string
	BusinessAt  time.Time
	ResourceID  uint64
	PayloadJSON []byte
	RawResource string
}

type artifactRow struct {
	ObjectID            primitive.ObjectID `bson:"_id"`
	DomainID            uint64             `bson:"domain_id"`
	GenerationID        uint64             `bson:"generation_id"`
	OutcomeID           uint64             `bson:"outcome_id"`
	InterpretationRunID uint64             `bson:"interpretation_run_id"`
	OrgID               int64              `bson:"org_id"`
	AssessmentID        uint64             `bson:"assessment_id"`
	TesteeID            uint64             `bson:"testee_id"`
	GeneratedAt         time.Time          `bson:"generated_at"`
}

type assessmentRow struct {
	ID            uint64
	OrgID         int64
	TesteeID      uint64
	AnswerSheetID uint64
}

type outcomeRow struct {
	ID           uint64
	OrgID        int64
	AssessmentID uint64
	TesteeID     uint64
}

type generationRow struct {
	DomainID  uint64 `bson:"domain_id"`
	OutcomeID uint64 `bson:"outcome_id"`
	LatestRun uint64 `bson:"latest_run_id"`
	ReportID  uint64 `bson:"report_id"`
	Status    string `bson:"status"`
}

type runRow struct {
	DomainID   uint64 `bson:"domain_id"`
	Generation uint64 `bson:"generation_id"`
	Status     string `bson:"status"`
}

type catalogRow struct {
	AssessmentID uint64 `bson:"assessment_id"`
	OutcomeID    uint64 `bson:"outcome_id"`
	GenerationID uint64 `bson:"generation_id"`
	SourceKind   string `bson:"source_kind"`
	SourceID     uint64 `bson:"source_id"`
}

type reportEvidence struct {
	Stage      reportStage
	Artifact   *artifactRow
	Assessment *assessmentRow
	Outcome    *outcomeRow
	Generation *generationRow
	Run        *runRow
	Catalog    *catalogRow
}

func auditBatch(ctx context.Context, mysqlDB *sql.DB, mongoDB *mongo.Database, cfg config, progress io.Writer) (auditReport, error) {
	report := auditReport{
		Version: auditReportVersion, OrgID: cfg.OrgID, BatchID: cfg.BatchID,
		From: cfg.From, To: cfg.To, GeneratedAt: time.Now().UTC(), StageCounts: map[string]int64{},
	}
	storage, err := loadStorageIdentity(ctx, mysqlDB, mongoDB)
	if err != nil {
		return report, err
	}
	report.Storage = storage
	counts, err := loadStageCounts(ctx, mysqlDB, cfg)
	if err != nil {
		return report, err
	}
	if len(counts) == 0 {
		return report, fmt.Errorf("seed_backfill_stage has no completed rows for org=%d batch=%s", cfg.OrgID, cfg.BatchID)
	}
	report.StageCounts = counts

	checks, err := runSetChecks(ctx, mysqlDB, cfg)
	if err != nil {
		return report, err
	}
	for _, check := range checks {
		report.Checks = append(report.Checks, check)
		if check.Severity == "error" {
			report.ProblemCount += check.Problems
		} else {
			report.WarningCount += check.Problems
		}
	}

	if err := auditReportStages(ctx, mysqlDB, mongoDB, cfg, &report, progress); err != nil {
		return report, err
	}
	if err := auditAnswerSheetStages(ctx, mysqlDB, mongoDB, cfg, &report, progress); err != nil {
		return report, err
	}
	if len(report.DeletionCandidates) > cfg.MaxCandidates {
		addFinding(&report, cfg.MaxFindings, finding{
			Code: "delete_candidate_limit_exceeded", Severity: "error",
			Message: fmt.Sprintf("delete candidate count %d exceeds safety ceiling %d; apply will refuse this report", len(report.DeletionCandidates), cfg.MaxCandidates),
		})
	}
	sortCandidates(report.DeletionCandidates)
	report.PlanHash, err = deletionPlanHash(report)
	if err != nil {
		return report, err
	}
	report.Valid = report.ProblemCount == 0
	return report, nil
}

func loadStageCounts(ctx context.Context, db *sql.DB, cfg config) (map[string]int64, error) {
	from, to, err := cfg.businessWindow()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT stage, COUNT(*)
FROM seed_backfill_stage
WHERE org_id=? AND batch_id=? AND status='completed' AND business_at>=? AND business_at<?
GROUP BY stage ORDER BY stage`, cfg.OrgID, cfg.BatchID, from, to)
	if err != nil {
		return nil, fmt.Errorf("load stage counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]int64)
	for rows.Next() {
		var stage string
		var count int64
		if err := rows.Scan(&stage, &count); err != nil {
			return nil, err
		}
		result[stage] = count
	}
	return result, rows.Err()
}

type setCheckDefinition struct {
	Name, Severity, Meaning, Query string
}

func runSetChecks(ctx context.Context, db *sql.DB, cfg config) ([]integrityCheck, error) {
	from, to, err := cfg.businessWindow()
	if err != nil {
		return nil, err
	}
	definitions := []setCheckDefinition{
		{
			Name: "scenario_date_mismatch", Severity: "error",
			Meaning: "the scenario date prefix must equal the Shanghai business date",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<?
  AND s.scenario_id REGEXP '^[0-9]{4}-[0-9]{2}-[0-9]{2}/' AND LEFT(s.scenario_id,10)<>DATE_FORMAT(s.business_at,'%Y-%m-%d')`,
		},
		{
			Name: "invalid_stage_resource_identity", Severity: "error",
			Meaning: "resource-bearing completed stages must use a positive decimal ID and the canonical resource type",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<?
AND (s.resource_id NOT REGEXP '^[1-9][0-9]*$' OR s.resource_type<>CASE s.stage
 WHEN 'entry_resolve' THEN 'assessment_entry' WHEN 'entry_intake' THEN 'testee'
 WHEN 'plan_enrollment' THEN 'plan_enrollment' WHEN 'task_open' THEN 'plan_task' WHEN 'task_complete' THEN 'plan_task'
 WHEN 'answersheet_submit' THEN 'answer_sheet' WHEN 'assessment_created' THEN 'assessment'
 WHEN 'assessment_submitted' THEN 'assessment' WHEN 'outcome_committed' THEN 'evaluation_outcome'
 WHEN 'report_generated' THEN 'interpretation_report' ELSE s.resource_type END)`,
		},
		{
			Name: "missing_assessment_stage_resource", Severity: "error",
			Meaning: "assessment_created/submitted stages must reference a live Assessment in the same organization",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s LEFT JOIN assessment a
 ON a.org_id=s.org_id AND a.id=CAST(s.resource_id AS UNSIGNED)
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage IN ('assessment_created','assessment_submitted')
 AND s.resource_id REGEXP '^[1-9][0-9]*$' AND a.id IS NULL`,
		},
		{
			Name: "missing_outcome_stage_resource", Severity: "error",
			Meaning: "outcome_committed stages must reference a live EvaluationOutcome in the same organization",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s LEFT JOIN evaluation_outcome o
 ON o.org_id=s.org_id AND o.id=CAST(s.resource_id AS UNSIGNED)
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage='outcome_committed'
 AND s.resource_id REGEXP '^[1-9][0-9]*$' AND o.id IS NULL`,
		},
		{
			Name: "outcome_missing_assessment_parent", Severity: "error",
			Meaning: "a batch-owned EvaluationOutcome must retain its Assessment parent",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s
JOIN evaluation_outcome o ON o.org_id=s.org_id AND o.id=CAST(s.resource_id AS UNSIGNED)
LEFT JOIN assessment a ON a.org_id=o.org_id AND a.id=o.assessment_id
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage='outcome_committed'
 AND s.resource_id REGEXP '^[1-9][0-9]*$' AND a.id IS NULL`,
		},
		{
			Name: "missing_plan_enrollment_stage_resource", Severity: "error",
			Meaning: "plan_enrollment stages must reference a live enrollment in the same organization",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s LEFT JOIN plan_enrollment e
 ON e.org_id=s.org_id AND e.id=CAST(s.resource_id AS UNSIGNED)
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage='plan_enrollment'
 AND s.resource_id REGEXP '^[1-9][0-9]*$' AND e.id IS NULL`,
		},
		{
			Name: "missing_plan_task_stage_resource", Severity: "error",
			Meaning: "task_open/complete stages must reference a live Plan task in the same organization",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s LEFT JOIN assessment_task t
 ON t.org_id=s.org_id AND t.id=CAST(s.resource_id AS UNSIGNED)
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage IN ('task_open','task_complete')
 AND s.resource_id REGEXP '^[1-9][0-9]*$' AND t.id IS NULL`,
		},
		{
			Name: "plan_task_missing_enrollment_parent", Severity: "error",
			Meaning: "batch-owned Plan tasks must retain their enrollment parent",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s
JOIN assessment_task t ON t.org_id=s.org_id AND t.id=CAST(s.resource_id AS UNSIGNED)
LEFT JOIN plan_enrollment e ON e.org_id=t.org_id AND e.id=t.enrollment_id
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage IN ('task_open','task_complete')
 AND s.resource_id REGEXP '^[1-9][0-9]*$' AND e.id IS NULL`,
		},
		{
			Name: "assessment_answersheet_stage_mismatch", Severity: "error",
			Meaning: "within one scenario, Assessment.answer_sheet_id must equal the answersheet_submit resource",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage sa
JOIN seed_backfill_stage ss ON ss.org_id=sa.org_id AND ss.batch_id=sa.batch_id AND ss.scenario_id=sa.scenario_id
 AND ss.stage='answersheet_submit' AND ss.status='completed'
JOIN assessment a ON a.org_id=sa.org_id AND a.id=CAST(sa.resource_id AS UNSIGNED)
WHERE sa.org_id=? AND sa.batch_id=? AND sa.status='completed' AND sa.business_at>=? AND sa.business_at<? AND sa.stage='assessment_created'
 AND sa.resource_id REGEXP '^[1-9][0-9]*$' AND ss.resource_id REGEXP '^[1-9][0-9]*$'
 AND a.answer_sheet_id<>CAST(ss.resource_id AS UNSIGNED)`,
		},
		{
			Name: "missing_entry_resolve_log", Severity: "error",
			Meaning: "entry_resolve payload resolve_log_id must reference the durable resolve log",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s LEFT JOIN assessment_entry_resolve_log l
 ON l.id=CAST(JSON_UNQUOTE(JSON_EXTRACT(s.payload_json,'$.resolve_log_id')) AS UNSIGNED)
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage='entry_resolve'
 AND (JSON_EXTRACT(s.payload_json,'$.resolve_log_id') IS NULL OR l.id IS NULL)`,
		},
		{
			Name: "missing_entry_intake_log", Severity: "error",
			Meaning: "entry_intake payload intake_log_id must reference the durable intake log",
			Query: `SELECT COUNT(*) FROM seed_backfill_stage s LEFT JOIN assessment_entry_intake_log l
 ON l.id=CAST(JSON_UNQUOTE(JSON_EXTRACT(s.payload_json,'$.intake_log_id')) AS UNSIGNED)
WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<? AND s.stage='entry_intake'
 AND (JSON_EXTRACT(s.payload_json,'$.intake_log_id') IS NULL OR l.id IS NULL)`,
		},
		{
			Name: "scenario_timeline_inversion", Severity: "error",
			Meaning: "business timestamps must be monotonic in the canonical historical stage order",
			Query: `WITH ordered AS (
 SELECT s.business_at,
 LAG(s.business_at) OVER (PARTITION BY s.scenario_id ORDER BY CASE s.stage
  WHEN 'entry_resolve' THEN 10 WHEN 'entry_intake' THEN 20 WHEN 'plan_enrollment' THEN 30
  WHEN 'task_open' THEN 40 WHEN 'answersheet_submit' THEN 50 WHEN 'assessment_created' THEN 60
  WHEN 'assessment_submitted' THEN 70 WHEN 'task_complete' THEN 75 WHEN 'outcome_committed' THEN 80
  WHEN 'report_generated' THEN 90 ELSE 999 END) previous_at
	 FROM seed_backfill_stage s WHERE s.org_id=? AND s.batch_id=? AND s.status='completed' AND s.business_at>=? AND s.business_at<?)
SELECT COUNT(*) FROM ordered WHERE previous_at IS NOT NULL AND business_at<previous_at`,
		},
	}

	checks := make([]integrityCheck, 0, len(definitions))
	for _, definition := range definitions {
		args := []any{cfg.OrgID, cfg.BatchID, from, to}
		var count int64
		if err := db.QueryRowContext(ctx, definition.Query, args...).Scan(&count); err != nil {
			return nil, fmt.Errorf("run integrity check %s: %w", definition.Name, err)
		}
		checks = append(checks, integrityCheck{Name: definition.Name, Severity: definition.Severity, Problems: count, Meaning: definition.Meaning})
	}
	return checks, nil
}

func auditReportStages(ctx context.Context, db *sql.DB, mongoDB *mongo.Database, cfg config, report *auditReport, progress io.Writer) error {
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return err
	}
	var afterID uint64
	for {
		stages, err := loadReportStagePage(ctx, db, cfg, location, afterID)
		if err != nil {
			return err
		}
		if len(stages) == 0 {
			return nil
		}
		afterID = stages[len(stages)-1].ID
		if err := auditReportStagePage(ctx, db, mongoDB, cfg, report, stages); err != nil {
			return err
		}
		report.ReportStages += int64(len(stages))
		_, _ = fmt.Fprintf(progress, "audit report stages: %d\n", report.ReportStages)
	}
}

func loadReportStagePage(ctx context.Context, db *sql.DB, cfg config, location *time.Location, afterID uint64) ([]reportStage, error) {
	from, to, err := cfg.businessWindow()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id,scenario_id,DATE_FORMAT(business_at,'%Y-%m-%d %H:%i:%s.%f'),resource_id,payload_json
FROM seed_backfill_stage
WHERE org_id=? AND batch_id=? AND status='completed' AND business_at>=? AND business_at<? AND stage='report_generated' AND id>?
ORDER BY id LIMIT ?`, cfg.OrgID, cfg.BatchID, from, to, afterID, cfg.PageSize)
	if err != nil {
		return nil, fmt.Errorf("load report stage page: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]reportStage, 0, cfg.PageSize)
	for rows.Next() {
		var stage reportStage
		var businessAt, resourceID string
		if err := rows.Scan(&stage.ID, &stage.ScenarioID, &businessAt, &resourceID, &stage.PayloadJSON); err != nil {
			return nil, err
		}
		stage.RawResource = resourceID
		stage.BusinessAt, err = time.ParseInLocation("2006-01-02 15:04:05.000000", businessAt, location)
		if err != nil {
			return nil, fmt.Errorf("parse report stage %d business_at: %w", stage.ID, err)
		}
		stage.ResourceID, _ = strconv.ParseUint(resourceID, 10, 64)
		result = append(result, stage)
	}
	return result, rows.Err()
}

func auditReportStagePage(ctx context.Context, db *sql.DB, mongoDB *mongo.Database, cfg config, report *auditReport, stages []reportStage) error {
	reportIDs := make([]uint64, 0, len(stages))
	for _, stage := range stages {
		if stage.ResourceID == 0 {
			addFinding(report, cfg.MaxFindings, finding{Code: "invalid_report_stage_id", Severity: "error", ScenarioID: stage.ScenarioID, Stage: "report_generated", ResourceType: "interpretation_report", ResourceID: stage.RawResource, Message: "report stage resource_id is not a positive decimal ID"})
			continue
		}
		reportIDs = append(reportIDs, stage.ResourceID)
	}
	artifacts, err := loadArtifacts(ctx, mongoDB, reportIDs)
	if err != nil {
		return err
	}
	assessmentIDs, outcomeIDs, generationIDs, runIDs := make([]uint64, 0, len(artifacts)), make([]uint64, 0, len(artifacts)), make([]uint64, 0, len(artifacts)), make([]uint64, 0, len(artifacts))
	for _, artifact := range artifacts {
		assessmentIDs = append(assessmentIDs, artifact.AssessmentID)
		outcomeIDs = append(outcomeIDs, artifact.OutcomeID)
		generationIDs = append(generationIDs, artifact.GenerationID)
		runIDs = append(runIDs, artifact.InterpretationRunID)
	}
	assessments, err := loadAssessmentRows(ctx, db, cfg.OrgID, assessmentIDs)
	if err != nil {
		return err
	}
	outcomes, err := loadOutcomeRows(ctx, db, cfg.OrgID, outcomeIDs)
	if err != nil {
		return err
	}
	generations, err := loadGenerations(ctx, mongoDB, generationIDs)
	if err != nil {
		return err
	}
	runs, err := loadRuns(ctx, mongoDB, runIDs, generationIDs)
	if err != nil {
		return err
	}
	catalogs, err := loadCatalogs(ctx, mongoDB, assessmentIDs)
	if err != nil {
		return err
	}

	for _, stage := range stages {
		if stage.ResourceID == 0 {
			continue
		}
		artifact := artifacts[stage.ResourceID]
		evidence := reportEvidence{Stage: stage, Artifact: artifact}
		if artifact != nil {
			evidence.Assessment = assessments[artifact.AssessmentID]
			evidence.Outcome = outcomes[artifact.OutcomeID]
			evidence.Generation = generations[artifact.GenerationID]
			evidence.Run = selectRun(artifact, runs)
			evidence.Catalog = selectCatalog(artifact, catalogs)
		}
		findings, candidate := classifyReportEvidence(cfg, evidence)
		for _, item := range findings {
			addFinding(report, cfg.MaxFindings, item)
		}
		if candidate != nil {
			report.DeletionCandidates = append(report.DeletionCandidates, *candidate)
		}
	}
	return nil
}

func classifyReportEvidence(cfg config, evidence reportEvidence) ([]finding, *orphanCandidate) {
	stage := evidence.Stage
	base := finding{ScenarioID: stage.ScenarioID, Stage: "report_generated", ResourceType: "interpretation_report", ResourceID: strconv.FormatUint(stage.ResourceID, 10)}
	if evidence.Artifact == nil {
		base.Code, base.Severity, base.Message = "report_stage_artifact_missing", "error", "completed report stage has no Mongo report artifact"
		return []finding{base}, nil
	}
	artifact := evidence.Artifact
	var findings []finding
	add := func(code, severity, message string) {
		item := base
		item.Code, item.Severity, item.Message = code, severity, message
		findings = append(findings, item)
	}
	if artifact.OrgID != cfg.OrgID {
		add("report_org_mismatch", "error", fmt.Sprintf("artifact org_id=%d, expected %d", artifact.OrgID, cfg.OrgID))
		return findings, nil
	}
	autoDeleteBlocked := false
	if artifact.AssessmentID == 0 || artifact.OutcomeID == 0 || artifact.GenerationID == 0 || artifact.InterpretationRunID == 0 || artifact.TesteeID == 0 {
		add("report_identity_incomplete", "error", "report artifact is missing one or more required Assessment/Outcome/Generation/Run/Testee IDs")
		autoDeleteBlocked = true
	}
	if delta := artifact.GeneratedAt.Sub(stage.BusinessAt).Abs(); delta > time.Millisecond {
		add("report_business_time_mismatch", "error", fmt.Sprintf("artifact generated_at=%s, stage business_at=%s", artifact.GeneratedAt.Format(time.RFC3339Nano), stage.BusinessAt.Format(time.RFC3339Nano)))
		autoDeleteBlocked = true
	}
	payloadGeneration := payloadUint64(stage.PayloadJSON, "generation_id")
	payloadRun := payloadUint64(stage.PayloadJSON, "run_id")
	if payloadGeneration != 0 && payloadGeneration != artifact.GenerationID {
		add("report_generation_payload_mismatch", "error", fmt.Sprintf("stage generation_id=%d, artifact generation_id=%d", payloadGeneration, artifact.GenerationID))
		autoDeleteBlocked = true
	}
	if payloadRun != 0 && payloadRun != artifact.InterpretationRunID {
		add("report_run_payload_mismatch", "error", fmt.Sprintf("stage run_id=%d, artifact interpretation_run_id=%d", payloadRun, artifact.InterpretationRunID))
		autoDeleteBlocked = true
	}
	if evidence.Assessment != nil && (evidence.Assessment.OrgID != cfg.OrgID || evidence.Assessment.TesteeID != artifact.TesteeID) {
		add("report_assessment_mismatch", "error", "artifact assessment/testee identity does not match MySQL Assessment")
		autoDeleteBlocked = true
	}
	if evidence.Outcome != nil && (evidence.Outcome.OrgID != cfg.OrgID || evidence.Outcome.AssessmentID != artifact.AssessmentID || evidence.Outcome.TesteeID != artifact.TesteeID) {
		add("report_outcome_mismatch", "error", "artifact outcome identity does not match MySQL EvaluationOutcome")
		autoDeleteBlocked = true
	}
	if evidence.Generation != nil && (evidence.Generation.OutcomeID != artifact.OutcomeID || (evidence.Generation.ReportID != 0 && evidence.Generation.ReportID != artifact.DomainID)) {
		add("report_generation_mismatch", "error", "Mongo report generation points to a different Outcome or Report")
		autoDeleteBlocked = true
	}
	if evidence.Run != nil && evidence.Run.Generation != artifact.GenerationID {
		add("report_run_mismatch", "error", "Mongo interpretation run points to a different generation")
		autoDeleteBlocked = true
	}
	if evidence.Catalog != nil && (evidence.Catalog.AssessmentID != artifact.AssessmentID || evidence.Catalog.OutcomeID != artifact.OutcomeID || evidence.Catalog.SourceKind != "artifact" || evidence.Catalog.SourceID != artifact.DomainID) {
		add("report_catalog_mismatch", "error", "report query catalog points to a different report chain")
		autoDeleteBlocked = true
	}

	assessmentMissing := evidence.Assessment == nil
	outcomeMissing := evidence.Outcome == nil
	if assessmentMissing || outcomeMissing {
		missingParents := make([]string, 0, 2)
		if assessmentMissing {
			missingParents = append(missingParents, "assessment")
		}
		if outcomeMissing {
			missingParents = append(missingParents, "evaluation_outcome")
		}
		reason := "missing MySQL parent(s): " + strings.Join(missingParents, ",")
		add("orphan_report_mysql_parent_missing", "error", reason)
		if !assessmentMissing || !outcomeMissing {
			add("orphan_auto_delete_blocked", "error", "only one MySQL parent is missing; this one-time cleanup only deletes the known both-parents-missing failure shape")
			return findings, nil
		}
		if autoDeleteBlocked {
			add("orphan_auto_delete_blocked", "error", "the orphan also has an identity mismatch; automatic deletion is unsafe")
			return findings, nil
		}
		candidate := &orphanCandidate{
			ScenarioID: stage.ScenarioID, StageID: stage.ID, ArtifactObject: artifact.ObjectID.Hex(),
			ReportID: strconv.FormatUint(artifact.DomainID, 10), AssessmentID: strconv.FormatUint(artifact.AssessmentID, 10),
			OutcomeID: strconv.FormatUint(artifact.OutcomeID, 10), GenerationID: strconv.FormatUint(artifact.GenerationID, 10),
			RunID: strconv.FormatUint(artifact.InterpretationRunID, 10), Reason: reason, GeneratedAt: artifact.GeneratedAt.UTC().Format(time.RFC3339Nano),
		}
		return findings, candidate
	}

	if evidence.Generation == nil {
		add("report_generation_missing", "error", "report artifact has no Mongo report generation")
	}
	if evidence.Run == nil {
		add("report_run_missing", "error", "report artifact has no matching Mongo interpretation run")
	}
	if evidence.Catalog == nil {
		add("report_catalog_missing", "error", "report artifact has no matching report query catalog row")
	}
	return findings, nil
}

func auditAnswerSheetStages(ctx context.Context, db *sql.DB, mongoDB *mongo.Database, cfg config, report *auditReport, progress io.Writer) error {
	from, to, err := cfg.businessWindow()
	if err != nil {
		return err
	}
	var afterID uint64
	for {
		rows, err := db.QueryContext(ctx, `SELECT id,scenario_id,resource_id FROM seed_backfill_stage
WHERE org_id=? AND batch_id=? AND status='completed' AND business_at>=? AND business_at<? AND stage='answersheet_submit' AND id>?
ORDER BY id LIMIT ?`, cfg.OrgID, cfg.BatchID, from, to, afterID, cfg.PageSize)
		if err != nil {
			return fmt.Errorf("load answersheet stage page: %w", err)
		}
		type item struct {
			ID                     uint64
			ScenarioID, ResourceID string
		}
		var page []item
		for rows.Next() {
			var value item
			if err := rows.Scan(&value.ID, &value.ScenarioID, &value.ResourceID); err != nil {
				_ = rows.Close()
				return err
			}
			page = append(page, value)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		afterID = page[len(page)-1].ID
		ids := make([]uint64, 0, len(page))
		for _, value := range page {
			if id, err := strconv.ParseUint(value.ResourceID, 10, 64); err == nil && id != 0 {
				ids = append(ids, id)
			}
		}
		present, err := loadMongoIDSet(ctx, mongoDB.Collection("answersheets"), "domain_id", ids)
		if err != nil {
			return err
		}
		for _, value := range page {
			id, err := strconv.ParseUint(value.ResourceID, 10, 64)
			if err != nil || id == 0 {
				continue
			}
			if _, ok := present[id]; !ok {
				addFinding(report, cfg.MaxFindings, finding{Code: "answersheet_stage_document_missing", Severity: "error", ScenarioID: value.ScenarioID, Stage: "answersheet_submit", ResourceType: "answer_sheet", ResourceID: value.ResourceID, Message: "completed AnswerSheet stage has no Mongo answersheets document"})
			}
		}
		report.AnswerSheetStages += int64(len(page))
		_, _ = fmt.Fprintf(progress, "audit answersheet stages: %d\n", report.AnswerSheetStages)
	}
}

func loadArtifacts(ctx context.Context, db *mongo.Database, ids []uint64) (map[uint64]*artifactRow, error) {
	result := make(map[uint64]*artifactRow)
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := db.Collection("interpret_report_artifacts").Find(ctx, bson.M{"domain_id": bson.M{"$in": uniqueIDs(ids)}})
	if err != nil {
		return nil, fmt.Errorf("load report artifacts: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row artifactRow
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		copy := row
		result[row.DomainID] = &copy
	}
	return result, cursor.Err()
}

func loadAssessmentRows(ctx context.Context, db *sql.DB, orgID int64, ids []uint64) (map[uint64]*assessmentRow, error) {
	result := make(map[uint64]*assessmentRow)
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return result, nil
	}
	query, args := uint64InQuery(`SELECT id,org_id,testee_id,answer_sheet_id FROM assessment WHERE org_id=? AND id IN (`, orgID, ids)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load assessments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row assessmentRow
		if err := rows.Scan(&row.ID, &row.OrgID, &row.TesteeID, &row.AnswerSheetID); err != nil {
			return nil, err
		}
		copy := row
		result[row.ID] = &copy
	}
	return result, rows.Err()
}

func loadOutcomeRows(ctx context.Context, db *sql.DB, orgID int64, ids []uint64) (map[uint64]*outcomeRow, error) {
	result := make(map[uint64]*outcomeRow)
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return result, nil
	}
	query, args := uint64InQuery(`SELECT id,org_id,assessment_id,testee_id FROM evaluation_outcome WHERE org_id=? AND id IN (`, orgID, ids)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load evaluation outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row outcomeRow
		if err := rows.Scan(&row.ID, &row.OrgID, &row.AssessmentID, &row.TesteeID); err != nil {
			return nil, err
		}
		copy := row
		result[row.ID] = &copy
	}
	return result, rows.Err()
}

func loadGenerations(ctx context.Context, db *mongo.Database, ids []uint64) (map[uint64]*generationRow, error) {
	result := make(map[uint64]*generationRow)
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := db.Collection("report_generations").Find(ctx, bson.M{"domain_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row generationRow
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		copy := row
		result[row.DomainID] = &copy
	}
	return result, cursor.Err()
}

func loadRuns(ctx context.Context, db *mongo.Database, runIDs, generationIDs []uint64) (map[uint64][]runRow, error) {
	result := make(map[uint64][]runRow)
	filters := bson.A{}
	if ids := uniqueIDs(runIDs); len(ids) != 0 {
		filters = append(filters, bson.M{"domain_id": bson.M{"$in": ids}})
	}
	if ids := uniqueIDs(generationIDs); len(ids) != 0 {
		filters = append(filters, bson.M{"generation_id": bson.M{"$in": ids}})
	}
	if len(filters) == 0 {
		return result, nil
	}
	cursor, err := db.Collection("interpretation_runs").Find(ctx, bson.M{"$or": filters})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row runRow
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.Generation] = append(result[row.Generation], row)
	}
	return result, cursor.Err()
}

func loadCatalogs(ctx context.Context, db *mongo.Database, assessmentIDs []uint64) (map[uint64][]catalogRow, error) {
	result := make(map[uint64][]catalogRow)
	ids := uniqueIDs(assessmentIDs)
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := db.Collection("report_query_catalog").Find(ctx, bson.M{"assessment_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row catalogRow
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.SourceID] = append(result[row.SourceID], row)
	}
	return result, cursor.Err()
}

func selectRun(artifact *artifactRow, rows map[uint64][]runRow) *runRow {
	for _, row := range rows[artifact.GenerationID] {
		if artifact.InterpretationRunID == 0 || row.DomainID == artifact.InterpretationRunID {
			copy := row
			return &copy
		}
	}
	return nil
}

func selectCatalog(artifact *artifactRow, rows map[uint64][]catalogRow) *catalogRow {
	for _, row := range rows[artifact.DomainID] {
		if row.AssessmentID == artifact.AssessmentID {
			copy := row
			return &copy
		}
	}
	return nil
}

func loadMongoIDSet(ctx context.Context, collection *mongo.Collection, field string, ids []uint64) (map[uint64]struct{}, error) {
	result := make(map[uint64]struct{})
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return result, nil
	}
	cursor, err := collection.Find(ctx, bson.M{field: bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{field: 1}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	for cursor.Next(ctx) {
		var row struct {
			ID uint64 `bson:"domain_id"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.ID] = struct{}{}
	}
	return result, cursor.Err()
}

func uint64InQuery(prefix string, firstArg any, ids []uint64) (string, []any) {
	args := make([]any, 0, len(ids)+1)
	args = append(args, firstArg)
	for _, id := range ids {
		args = append(args, id)
	}
	return prefix + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")", args
}

func uniqueIDs(values []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(values))
	result := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func payloadUint64(payload []byte, field string) uint64 {
	if len(payload) == 0 {
		return 0
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(payload, &values) != nil {
		return 0
	}
	raw := values[field]
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		value, _ := strconv.ParseUint(asString, 10, 64)
		return value
	}
	var asNumber json.Number
	if json.Unmarshal(raw, &asNumber) == nil {
		value, _ := strconv.ParseUint(asNumber.String(), 10, 64)
		return value
	}
	return 0
}

func addFinding(report *auditReport, limit int, item finding) {
	if item.Severity == "error" {
		report.ProblemCount++
	} else {
		report.WarningCount++
	}
	if len(report.Findings) >= limit {
		report.FindingDetailsCut = true
		return
	}
	report.Findings = append(report.Findings, item)
}

func sortCandidates(candidates []orphanCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ReportID != candidates[j].ReportID {
			return candidates[i].ReportID < candidates[j].ReportID
		}
		return candidates[i].StageID < candidates[j].StageID
	})
}

func deletionPlanHash(report auditReport) (string, error) {
	payload, err := json.Marshal(struct {
		Version    int               `json:"version"`
		OrgID      int64             `json:"org_id"`
		BatchID    string            `json:"batch_id"`
		From       string            `json:"from"`
		To         string            `json:"to"`
		Storage    storageIdentity   `json:"storage"`
		Candidates []orphanCandidate `json:"candidates"`
	}{Version: report.Version, OrgID: report.OrgID, BatchID: report.BatchID, From: report.From, To: report.To, Storage: report.Storage, Candidates: report.DeletionCandidates})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func decodeAuditReport(path string) (auditReport, error) {
	payload, err := osReadFile(path)
	if err != nil {
		return auditReport{}, err
	}
	var report auditReport
	if err := json.Unmarshal(payload, &report); err != nil {
		return auditReport{}, fmt.Errorf("decode audit report: %w", err)
	}
	if report.Version != auditReportVersion || report.OrgID <= 0 || report.BatchID == "" || report.PlanHash == "" || report.Storage.MySQLDatabase == "" || report.Storage.MongoDatabase == "" {
		return auditReport{}, errors.New("audit report is incomplete or unsupported")
	}
	want, err := deletionPlanHash(report)
	if err != nil {
		return auditReport{}, err
	}
	if want != report.PlanHash {
		return auditReport{}, errors.New("audit report deletion plan hash mismatch")
	}
	return report, nil
}

var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }
