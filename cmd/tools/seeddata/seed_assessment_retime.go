package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	answerSheetMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	evaluationMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	statisticsMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

const assessmentRetimeBatchSize = 500

type assessmentRetimeFilter struct {
	ScopeTesteeIDs []uint64
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	Offset         time.Duration
	Limit          int
	AllowAll       bool
	DryRun         bool
	Verbose        bool
}

type assessmentRetimeRow struct {
	AssessmentID    uint64     `gorm:"column:assessment_id"`
	OrgID           int64      `gorm:"column:org_id"`
	TesteeID        uint64     `gorm:"column:testee_id"`
	TesteeCreatedAt time.Time  `gorm:"column:testee_created_at"`
	AnswerSheetID   uint64     `gorm:"column:answer_sheet_id"`
	Status          string     `gorm:"column:status"`
	AssessmentAt    time.Time  `gorm:"column:assessment_created_at"`
	SubmittedAt     *time.Time `gorm:"column:submitted_at"`
	InterpretedAt   *time.Time `gorm:"column:interpreted_at"`
	FailedAt        *time.Time `gorm:"column:failed_at"`
	EpisodeID       *uint64    `gorm:"column:episode_id"`
	EntryID         *uint64    `gorm:"column:entry_id"`
	ClinicianID     *uint64    `gorm:"column:clinician_id"`
	EpisodeReportID *uint64    `gorm:"column:report_id"`
	FailureReason   *string    `gorm:"column:failure_reason"`
}

type assessmentRetimeTimes struct {
	SubmittedAt time.Time
	TerminalAt  time.Time
}

type assessmentRetimeStats struct {
	AssessmentsProcessed               int
	AssessmentsUpdated                 int
	EpisodesUpdated                    int
	AnswerSheetsUpdated                int
	ReportsUpdated                     int
	AnswerSheetFootprintsUpdated       int64
	AssessmentCreatedFootprintsUpdated int64
	ReportGeneratedFootprintsUpdated   int64
	MissingEpisodes                    int
	MissingAnswerSheets                int
	MissingReports                     int
	MissingAnswerSheetFootprints       int
	MissingAssessmentCreatedFootprints int
	MissingReportGeneratedFootprints   int
	TouchedTestees                     int
}

type assessmentRetimeTesteeSummary struct {
	TotalAssessments int64      `gorm:"column:total_assessments"`
	LastAssessmentAt *time.Time `gorm:"column:last_assessment_at"`
}

type assessmentRetimeLatestRisk struct {
	RiskLevel *string `gorm:"column:risk_level"`
}

