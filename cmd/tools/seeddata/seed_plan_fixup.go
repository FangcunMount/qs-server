package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	answerSheetMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	evaluationMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	evaluationMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	planMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

const (
	planFixupBatchSize        = 500
	planFixupCompletionOffset = 5 * time.Minute
	planFixupInterpretOffset  = 30 * time.Second
	planFixupDefaultExpireTTL = 7 * 24 * time.Hour
)

type planFixupTaskRow struct {
	ID           uint64     `gorm:"column:id"`
	PlanID       uint64     `gorm:"column:plan_id"`
	OrgID        int64      `gorm:"column:org_id"`
	TesteeID     uint64     `gorm:"column:testee_id"`
	PlannedAt    time.Time  `gorm:"column:planned_at"`
	OpenAt       *time.Time `gorm:"column:open_at"`
	ExpireAt     *time.Time `gorm:"column:expire_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at"`
	Status       string     `gorm:"column:status"`
	AssessmentID *uint64    `gorm:"column:assessment_id"`
}

type planFixupAssessmentRow struct {
	ID            uint64     `gorm:"column:id"`
	AnswerSheetID uint64     `gorm:"column:answer_sheet_id"`
	Status        string     `gorm:"column:status"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
	SubmittedAt   *time.Time `gorm:"column:submitted_at"`
	InterpretedAt *time.Time `gorm:"column:interpreted_at"`
	FailedAt      *time.Time `gorm:"column:failed_at"`
}

type planFixupTimes struct {
	OpenAt       time.Time
	ExpireAt     time.Time
	CompletionAt time.Time
	InterpretAt  time.Time
	TTL          time.Duration
}

type planFixupTaskPatch struct {
	OpenAt      time.Time
	ExpireAt    time.Time
	CompletedAt *time.Time
	UpdatedAt   time.Time
}

type planFixupStats struct {
	TasksProcessed        int
	TasksUpdated          int
	AssessmentsUpdated    int
	AnswerSheetsUpdated   int
	ReportsUpdated        int
	MissingAssessments    int
	MissingAnswerSheets   int
	MissingReports        int
	SkippedInvalidCreated int
}

func seedPlanFixupTimestamps(ctx context.Context, deps *dependencies, opts planFixupOptions) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	cfg := deps.Config.Local
	if strings.TrimSpace(cfg.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for plan_fixup_timestamps")
	}
	if strings.TrimSpace(cfg.MongoURI) == "" {
		return fmt.Errorf("seeddata local.mongo_uri is required for plan_fixup_timestamps")
	}
	if strings.TrimSpace(cfg.MongoDatabase) == "" {
		return fmt.Errorf("seeddata local.mongo_database is required for plan_fixup_timestamps")
	}

	planID := normalizePlanID(opts.PlanID)
	planIDUint := parseID(planID)
	if planIDUint == 0 {
		return fmt.Errorf("invalid plan id: %s", planID)
	}
	scopeTesteeIDs, err := parsePlanFixupScopeTesteeIDs(opts.ScopeTesteeIDs)
	if err != nil {
		return err
	}

	mysqlDB, err := openLocalSeedMySQL(cfg.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after plan timestamp fixup", "error", closeErr.Error())
		}
	}()

	mongoClient, mongoDB, err := openLocalSeedMongo(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := mongoClient.Disconnect(context.Background()); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mongo after plan timestamp fixup", "error", closeErr.Error())
		}
	}()

	deps.Logger.Infow("Plan timestamp fixup started",
		"plan_id", planID,
		"scope_testee_count", len(scopeTesteeIDs),
		"batch_size", planFixupBatchSize,
		"completion_offset", planFixupCompletionOffset.String(),
		"interpret_offset", planFixupInterpretOffset.String(),
		"default_expire_ttl", planFixupDefaultExpireTTL.String(),
	)

	stats, err := runPlanTimestampFixup(ctx, mysqlDB, mongoDB, deps.Logger, planIDUint, deps.Config.Global.OrgID, scopeTesteeIDs, opts.Verbose)
	if err != nil {
		return err
	}

	deps.Logger.Infow("Plan timestamp fixup completed",
		"plan_id", planID,
		"scope_testee_count", len(scopeTesteeIDs),
		"tasks_processed", stats.TasksProcessed,
		"tasks_updated", stats.TasksUpdated,
		"assessments_updated", stats.AssessmentsUpdated,
		"answersheets_updated", stats.AnswerSheetsUpdated,
		"reports_updated", stats.ReportsUpdated,
		"missing_assessments", stats.MissingAssessments,
		"missing_answersheets", stats.MissingAnswerSheets,
		"missing_reports", stats.MissingReports,
	)
	return nil
}

func runPlanTimestampFixup(
	ctx context.Context,
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	logger interface {
		Infow(string, ...interface{})
		Warnw(string, ...interface{})
	},
	planID uint64,
	orgID int64,
	scopeTesteeIDs []uint64,
	verbose bool,
) (*planFixupStats, error) {
	stats := &planFixupStats{}
	var lastID uint64
	for {
		tasks, err := loadPlanFixupTaskBatch(ctx, mysqlDB, planID, orgID, scopeTesteeIDs, lastID, planFixupBatchSize)
		if err != nil {
			return nil, err
		}
		if len(tasks) == 0 {
			return stats, nil
		}
		lastID = tasks[len(tasks)-1].ID

		assessments, err := loadPlanFixupAssessments(ctx, mysqlDB, tasks)
		if err != nil {
			return nil, err
		}

		for _, task := range tasks {
			stats.TasksProcessed++
			if err := applyPlanTaskTimestampFixup(ctx, mysqlDB, mongoDB, logger, task, assessments, stats, verbose); err != nil {
				return nil, err
			}
		}
	}
}

func loadPlanFixupTaskBatch(
	ctx context.Context,
	mysqlDB *gorm.DB,
	planID uint64,
	orgID int64,
	scopeTesteeIDs []uint64,
	lastID uint64,
	limit int,
) ([]planFixupTaskRow, error) {
	query := mysqlDB.WithContext(ctx).
		Table((planMySQL.AssessmentTaskPO{}).TableName()).
		Select("id, plan_id, org_id, testee_id, planned_at, open_at, expire_at, completed_at, status, assessment_id").
		Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", orgID, planID).
		Where("status IN ?", []string{"opened", "completed", "expired"}).
		Where("id > ?", lastID).
		Order("id ASC").
		Limit(limit)
	if len(scopeTesteeIDs) > 0 {
		query = query.Where("testee_id IN ?", scopeTesteeIDs)
	}

	var tasks []planFixupTaskRow
	if err := query.Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("load plan fixup task batch: %w", err)
	}
	return tasks, nil
}

func loadPlanFixupAssessments(
	ctx context.Context,
	mysqlDB *gorm.DB,
	tasks []planFixupTaskRow,
) (map[uint64]planFixupAssessmentRow, error) {
	assessmentIDs := collectPlanFixupAssessmentIDs(tasks)
	if len(assessmentIDs) == 0 {
		return map[uint64]planFixupAssessmentRow{}, nil
	}

	var rows []planFixupAssessmentRow
	if err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Select("id, answer_sheet_id, status, created_at, updated_at, submitted_at, interpreted_at, failed_at").
		Where("deleted_at IS NULL AND id IN ?", assessmentIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load plan fixup assessments: %w", err)
	}

	result := make(map[uint64]planFixupAssessmentRow, len(rows))
	for _, row := range rows {
		result[row.ID] = row
	}
	return result, nil
}

func applyPlanTaskTimestampFixup(
	ctx context.Context,
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	logger interface {
		Infow(string, ...interface{})
		Warnw(string, ...interface{})
	},
	task planFixupTaskRow,
	assessments map[uint64]planFixupAssessmentRow,
	stats *planFixupStats,
	verbose bool,
) error {
	times := computePlanFixupTimes(task.PlannedAt, task.OpenAt, task.ExpireAt)
	taskPatch := buildPlanFixupTaskPatch(task.Status, times)
	if err := updatePlanFixupTask(ctx, mysqlDB, task.ID, taskPatch); err != nil {
		return err
	}
	stats.TasksUpdated++

	if verbose {
		logger.Infow("Plan task timestamps fixed",
			"task_id", task.ID,
			"status", task.Status,
			"planned_at", task.PlannedAt,
			"open_at", taskPatch.OpenAt,
			"expire_at", taskPatch.ExpireAt,
			"completed_at", taskPatch.CompletedAt,
		)
	}

	if normalizeTaskStatus(task.Status) != "completed" {
		return nil
	}
	if task.AssessmentID == nil || *task.AssessmentID == 0 {
		logger.Warnw("Skipping completed task fixup because assessment is missing",
			"task_id", task.ID,
			"plan_id", task.PlanID,
			"status", task.Status,
		)
		stats.MissingAssessments++
		return nil
	}

	assessmentRow, ok := assessments[*task.AssessmentID]
	if !ok {
		logger.Warnw("Assessment not found during plan timestamp fixup",
			"task_id", task.ID,
			"assessment_id", *task.AssessmentID,
		)
		stats.MissingAssessments++
		return nil
	}

	assessmentStatus := normalizeTaskStatus(assessmentRow.Status)
	reportExists := false
	if assessmentStatus != "failed" {
		var updated bool
		updated, err := updatePlanFixupReport(ctx, mongoDB, assessmentRow.ID, times.InterpretAt)
		if err != nil {
			return err
		}
		if updated {
			reportExists = true
			stats.ReportsUpdated++
		} else {
			stats.MissingReports++
			logger.Warnw("Interpret report not found during plan timestamp fixup",
				"task_id", task.ID,
				"assessment_id", assessmentRow.ID,
			)
		}
	}

	if err := updatePlanFixupAssessment(ctx, mysqlDB, assessmentRow, times, reportExists); err != nil {
		return err
	}
	stats.AssessmentsUpdated++

	if assessmentRow.AnswerSheetID == 0 {
		logger.Warnw("Answer sheet id is missing during plan timestamp fixup",
			"task_id", task.ID,
			"assessment_id", assessmentRow.ID,
		)
		stats.MissingAnswerSheets++
		return nil
	}

	answerSheetUpdatedAt := times.CompletionAt
	if assessmentStatus == "interpreted" || reportExists {
		answerSheetUpdatedAt = times.InterpretAt
	}
	updated, err := updatePlanFixupAnswerSheet(ctx, mongoDB, assessmentRow.AnswerSheetID, times.CompletionAt, answerSheetUpdatedAt)
	if err != nil {
		return err
	}
	if !updated {
		stats.MissingAnswerSheets++
		logger.Warnw("Answer sheet not found during plan timestamp fixup",
			"task_id", task.ID,
			"assessment_id", assessmentRow.ID,
			"answer_sheet_id", assessmentRow.AnswerSheetID,
		)
		return nil
	}
	stats.AnswerSheetsUpdated++
	return nil
}

func updatePlanFixupTask(ctx context.Context, mysqlDB *gorm.DB, taskID uint64, patch planFixupTaskPatch) error {
	updates := map[string]interface{}{
		"open_at":      patch.OpenAt,
		"expire_at":    patch.ExpireAt,
		"completed_at": patch.CompletedAt,
		"updated_at":   patch.UpdatedAt,
	}
	if err := mysqlDB.WithContext(ctx).
		Table((planMySQL.AssessmentTaskPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", taskID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update assessment_task %d timestamps: %w", taskID, err)
	}
	return nil
}

func updatePlanFixupAssessment(
	ctx context.Context,
	mysqlDB *gorm.DB,
	assessmentRow planFixupAssessmentRow,
	times planFixupTimes,
	reportExists bool,
) error {
	assessmentStatus := normalizeTaskStatus(assessmentRow.Status)
	updates := map[string]interface{}{
		"created_at":   times.CompletionAt,
		"submitted_at": times.CompletionAt,
	}

	switch {
	case assessmentStatus == "failed":
		updates["failed_at"] = times.InterpretAt
		updates["interpreted_at"] = nil
		updates["updated_at"] = times.InterpretAt
	case assessmentStatus == "interpreted" || reportExists:
		updates["interpreted_at"] = times.InterpretAt
		updates["failed_at"] = nil
		updates["updated_at"] = times.InterpretAt
	default:
		updates["interpreted_at"] = nil
		updates["failed_at"] = nil
		updates["updated_at"] = times.CompletionAt
	}

	if err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", assessmentRow.ID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update assessment %d timestamps: %w", assessmentRow.ID, err)
	}
	return nil
}

func updatePlanFixupAnswerSheet(
	ctx context.Context,
	mongoDB *mongo.Database,
	answerSheetID uint64,
	filledAt time.Time,
	updatedAt time.Time,
) (bool, error) {
	result, err := mongoDB.Collection((answerSheetMongo.AnswerSheetPO{}).CollectionName()).UpdateOne(
		ctx,
		bson.M{"domain_id": answerSheetID, "deleted_at": nil},
		bson.M{"$set": bson.M{
			"filled_at":  filledAt,
			"created_at": filledAt,
			"updated_at": updatedAt,
		}},
	)
	if err != nil {
		return false, fmt.Errorf("update answersheet %d timestamps: %w", answerSheetID, err)
	}
	return result.MatchedCount > 0, nil
}

func updatePlanFixupReport(
	ctx context.Context,
	mongoDB *mongo.Database,
	assessmentID uint64,
	interpretAt time.Time,
) (bool, error) {
	result, err := mongoDB.Collection((evaluationMongo.InterpretReportPO{}).CollectionName()).UpdateOne(
		ctx,
		bson.M{"domain_id": assessmentID, "deleted_at": nil},
		bson.M{"$set": bson.M{
			"created_at": interpretAt,
			"updated_at": interpretAt,
		}},
	)
	if err != nil {
		return false, fmt.Errorf("update interpret report for assessment %d timestamps: %w", assessmentID, err)
	}
	return result.MatchedCount > 0, nil
}

func computePlanFixupTimes(plannedAt time.Time, openAt, expireAt *time.Time) planFixupTimes {
	ttl := planFixupDefaultExpireTTL
	if openAt != nil && expireAt != nil && expireAt.After(*openAt) {
		ttl = expireAt.Sub(*openAt)
	}
	completionAt := plannedAt.Add(planFixupCompletionOffset)
	return planFixupTimes{
		OpenAt:       plannedAt,
		ExpireAt:     plannedAt.Add(ttl),
		CompletionAt: completionAt,
		InterpretAt:  completionAt.Add(planFixupInterpretOffset),
		TTL:          ttl,
	}
}

func buildPlanFixupTaskPatch(status string, times planFixupTimes) planFixupTaskPatch {
	normalizedStatus := normalizeTaskStatus(status)
	patch := planFixupTaskPatch{
		OpenAt:    times.OpenAt,
		ExpireAt:  times.ExpireAt,
		UpdatedAt: times.OpenAt,
	}
	switch normalizedStatus {
	case "completed":
		completedAt := times.CompletionAt
		patch.CompletedAt = &completedAt
		patch.UpdatedAt = times.CompletionAt
	case "expired":
		patch.CompletedAt = nil
		patch.UpdatedAt = times.ExpireAt
	default:
		patch.CompletedAt = nil
		patch.UpdatedAt = times.OpenAt
	}
	return patch
}

func collectPlanFixupAssessmentIDs(tasks []planFixupTaskRow) []uint64 {
	seen := make(map[uint64]struct{}, len(tasks))
	ids := make([]uint64, 0, len(tasks))
	for _, task := range tasks {
		if task.AssessmentID == nil || *task.AssessmentID == 0 {
			continue
		}
		if _, ok := seen[*task.AssessmentID]; ok {
			continue
		}
		seen[*task.AssessmentID] = struct{}{}
		ids = append(ids, *task.AssessmentID)
	}
	return ids
}

func parsePlanFixupScopeTesteeIDs(rawIDs []string) ([]uint64, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}
	parsed := make([]uint64, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		rawID = strings.TrimSpace(rawID)
		if rawID == "" {
			continue
		}
		id := parseID(rawID)
		if id == 0 {
			return nil, fmt.Errorf("invalid plan fixup testee id: %s", rawID)
		}
		parsed = append(parsed, id)
	}
	return parsed, nil
}