func seedAssessmentRetimeTimestamps(ctx context.Context, deps *dependencies, opts assessmentRetimeOptions) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for assessment retime")
	}
	cfg := deps.Config.Local
	if strings.TrimSpace(cfg.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for assessment retime")
	}
	if strings.TrimSpace(cfg.MongoURI) == "" {
		return fmt.Errorf("seeddata local.mongo_uri is required for assessment retime")
	}
	if strings.TrimSpace(cfg.MongoDatabase) == "" {
		return fmt.Errorf("seeddata local.mongo_database is required for assessment retime")
	}

	filter, err := normalizeAssessmentRetimeOptions(opts)
	if err != nil {
		return err
	}

	mysqlDB, err := openLocalSeedMySQL(cfg.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after assessment retime", "error", closeErr.Error())
		}
	}()

	mongoClient, mongoDB, err := openLocalSeedMongo(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := mongoClient.Disconnect(context.Background()); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mongo after assessment retime", "error", closeErr.Error())
		}
	}()

	totalMatches, err := countAssessmentRetimeRows(ctx, mysqlDB, deps.Config.Global.OrgID, filter)
	if err != nil {
		return err
	}
	if filter.Limit > 0 && totalMatches > filter.Limit {
		totalMatches = filter.Limit
	}

	deps.Logger.Infow("Assessment retime started",
		"org_id", deps.Config.Global.OrgID,
		"scope_testee_count", len(filter.ScopeTesteeIDs),
		"created_after", filter.CreatedAfter,
		"created_before", filter.CreatedBefore,
		"offset", filter.Offset.String(),
		"limit", filter.Limit,
		"dry_run", filter.DryRun,
		"matched_assessments", totalMatches,
	)
	if totalMatches == 0 {
		return nil
	}

	totalBatches := (totalMatches + assessmentRetimeBatchSize - 1) / assessmentRetimeBatchSize
	batchProgress := newSeedProgressBar("assessment_retime batches", totalBatches)
	defer batchProgress.Close()
	assessmentProgress := newSeedProgressBar("assessment_retime assessments", totalMatches)
	defer assessmentProgress.Close()

	stats := &assessmentRetimeStats{}
	touchedTestees := make(map[uint64]struct{})
	var lastAssessmentID uint64
	remaining := totalMatches

	for remaining > 0 {
		batchLimit := assessmentRetimeBatchSize
		if remaining < batchLimit {
			batchLimit = remaining
		}

		rows, err := loadAssessmentRetimeBatch(ctx, mysqlDB, deps.Config.Global.OrgID, filter, lastAssessmentID, batchLimit)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		lastAssessmentID = rows[len(rows)-1].AssessmentID

		for _, row := range rows {
			stats.AssessmentsProcessed++
			touchedTestees[row.TesteeID] = struct{}{}
			if !filter.DryRun {
				if err := applyAssessmentRetime(ctx, mysqlDB, mongoDB, row, filter.Offset, stats, filter.Verbose, deps.Logger); err != nil {
					return err
				}
			} else if filter.Verbose {
				times := computeAssessmentRetimeTimes(row.TesteeCreatedAt, filter.Offset)
				deps.Logger.Infow("Assessment retime dry-run match",
					"assessment_id", row.AssessmentID,
					"testee_id", row.TesteeID,
					"current_created_at", row.AssessmentAt,
					"target_submitted_at", times.SubmittedAt,
					"target_terminal_at", times.TerminalAt,
					"status", row.Status,
				)
			}
			assessmentProgress.Increment()
			remaining--
		}
		batchProgress.Increment()
	}

	stats.TouchedTestees = len(touchedTestees)
	if !filter.DryRun {
		if err := rebuildAssessmentRetimeTesteeStats(ctx, mysqlDB, deps.Config.Global.OrgID, mapKeysUint64(touchedTestees), filter.Verbose, deps.Logger); err != nil {
			return err
		}
	}

	batchProgress.Complete()
	assessmentProgress.Complete()
	deps.Logger.Infow("Assessment retime completed",
		"org_id", deps.Config.Global.OrgID,
		"dry_run", filter.DryRun,
		"assessments_processed", stats.AssessmentsProcessed,
		"assessments_updated", stats.AssessmentsUpdated,
		"episodes_updated", stats.EpisodesUpdated,
		"answersheets_updated", stats.AnswerSheetsUpdated,
		"reports_updated", stats.ReportsUpdated,
		"answersheet_footprints_updated", stats.AnswerSheetFootprintsUpdated,
		"assessment_created_footprints_updated", stats.AssessmentCreatedFootprintsUpdated,
		"report_generated_footprints_updated", stats.ReportGeneratedFootprintsUpdated,
		"missing_episodes", stats.MissingEpisodes,
		"missing_answersheets", stats.MissingAnswerSheets,
		"missing_reports", stats.MissingReports,
		"missing_answersheet_footprints", stats.MissingAnswerSheetFootprints,
		"missing_assessment_created_footprints", stats.MissingAssessmentCreatedFootprints,
		"missing_report_generated_footprints", stats.MissingReportGeneratedFootprints,
		"touched_testees", stats.TouchedTestees,
		"next_step", "statistics_backfill",
	)
	return nil
}

func normalizeAssessmentRetimeOptions(opts assessmentRetimeOptions) (assessmentRetimeFilter, error) {
	scopeTesteeIDs, err := parsePlanFixupScopeTesteeIDs(opts.ScopeTesteeIDs)
	if err != nil {
		return assessmentRetimeFilter{}, err
	}

	offsetRaw := strings.TrimSpace(opts.Offset)
	if offsetRaw == "" {
		offsetRaw = "30d"
	}
	offset, err := parseSeedRelativeDuration(offsetRaw)
	if err != nil {
		return assessmentRetimeFilter{}, fmt.Errorf("invalid assessment retime offset %q: %w", offsetRaw, err)
	}
	if offset <= 0 {
		return assessmentRetimeFilter{}, fmt.Errorf("assessment retime offset must be greater than 0")
	}

	var createdAfter *time.Time
	if raw := strings.TrimSpace(opts.CreatedAfter); raw != "" {
		parsed, err := parseFlexibleSeedTime(raw)
		if err != nil {
			return assessmentRetimeFilter{}, fmt.Errorf("invalid assessment-retime-created-after %q: %w", raw, err)
		}
		parsed = parsed.Round(0)
		createdAfter = &parsed
	}

	var createdBefore *time.Time
	if raw := strings.TrimSpace(opts.CreatedBefore); raw != "" {
		parsed, err := parseFlexibleSeedTime(raw)
		if err != nil {
			return assessmentRetimeFilter{}, fmt.Errorf("invalid assessment-retime-created-before %q: %w", raw, err)
		}
		parsed = parsed.Round(0)
		createdBefore = &parsed
	}

	if createdAfter != nil && createdBefore != nil && createdAfter.After(*createdBefore) {
		return assessmentRetimeFilter{}, fmt.Errorf("assessment-retime-created-after must be before assessment-retime-created-before")
	}
	if !opts.AllowAll && len(scopeTesteeIDs) == 0 && createdAfter == nil && createdBefore == nil {
		return assessmentRetimeFilter{}, fmt.Errorf("assessment retime requires a scope filter; set --assessment-retime-created-after/--assessment-retime-created-before, --assessment-retime-testee-ids, or pass --assessment-retime-all")
	}
	if opts.Limit < 0 {
		return assessmentRetimeFilter{}, fmt.Errorf("assessment retime limit must be greater than or equal to 0")
	}

	return assessmentRetimeFilter{
		ScopeTesteeIDs: scopeTesteeIDs,
		CreatedAfter:   createdAfter,
		CreatedBefore:  createdBefore,
		Offset:         offset,
		Limit:          opts.Limit,
		AllowAll:       opts.AllowAll,
		DryRun:         opts.DryRun,
		Verbose:        opts.Verbose,
	}, nil
}

func countAssessmentRetimeRows(ctx context.Context, mysqlDB *gorm.DB, orgID int64, filter assessmentRetimeFilter) (int, error) {
	query := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()+" AS a").
		Joins("JOIN "+(actorMySQL.TesteePO{}).TableName()+" AS t ON t.id = a.testee_id AND t.deleted_at IS NULL").
		Where("a.org_id = ? AND a.deleted_at IS NULL", orgID)

	query = applyAssessmentRetimeFilters(query, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, fmt.Errorf("count assessment retime rows: %w", err)
	}
	if total <= 0 {
		return 0, nil
	}
	return int(total), nil
}

func loadAssessmentRetimeBatch(
	ctx context.Context,
	mysqlDB *gorm.DB,
	orgID int64,
	filter assessmentRetimeFilter,
	lastAssessmentID uint64,
	limit int,
) ([]assessmentRetimeRow, error) {
	query := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()+" AS a").
		Select(strings.Join([]string{
			"a.id AS assessment_id",
			"a.org_id AS org_id",
			"a.testee_id AS testee_id",
			"t.created_at AS testee_created_at",
			"a.answer_sheet_id AS answer_sheet_id",
			"a.status AS status",
			"a.created_at AS assessment_created_at",
			"a.submitted_at AS submitted_at",
			"a.interpreted_at AS interpreted_at",
			"a.failed_at AS failed_at",
			"a.failure_reason AS failure_reason",
			"e.episode_id AS episode_id",
			"e.entry_id AS entry_id",
			"e.clinician_id AS clinician_id",
			"e.report_id AS report_id",
		}, ", ")).
		Joins("JOIN "+(actorMySQL.TesteePO{}).TableName()+" AS t ON t.id = a.testee_id AND t.deleted_at IS NULL").
		Joins("LEFT JOIN "+(statisticsMySQL.AssessmentEpisodePO{}).TableName()+" AS e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL").
		Where("a.org_id = ? AND a.deleted_at IS NULL AND a.id > ?", orgID, lastAssessmentID).
		Order("a.id ASC").
		Limit(limit)

	query = applyAssessmentRetimeFilters(query, filter)

	var rows []assessmentRetimeRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load assessment retime batch: %w", err)
	}
	return rows, nil
}

func applyAssessmentRetimeFilters(query *gorm.DB, filter assessmentRetimeFilter) *gorm.DB {
	if len(filter.ScopeTesteeIDs) > 0 {
		query = query.Where("a.testee_id IN ?", filter.ScopeTesteeIDs)
	}
	if filter.CreatedAfter != nil {
		query = query.Where("a.created_at >= ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query = query.Where("a.created_at <= ?", *filter.CreatedBefore)
	}
	return query
}

func applyAssessmentRetime(
	ctx context.Context,
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	row assessmentRetimeRow,
	offset time.Duration,
	stats *assessmentRetimeStats,
	verbose bool,
	logger interface {
		Infow(string, ...interface{})
		Warnw(string, ...interface{})
	},
) error {
	times := computeAssessmentRetimeTimes(row.TesteeCreatedAt, offset)
	reportID, reportExpected := resolveAssessmentRetimeReportID(row)
	hasSubmittedAt := row.SubmittedAt != nil || row.InterpretedAt != nil || row.FailedAt != nil
	hasFailedAt := row.FailedAt != nil || normalizeAssessmentRetimeStatus(row.Status) == "failed"
	hasReportAt := !hasFailedAt && (row.InterpretedAt != nil || reportExpected || normalizeAssessmentRetimeStatus(row.Status) == "interpreted")

	if err := mysqlDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := updateAssessmentRetimeAssessment(ctx, tx, row, times, hasSubmittedAt, hasReportAt, hasFailedAt); err != nil {
			return err
		}
		stats.AssessmentsUpdated++

		episodeUpdated, err := updateAssessmentRetimeEpisode(ctx, tx, row, times, reportID, hasReportAt, hasFailedAt)
		if err != nil {
			return err
		}
		if episodeUpdated {
			stats.EpisodesUpdated++
		} else {
			stats.MissingEpisodes++
		}

		updated, err := updateAssessmentRetimeBehaviorFootprint(ctx, tx, row.OrgID, statisticsDomain.BehaviorEventAnswerSheetSubmitted, "answersheet_id = ?", row.AnswerSheetID, times.SubmittedAt)
		if err != nil {
			return err
		}
		if updated > 0 {
			stats.AnswerSheetFootprintsUpdated += updated
		} else {
			stats.MissingAnswerSheetFootprints++
		}

		updated, err = updateAssessmentRetimeBehaviorFootprint(ctx, tx, row.OrgID, statisticsDomain.BehaviorEventAssessmentCreated, "assessment_id = ?", row.AssessmentID, times.SubmittedAt)
		if err != nil {
			return err
		}
		if updated > 0 {
			stats.AssessmentCreatedFootprintsUpdated += updated
		} else {
			stats.MissingAssessmentCreatedFootprints++
		}

		if hasReportAt {
			updated, err = updateAssessmentRetimeBehaviorFootprint(ctx, tx, row.OrgID, statisticsDomain.BehaviorEventReportGenerated, "assessment_id = ?", row.AssessmentID, times.TerminalAt)
			if err != nil {
				return err
			}
			if updated == 0 && reportID > 0 && reportID != row.AssessmentID {
				updated, err = updateAssessmentRetimeBehaviorFootprint(ctx, tx, row.OrgID, statisticsDomain.BehaviorEventReportGenerated, "report_id = ?", reportID, times.TerminalAt)
				if err != nil {
					return err
				}
			}
			if updated > 0 {
				stats.ReportGeneratedFootprintsUpdated += updated
			} else {
				stats.MissingReportGeneratedFootprints++
			}
		}

		return nil
	}); err != nil {
		return err
	}

	answerSheetUpdatedAt := times.SubmittedAt
	if hasReportAt || hasFailedAt {
		answerSheetUpdatedAt = times.TerminalAt
	}
	answerSheetUpdated, err := updateAssessmentRetimeAnswerSheet(ctx, mongoDB, row.AnswerSheetID, times.SubmittedAt, answerSheetUpdatedAt)
	if err != nil {
		return err
	}
	if answerSheetUpdated {
		stats.AnswerSheetsUpdated++
	} else {
		stats.MissingAnswerSheets++
	}

	if hasReportAt {
		reportUpdated, err := updateAssessmentRetimeReport(ctx, mongoDB, reportID, times.TerminalAt)
		if err != nil {
			return err
		}
		if reportUpdated {
			stats.ReportsUpdated++
		} else {
			stats.MissingReports++
		}
	}

	if verbose {
		logger.Infow("Assessment chain retimed",
			"assessment_id", row.AssessmentID,
			"testee_id", row.TesteeID,
			"answersheet_id", row.AnswerSheetID,
			"report_id", reportID,
			"status", row.Status,
			"target_submitted_at", times.SubmittedAt,
			"target_terminal_at", times.TerminalAt,
		)
	}
	return nil
}

func computeAssessmentRetimeTimes(testeeCreatedAt time.Time, offset time.Duration) assessmentRetimeTimes {
	submittedAt := testeeCreatedAt.Round(0).Add(offset)
	return assessmentRetimeTimes{
		SubmittedAt: submittedAt,
		TerminalAt:  deriveAssessmentInterpretAt(submittedAt),
	}
}

func resolveAssessmentRetimeReportID(row assessmentRetimeRow) (uint64, bool) {
	if row.EpisodeReportID != nil && *row.EpisodeReportID != 0 {
		return *row.EpisodeReportID, true
	}
	switch normalizeAssessmentRetimeStatus(row.Status) {
	case "interpreted":
		return row.AssessmentID, true
	case "failed":
		return 0, false
	}
	if row.InterpretedAt != nil {
		return row.AssessmentID, true
	}
	return 0, false
}

func normalizeAssessmentRetimeStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func updateAssessmentRetimeAssessment(
	ctx context.Context,
	mysqlDB *gorm.DB,
	row assessmentRetimeRow,
	times assessmentRetimeTimes,
	hasSubmittedAt bool,
	hasReportAt bool,
	hasFailedAt bool,
) error {
	updates := map[string]interface{}{
		"created_at": times.SubmittedAt,
	}
	if hasSubmittedAt {
		updates["submitted_at"] = times.SubmittedAt
	} else {
		updates["submitted_at"] = nil
	}
	if hasReportAt {
		updates["interpreted_at"] = times.TerminalAt
		updates["failed_at"] = nil
		updates["updated_at"] = times.TerminalAt
	} else if hasFailedAt {
		updates["interpreted_at"] = nil
		updates["failed_at"] = times.TerminalAt
		updates["updated_at"] = times.TerminalAt
	} else {
		updates["interpreted_at"] = nil
		updates["failed_at"] = nil
		updates["updated_at"] = times.SubmittedAt
	}

	if err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Where("id = ? AND org_id = ? AND deleted_at IS NULL", row.AssessmentID, row.OrgID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update assessment %d timestamps: %w", row.AssessmentID, err)
	}
	return nil
}

func updateAssessmentRetimeEpisode(
	ctx context.Context,
	mysqlDB *gorm.DB,
	row assessmentRetimeRow,
	times assessmentRetimeTimes,
	reportID uint64,
	hasReportAt bool,
	hasFailedAt bool,
) (bool, error) {
	if row.EpisodeID == nil || *row.EpisodeID == 0 {
		return false, nil
	}

	updates := map[string]interface{}{
		"assessment_id":         row.AssessmentID,
		"submitted_at":          times.SubmittedAt,
		"assessment_created_at": times.SubmittedAt,
		"created_at":            times.SubmittedAt,
	}
	if hasReportAt {
		updates["report_generated_at"] = times.TerminalAt
		updates["failed_at"] = nil
		updates["report_id"] = reportID
		updates["status"] = string(statisticsDomain.EpisodeStatusCompleted)
		updates["updated_at"] = times.TerminalAt
	} else if hasFailedAt {
		updates["report_generated_at"] = nil
		updates["failed_at"] = times.TerminalAt
		updates["status"] = string(statisticsDomain.EpisodeStatusFailed)
		updates["updated_at"] = times.TerminalAt
	} else {
		updates["report_generated_at"] = nil
		updates["failed_at"] = nil
		updates["status"] = string(statisticsDomain.EpisodeStatusActive)
		updates["updated_at"] = times.SubmittedAt
	}

	result := mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AssessmentEpisodePO{}).TableName()).
		Where("episode_id = ? AND deleted_at IS NULL", *row.EpisodeID).
		Updates(updates)
	if result.Error != nil {
		return false, fmt.Errorf("update assessment_episode %d timestamps: %w", *row.EpisodeID, result.Error)
	}
	return result.RowsAffected > 0, nil
}

func updateAssessmentRetimeBehaviorFootprint(
	ctx context.Context,
	mysqlDB *gorm.DB,
	orgID int64,
	eventName statisticsDomain.BehaviorEventName,
	condition string,
	args interface{},
	occurredAt time.Time,
) (int64, error) {
	updates := map[string]interface{}{
		"occurred_at": occurredAt,
		"created_at":  occurredAt,
		"updated_at":  occurredAt,
	}

	query := mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.BehaviorFootprintPO{}).TableName()).
		Where("org_id = ? AND deleted_at IS NULL AND event_name = ?", orgID, string(eventName))

	switch typed := args.(type) {
	case []interface{}:
		query = query.Where(condition, typed...)
	default:
		query = query.Where(condition, typed)
	}

	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update behavior_footprint %s timestamps: %w", eventName, result.Error)
	}
	return result.RowsAffected, nil
}

func updateAssessmentRetimeAnswerSheet(
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

func updateAssessmentRetimeReport(
	ctx context.Context,
	mongoDB *mongo.Database,
	reportID uint64,
	reportAt time.Time,
) (bool, error) {
	if reportID == 0 {
		return false, nil
	}
	result, err := mongoDB.Collection((evaluationMongo.InterpretReportPO{}).CollectionName()).UpdateOne(
		ctx,
		bson.M{"domain_id": reportID, "deleted_at": nil},
		bson.M{"$set": bson.M{
			"created_at": reportAt,
			"updated_at": reportAt,
		}},
	)
	if err != nil {
		return false, fmt.Errorf("update interpret report %d timestamps: %w", reportID, err)
	}
	return result.MatchedCount > 0, nil
}

func rebuildAssessmentRetimeTesteeStats(
	ctx context.Context,
	mysqlDB *gorm.DB,
	orgID int64,
	testeeIDs []uint64,
	verbose bool,
	logger interface {
		Infow(string, ...interface{})
		Warnw(string, ...interface{})
	},
) error {
	if len(testeeIDs) == 0 {
		return nil
	}

	progress := newSeedProgressBar("assessment_retime testee_stats", len(testeeIDs))
	defer progress.Close()

	for _, testeeID := range testeeIDs {
		summary, err := loadAssessmentRetimeTesteeSummary(ctx, mysqlDB, orgID, testeeID)
		if err != nil {
			return err
		}
		lastRisk, err := loadAssessmentRetimeLatestRisk(ctx, mysqlDB, orgID, testeeID)
		if err != nil {
			return err
		}

		updates := map[string]interface{}{
			"total_assessments":  summary.TotalAssessments,
			"last_assessment_at": nil,
			"last_risk_level":    nil,
		}
		if summary.LastAssessmentAt != nil {
			updates["last_assessment_at"] = *summary.LastAssessmentAt
			updates["last_risk_level"] = lastRisk
		}

		if err := mysqlDB.WithContext(ctx).
			Table((actorMySQL.TesteePO{}).TableName()).
			Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, testeeID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update testee %d assessment stats: %w", testeeID, err)
		}
		if verbose {
			logger.Infow("Rebuilt testee assessment stats",
				"testee_id", testeeID,
				"total_assessments", summary.TotalAssessments,
				"last_assessment_at", summary.LastAssessmentAt,
				"last_risk_level", lastRisk,
			)
		}
		progress.Increment()
	}

	progress.Complete()
	return nil
}

func loadAssessmentRetimeTesteeSummary(ctx context.Context, mysqlDB *gorm.DB, orgID int64, testeeID uint64) (assessmentRetimeTesteeSummary, error) {
	var result assessmentRetimeTesteeSummary
	if err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Select("COUNT(*) AS total_assessments, MAX(created_at) AS last_assessment_at").
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Scan(&result).Error; err != nil {
		return assessmentRetimeTesteeSummary{}, fmt.Errorf("load assessment summary for testee %d: %w", testeeID, err)
	}
	return result, nil
}

func loadAssessmentRetimeLatestRisk(ctx context.Context, mysqlDB *gorm.DB, orgID int64, testeeID uint64) (*string, error) {
	var result assessmentRetimeLatestRisk
	if err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Select("risk_level").
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Order("created_at DESC, id DESC").
		Limit(1).
		Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("load latest risk for testee %d: %w", testeeID, err)
	}
	return result.RiskLevel, nil
}

func mapKeysUint64(items map[uint64]struct{}) []uint64 {
	if len(items) == 0 {
		return nil
	}
	result := make([]uint64, 0, len(items))
	for id := range items {
		result = append(result, id)
	}
	return result
}
