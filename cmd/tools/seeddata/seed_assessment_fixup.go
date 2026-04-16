package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	answerSheetMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	evaluationMongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	planMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	statisticsMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

const (
	assessmentFixupStandaloneInitialOffset = 48 * time.Hour
	assessmentFixupStandaloneMinGap        = 14 * 24 * time.Hour
	assessmentFixupStandaloneGapJitter     = 42 * 24 * time.Hour
	assessmentFixupStandaloneWindow        = 30 * 24 * time.Hour
)

type assessmentFixupEntryRow struct {
	ID            uint64     `gorm:"column:id"`
	ClinicianID   uint64     `gorm:"column:clinician_id"`
	TargetType    string     `gorm:"column:target_type"`
	TargetCode    string     `gorm:"column:target_code"`
	TargetVersion *string    `gorm:"column:target_version"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	ExpiresAt     *time.Time `gorm:"column:expires_at"`
}

type assessmentFixupEntryRelationRow struct {
	ID              uint64    `gorm:"column:id"`
	ClinicianID     uint64    `gorm:"column:clinician_id"`
	TesteeID        uint64    `gorm:"column:testee_id"`
	EntryID         uint64    `gorm:"column:entry_id"`
	RelationType    string    `gorm:"column:relation_type"`
	BoundAt         time.Time `gorm:"column:bound_at"`
	TesteeCreatedAt time.Time `gorm:"column:testee_created_at"`
}

type assessmentFixupResolveLogRow struct {
	ID          uint64    `gorm:"column:id"`
	EntryID     uint64    `gorm:"column:entry_id"`
	ClinicianID uint64    `gorm:"column:clinician_id"`
	ResolvedAt  time.Time `gorm:"column:resolved_at"`
}

type assessmentFixupAssessmentRow struct {
	ID                   uint64     `gorm:"column:id"`
	TesteeID             uint64     `gorm:"column:testee_id"`
	AnswerSheetID        uint64     `gorm:"column:answer_sheet_id"`
	QuestionnaireCode    string     `gorm:"column:questionnaire_code"`
	QuestionnaireVersion string     `gorm:"column:questionnaire_version"`
	MedicalScaleCode     *string    `gorm:"column:medical_scale_code"`
	Status               string     `gorm:"column:status"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at"`
	SubmittedAt          *time.Time `gorm:"column:submitted_at"`
	InterpretedAt        *time.Time `gorm:"column:interpreted_at"`
	FailedAt             *time.Time `gorm:"column:failed_at"`
	TesteeCreatedAt      time.Time  `gorm:"column:testee_created_at"`
}

type assessmentFixupEntryAssessmentMatch struct {
	Relation    assessmentFixupEntryRelationRow
	Entry       assessmentFixupEntryRow
	ResolveAt   time.Time
	IntakeAt    time.Time
	SubmitAt    time.Time
	InterpretAt time.Time
	Assessment  assessmentFixupAssessmentRow
}

type assessmentFixupStats struct {
	EntriesProcessed               int
	EntriesUpdated                 int
	EntriesSkipped                 int
	EntriesMissingAnchor           int
	EntryCreatorRelationsProcessed int
	EntryCreatorRelationsUpdated   int
	EntryAccessRelationsUpdated    int
	EntryResolveLogsUpdated        int
	EntryResolveLogMismatches      int
	EntryAssessmentsMatched        int
	EntryAssessmentsUpdated        int
	EntryAssessmentsUnmatched      int
	StandaloneAssessmentsUpdated   int
	AssessmentsUpdated             int
	AnswerSheetsUpdated            int
	ReportsUpdated                 int
	MissingAnswerSheets            int
	MissingReports                 int
}

type assessmentFixupInterpretedAtScope struct {
	From        *time.Time
	To          *time.Time
	ToExclusive bool
}

func seedAssessmentEntryFixupTimestamps(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	orgID, mysqlDB, mongoDB, closeFn, err := openAssessmentFixupStores(ctx, deps, "assessment_entry_fixup_timestamps")
	if err != nil {
		return err
	}
	defer closeFn()

	anchors, err := loadClinicianAssessmentEntryAnchors(ctx, mysqlDB, orgID)
	if err != nil {
		return err
	}
	entries, err := loadAssessmentEntriesForFixup(ctx, mysqlDB, orgID)
	if err != nil {
		return err
	}
	relations, err := loadAssessmentEntryRelationsForFixup(ctx, mysqlDB, orgID)
	if err != nil {
		return err
	}
	resolveLogs, err := loadAssessmentEntryResolveLogsForFixup(ctx, mysqlDB, orgID)
	if err != nil {
		return err
	}
	nonPlanAssessments, err := loadNonPlanAssessmentsForFixup(ctx, mysqlDB, orgID, assessmentFixupInterpretedAtScope{})
	if err != nil {
		return err
	}

	deps.Logger.Infow("Assessment entry timestamp fixup started",
		"org_id", orgID,
		"entry_count", len(entries),
		"entry_relation_count", len(relations),
		"entry_resolve_log_count", len(resolveLogs),
		"candidate_assessment_count", len(nonPlanAssessments),
	)

	stats := &assessmentFixupStats{}

	entryProgress := newSeedProgressBar("assessment_entry_fixup entries", len(entries))
	defer entryProgress.Close()
	entryMap, creatorPlans, err := fixAssessmentEntryChain(ctx, mysqlDB, anchors, entries, relations, resolveLogs, stats, entryProgress)
	if err != nil {
		return err
	}
	entryProgress.Complete()

	entryAssessmentProgress := newSeedProgressBar("assessment_entry_fixup assessments", len(creatorPlans))
	defer entryAssessmentProgress.Close()
	_, err = fixEntryBasedAssessments(ctx, mysqlDB, mongoDB, creatorPlans, nonPlanAssessments, stats, entryAssessmentProgress)
	if err != nil {
		return err
	}
	entryAssessmentProgress.Complete()

	deps.Logger.Infow("Assessment entry timestamp fixup completed",
		"org_id", orgID,
		"entry_count", len(entryMap),
		"entries_processed", stats.EntriesProcessed,
		"entries_updated", stats.EntriesUpdated,
		"entries_skipped", stats.EntriesSkipped,
		"entries_missing_anchor", stats.EntriesMissingAnchor,
		"entry_creator_relations_processed", stats.EntryCreatorRelationsProcessed,
		"entry_creator_relations_updated", stats.EntryCreatorRelationsUpdated,
		"entry_access_relations_updated", stats.EntryAccessRelationsUpdated,
		"entry_resolve_logs_updated", stats.EntryResolveLogsUpdated,
		"entry_resolve_log_mismatches", stats.EntryResolveLogMismatches,
		"entry_assessments_matched", stats.EntryAssessmentsMatched,
		"entry_assessments_updated", stats.EntryAssessmentsUpdated,
		"entry_assessments_unmatched", stats.EntryAssessmentsUnmatched,
		"assessments_updated", stats.AssessmentsUpdated,
		"answersheets_updated", stats.AnswerSheetsUpdated,
		"reports_updated", stats.ReportsUpdated,
		"missing_answersheets", stats.MissingAnswerSheets,
		"missing_reports", stats.MissingReports,
	)
	return nil
}

func seedAssessmentFixupTimestamps(ctx context.Context, deps *dependencies, opts assessmentFixupOptions) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	orgID, mysqlDB, mongoDB, closeFn, err := openAssessmentFixupStores(ctx, deps, "assessment_fixup_timestamps")
	if err != nil {
		return err
	}
	defer closeFn()

	nonPlanAssessments, err := loadNonPlanAssessmentsForFixup(ctx, mysqlDB, orgID, opts.InterpretedAtScope)
	if err != nil {
		return err
	}
	entries, err := loadAssessmentEntriesForFixup(ctx, mysqlDB, orgID)
	if err != nil {
		return err
	}
	relations, err := loadAssessmentEntryRelationsForFixup(ctx, mysqlDB, orgID)
	if err != nil {
		return err
	}

	deps.Logger.Infow("Assessment timestamp fixup started",
		"org_id", orgID,
		"non_plan_assessment_count", len(nonPlanAssessments),
		"entry_count", len(entries),
		"entry_relation_count", len(relations),
		"interpreted_at_from", formatAssessmentFixupScopeTime(opts.InterpretedAtScope.From),
		"interpreted_at_to", formatAssessmentFixupScopeTime(opts.InterpretedAtScope.To),
		"interpreted_at_to_exclusive", opts.InterpretedAtScope.ToExclusive,
	)

	stats := &assessmentFixupStats{}
	entryMap := make(map[uint64]assessmentFixupEntryRow, len(entries))
	for _, row := range entries {
		entryMap[row.ID] = row
	}
	creatorPlans := buildEntryAssessmentPlansWithoutMutation(entryMap, relations)
	matchedAssessmentIDs, err := collectMatchedEntryAssessmentIDs(creatorPlans, nonPlanAssessments, stats)
	if err != nil {
		return err
	}

	standaloneAssessments := collectStandaloneAssessments(nonPlanAssessments, matchedAssessmentIDs)
	standaloneProgress := newSeedProgressBar("assessment_fixup adhoc_assessments", len(standaloneAssessments))
	defer standaloneProgress.Close()
	if err := fixStandaloneAssessments(ctx, mysqlDB, mongoDB, standaloneAssessments, stats, standaloneProgress); err != nil {
		return err
	}
	standaloneProgress.Complete()

	deps.Logger.Infow("Assessment timestamp fixup completed",
		"org_id", orgID,
		"non_plan_assessment_count", len(nonPlanAssessments),
		"excluded_entry_assessments", len(matchedAssessmentIDs),
		"standalone_assessment_count", len(standaloneAssessments),
		"standalone_assessments_updated", stats.StandaloneAssessmentsUpdated,
		"interpreted_at_from", formatAssessmentFixupScopeTime(opts.InterpretedAtScope.From),
		"interpreted_at_to", formatAssessmentFixupScopeTime(opts.InterpretedAtScope.To),
		"interpreted_at_to_exclusive", opts.InterpretedAtScope.ToExclusive,
		"assessments_updated", stats.AssessmentsUpdated,
		"answersheets_updated", stats.AnswerSheetsUpdated,
		"reports_updated", stats.ReportsUpdated,
		"missing_answersheets", stats.MissingAnswerSheets,
		"missing_reports", stats.MissingReports,
	)
	return nil
}

func loadAssessmentEntriesForFixup(ctx context.Context, mysqlDB *gorm.DB, orgID int64) ([]assessmentFixupEntryRow, error) {
	rows := make([]assessmentFixupEntryRow, 0, 256)
	if err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.AssessmentEntryPO{}).TableName()).
		Select("id, clinician_id, target_type, target_code, target_version, created_at, expires_at").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("clinician_id ASC, created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load assessment entries for fixup: %w", err)
	}
	return rows, nil
}

func loadAssessmentEntryRelationsForFixup(ctx context.Context, mysqlDB *gorm.DB, orgID int64) ([]assessmentFixupEntryRelationRow, error) {
	rows := make([]assessmentFixupEntryRelationRow, 0, 512)
	if err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()+" AS cr").
		Select("cr.id, cr.clinician_id, cr.testee_id, cr.source_id AS entry_id, cr.relation_type, cr.bound_at, t.created_at AS testee_created_at").
		Joins("JOIN "+(actorMySQL.TesteePO{}).TableName()+" AS t ON t.id = cr.testee_id AND t.deleted_at IS NULL").
		Where("cr.org_id = ? AND cr.source_type = ? AND cr.is_active = 1 AND cr.deleted_at IS NULL AND cr.source_id IS NOT NULL", orgID, "assessment_entry").
		Order("cr.source_id ASC, t.created_at ASC, cr.testee_id ASC, cr.id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load assessment_entry relations for fixup: %w", err)
	}
	return rows, nil
}

func loadAssessmentEntryResolveLogsForFixup(ctx context.Context, mysqlDB *gorm.DB, orgID int64) ([]assessmentFixupResolveLogRow, error) {
	rows := make([]assessmentFixupResolveLogRow, 0, 256)
	if err := mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AssessmentEntryResolveLogPO{}).TableName()).
		Select("id, entry_id, clinician_id, resolved_at").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Order("entry_id ASC, clinician_id ASC, resolved_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load assessment_entry resolve logs for fixup: %w", err)
	}
	return rows, nil
}

func loadNonPlanAssessmentsForFixup(
	ctx context.Context,
	mysqlDB *gorm.DB,
	orgID int64,
	scope assessmentFixupInterpretedAtScope,
) ([]assessmentFixupAssessmentRow, error) {
	rows := make([]assessmentFixupAssessmentRow, 0, 1024)
	db := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()+" AS a").
		Select("a.id, a.testee_id, a.answer_sheet_id, a.questionnaire_code, a.questionnaire_version, a.medical_scale_code, a.status, a.created_at, a.updated_at, a.submitted_at, a.interpreted_at, a.failed_at, t.created_at AS testee_created_at").
		Joins("JOIN "+(actorMySQL.TesteePO{}).TableName()+" AS t ON t.id = a.testee_id AND t.deleted_at IS NULL").
		Joins("LEFT JOIN "+(planMySQL.AssessmentTaskPO{}).TableName()+" AS task ON task.assessment_id = a.id AND task.deleted_at IS NULL").
		Where("a.org_id = ? AND a.deleted_at IS NULL AND task.id IS NULL", orgID)
	if scope.From != nil {
		db = db.Where("a.interpreted_at >= ?", *scope.From)
	}
	if scope.To != nil {
		if scope.ToExclusive {
			db = db.Where("a.interpreted_at < ?", *scope.To)
		} else {
			db = db.Where("a.interpreted_at <= ?", *scope.To)
		}
	}
	if err := db.
		Order("a.testee_id ASC, a.created_at ASC, a.id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load non-plan assessments for fixup: %w", err)
	}
	return rows, nil
}

func parseAssessmentFixupInterpretedAtScope(fromRaw, toRaw string) (assessmentFixupInterpretedAtScope, error) {
	scope := assessmentFixupInterpretedAtScope{}

	fromRaw = strings.TrimSpace(fromRaw)
	toRaw = strings.TrimSpace(toRaw)
	if fromRaw == "" && toRaw == "" {
		return scope, nil
	}

	if fromRaw != "" {
		parsed, err := parseFlexibleSeedTimeInLocal(fromRaw)
		if err != nil {
			return scope, fmt.Errorf("parse assessment_fixup interpreted_from %q: %w", fromRaw, err)
		}
		scope.From = &parsed
	}
	if toRaw != "" {
		parsed, err := parseFlexibleSeedTimeInLocal(toRaw)
		if err != nil {
			return scope, fmt.Errorf("parse assessment_fixup interpreted_to %q: %w", toRaw, err)
		}
		if isDateOnlySeedTime(toRaw) {
			parsed = parsed.Add(24 * time.Hour)
			scope.ToExclusive = true
		}
		scope.To = &parsed
	}
	if scope.From != nil && scope.To != nil {
		if scope.ToExclusive {
			if !scope.From.Before(*scope.To) {
				return scope, fmt.Errorf("assessment_fixup interpreted_at range is empty: from=%s to=%s", scope.From.Format(time.RFC3339), scope.To.Format(time.RFC3339))
			}
		} else if scope.From.After(*scope.To) {
			return scope, fmt.Errorf("assessment_fixup interpreted_at range is invalid: from=%s to=%s", scope.From.Format(time.RFC3339), scope.To.Format(time.RFC3339))
		}
	}
	return scope, nil
}

func parseFlexibleSeedTimeInLocal(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layoutsInLocal := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layoutsInLocal {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", raw)
}

func isDateOnlySeedTime(raw string) bool {
	raw = strings.TrimSpace(raw)
	return len(raw) == len("2006-01-02") && !strings.ContainsAny(raw, "T :")
}

func formatAssessmentFixupScopeTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func openAssessmentFixupStores(
	ctx context.Context,
	deps *dependencies,
	stepName string,
) (int64, *gorm.DB, *mongo.Database, func(), error) {
	if deps.Config.Global.OrgID == 0 {
		return 0, nil, nil, nil, fmt.Errorf("global.orgId is required for %s", stepName)
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return 0, nil, nil, nil, fmt.Errorf("seeddata local.mysql_dsn is required for %s", stepName)
	}
	if strings.TrimSpace(deps.Config.Local.MongoURI) == "" {
		return 0, nil, nil, nil, fmt.Errorf("seeddata local.mongo_uri is required for %s", stepName)
	}
	if strings.TrimSpace(deps.Config.Local.MongoDatabase) == "" {
		return 0, nil, nil, nil, fmt.Errorf("seeddata local.mongo_database is required for %s", stepName)
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	mongoClient, mongoDB, err := openLocalSeedMongo(ctx, deps.Config.Local.MongoURI, deps.Config.Local.MongoDatabase)
	if err != nil {
		_ = closeLocalSeedMySQL(mysqlDB)
		return 0, nil, nil, nil, err
	}
	closeFn := func() {
		if closeErr := mongoClient.Disconnect(context.Background()); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mongo after "+stepName, "error", closeErr.Error())
		}
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after "+stepName, "error", closeErr.Error())
		}
	}
	return deps.Config.Global.OrgID, mysqlDB, mongoDB, closeFn, nil
}

func fixAssessmentEntryChain(
	ctx context.Context,
	mysqlDB *gorm.DB,
	anchors map[string]clinicianAssessmentEntryAnchor,
	entries []assessmentFixupEntryRow,
	relations []assessmentFixupEntryRelationRow,
	resolveLogs []assessmentFixupResolveLogRow,
	stats *assessmentFixupStats,
	progress *seedProgressBar,
) (map[uint64]assessmentFixupEntryRow, []assessmentFixupEntryAssessmentMatch, error) {
	entryByID := make(map[uint64]assessmentFixupEntryRow, len(entries))
	entriesByClinician := make(map[uint64][]assessmentFixupEntryRow, len(entries))
	for _, row := range entries {
		entriesByClinician[row.ClinicianID] = append(entriesByClinician[row.ClinicianID], row)
	}

	for clinicianID, clinicianEntries := range entriesByClinician {
		sort.SliceStable(clinicianEntries, func(i, j int) bool {
			if !clinicianEntries[i].CreatedAt.Equal(clinicianEntries[j].CreatedAt) {
				return clinicianEntries[i].CreatedAt.Before(clinicianEntries[j].CreatedAt)
			}
			return clinicianEntries[i].ID < clinicianEntries[j].ID
		})

		anchor, ok := anchors[strconv.FormatUint(clinicianID, 10)]
		for idx, row := range clinicianEntries {
			stats.EntriesProcessed++
			if !ok || anchor.AnchorCreatedAt.IsZero() {
				stats.EntriesMissingAnchor++
				stats.EntriesSkipped++
				entryByID[row.ID] = row
				progress.Increment()
				continue
			}

			targetCreatedAt := deriveAssessmentEntryCreatedAt(anchor.AnchorCreatedAt, idx)
			targetExpiresAt := deriveAssessmentEntryExpiresAtPreservingTTL(row.CreatedAt, row.ExpiresAt, targetCreatedAt)
			if row.CreatedAt.Equal(targetCreatedAt) && sameOptionalTime(row.ExpiresAt, targetExpiresAt) {
				stats.EntriesSkipped++
			} else {
				if err := backfillAssessmentEntryTimes(ctx, mysqlDB, strconv.FormatUint(row.ID, 10), targetCreatedAt, targetExpiresAt); err != nil {
					return nil, nil, err
				}
				stats.EntriesUpdated++
				row.CreatedAt = targetCreatedAt
				row.ExpiresAt = targetExpiresAt
			}
			entryByID[row.ID] = row
			progress.Increment()
		}
	}

	creatorByEntry := make(map[uint64][]assessmentFixupEntryRelationRow, len(relations))
	accessByEntryTestee := make(map[string][]assessmentFixupEntryRelationRow, len(relations))
	for _, row := range relations {
		if strings.EqualFold(strings.TrimSpace(row.RelationType), "creator") {
			creatorByEntry[row.EntryID] = append(creatorByEntry[row.EntryID], row)
			continue
		}
		if !isAccessGrantRelationType(row.RelationType) {
			continue
		}
		accessByEntryTestee[assessmentFixupEntryTesteeKey(row.EntryID, row.TesteeID)] = append(accessByEntryTestee[assessmentFixupEntryTesteeKey(row.EntryID, row.TesteeID)], row)
	}
	resolveLogsByEntryClinician := make(map[string][]assessmentFixupResolveLogRow, len(resolveLogs))
	for _, row := range resolveLogs {
		key := assessmentFixupEntryClinicianKey(row.EntryID, row.ClinicianID)
		resolveLogsByEntryClinician[key] = append(resolveLogsByEntryClinician[key], row)
	}

	entryIDs := make([]uint64, 0, len(entryByID))
	for entryID := range entryByID {
		entryIDs = append(entryIDs, entryID)
	}
	sort.Slice(entryIDs, func(i, j int) bool { return entryIDs[i] < entryIDs[j] })

	creatorPlans := make([]assessmentFixupEntryAssessmentMatch, 0, len(relations))
	for _, entryID := range entryIDs {
		entry := entryByID[entryID]
		creators := creatorByEntry[entryID]
		if len(creators) == 0 {
			continue
		}
		sort.SliceStable(creators, func(i, j int) bool {
			if !creators[i].TesteeCreatedAt.Equal(creators[j].TesteeCreatedAt) {
				return creators[i].TesteeCreatedAt.Before(creators[j].TesteeCreatedAt)
			}
			if creators[i].TesteeID != creators[j].TesteeID {
				return creators[i].TesteeID < creators[j].TesteeID
			}
			return creators[i].ID < creators[j].ID
		})

		resolvedAts := make([]time.Time, 0, len(creators))
		for _, creator := range creators {
			stats.EntryCreatorRelationsProcessed++
			resolveAt := deriveEntryResolveAt(entry.CreatedAt, creator.TesteeCreatedAt)
			intakeAt := deriveEntryIntakeAt(resolveAt)
			accessAt := deriveEntryAccessRelationAt(intakeAt)

			if !creator.BoundAt.Equal(intakeAt) {
				if err := updateAssessmentEntryRelationTimes(ctx, mysqlDB, strconv.FormatUint(creator.ID, 10), intakeAt); err != nil {
					return nil, nil, err
				}
				stats.EntryCreatorRelationsUpdated++
				creator.BoundAt = intakeAt
			}

			for _, accessRow := range accessByEntryTestee[assessmentFixupEntryTesteeKey(entryID, creator.TesteeID)] {
				if err := updateAssessmentEntryRelationTimes(ctx, mysqlDB, strconv.FormatUint(accessRow.ID, 10), accessAt); err != nil {
					return nil, nil, err
				}
				stats.EntryAccessRelationsUpdated++
			}

			resolvedAts = append(resolvedAts, resolveAt)
			creatorPlans = append(creatorPlans, assessmentFixupEntryAssessmentMatch{
				Relation:    creator,
				Entry:       entry,
				ResolveAt:   resolveAt,
				IntakeAt:    intakeAt,
				SubmitAt:    deriveEntryAssessmentSubmitAt(intakeAt),
				InterpretAt: deriveAssessmentInterpretAt(deriveEntryAssessmentSubmitAt(intakeAt)),
			})
		}

		logKey := assessmentFixupEntryClinicianKey(entryID, entry.ClinicianID)
		logRows := resolveLogsByEntryClinician[logKey]
		sort.SliceStable(logRows, func(i, j int) bool {
			if !logRows[i].ResolvedAt.Equal(logRows[j].ResolvedAt) {
				return logRows[i].ResolvedAt.Before(logRows[j].ResolvedAt)
			}
			return logRows[i].ID < logRows[j].ID
		})
		if len(logRows) != len(resolvedAts) {
			stats.EntryResolveLogMismatches++
		}
		limit := len(logRows)
		if len(resolvedAts) < limit {
			limit = len(resolvedAts)
		}
		for idx := 0; idx < limit; idx++ {
			if err := updateAssessmentEntryResolveLogByID(ctx, mysqlDB, logRows[idx].ID, resolvedAts[idx]); err != nil {
				return nil, nil, err
			}
			stats.EntryResolveLogsUpdated++
		}
	}

	return entryByID, creatorPlans, nil
}

func deriveAssessmentEntryExpiresAtPreservingTTL(currentCreatedAt time.Time, currentExpiresAt *time.Time, targetCreatedAt time.Time) *time.Time {
	if currentExpiresAt == nil {
		return nil
	}
	if currentCreatedAt.IsZero() || !currentExpiresAt.After(currentCreatedAt) {
		value := currentExpiresAt.Round(0)
		return &value
	}
	value := targetCreatedAt.Add(currentExpiresAt.Sub(currentCreatedAt)).Round(0)
	return &value
}

func updateAssessmentEntryResolveLogByID(ctx context.Context, mysqlDB *gorm.DB, logID uint64, resolvedAt time.Time) error {
	if err := mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AssessmentEntryResolveLogPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", logID).
		Updates(map[string]interface{}{
			"resolved_at": resolvedAt,
			"created_at":  resolvedAt,
			"updated_at":  resolvedAt,
		}).Error; err != nil {
		return fmt.Errorf("update assessment_entry_resolve_log %d timestamps: %w", logID, err)
	}
	return nil
}

func fixEntryBasedAssessments(
	ctx context.Context,
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	creatorPlans []assessmentFixupEntryAssessmentMatch,
	assessments []assessmentFixupAssessmentRow,
	stats *assessmentFixupStats,
	progress *seedProgressBar,
) (map[uint64]struct{}, error) {
	assessmentsByTestee := make(map[uint64][]assessmentFixupAssessmentRow, 128)
	for _, row := range assessments {
		assessmentsByTestee[row.TesteeID] = append(assessmentsByTestee[row.TesteeID], row)
	}
	for testeeID := range assessmentsByTestee {
		sort.SliceStable(assessmentsByTestee[testeeID], func(i, j int) bool {
			if !assessmentsByTestee[testeeID][i].CreatedAt.Equal(assessmentsByTestee[testeeID][j].CreatedAt) {
				return assessmentsByTestee[testeeID][i].CreatedAt.Before(assessmentsByTestee[testeeID][j].CreatedAt)
			}
			return assessmentsByTestee[testeeID][i].ID < assessmentsByTestee[testeeID][j].ID
		})
	}

	sort.SliceStable(creatorPlans, func(i, j int) bool {
		if creatorPlans[i].Relation.TesteeID != creatorPlans[j].Relation.TesteeID {
			return creatorPlans[i].Relation.TesteeID < creatorPlans[j].Relation.TesteeID
		}
		if !creatorPlans[i].IntakeAt.Equal(creatorPlans[j].IntakeAt) {
			return creatorPlans[i].IntakeAt.Before(creatorPlans[j].IntakeAt)
		}
		return creatorPlans[i].Relation.ID < creatorPlans[j].Relation.ID
	})

	usedAssessmentIDs := make(map[uint64]struct{}, len(creatorPlans))
	for _, plan := range creatorPlans {
		matched := false
		for _, row := range assessmentsByTestee[plan.Relation.TesteeID] {
			if _, exists := usedAssessmentIDs[row.ID]; exists {
				continue
			}
			if !assessmentMatchesEntryTarget(row, plan.Entry) {
				continue
			}
			usedAssessmentIDs[row.ID] = struct{}{}
			stats.EntryAssessmentsMatched++
			if _, err := applyAssessmentTimestampFixup(ctx, mysqlDB, mongoDB, row, plan.SubmitAt, plan.InterpretAt, stats); err != nil {
				return nil, err
			}
			stats.EntryAssessmentsUpdated++
			matched = true
			break
		}
		if !matched {
			stats.EntryAssessmentsUnmatched++
		}
		progress.Increment()
	}
	return usedAssessmentIDs, nil
}

func buildEntryAssessmentPlansWithoutMutation(
	entryByID map[uint64]assessmentFixupEntryRow,
	relations []assessmentFixupEntryRelationRow,
) []assessmentFixupEntryAssessmentMatch {
	creatorByEntry := make(map[uint64][]assessmentFixupEntryRelationRow, len(relations))
	for _, row := range relations {
		if strings.EqualFold(strings.TrimSpace(row.RelationType), "creator") {
			creatorByEntry[row.EntryID] = append(creatorByEntry[row.EntryID], row)
		}
	}

	entryIDs := make([]uint64, 0, len(entryByID))
	for entryID := range entryByID {
		entryIDs = append(entryIDs, entryID)
	}
	sort.Slice(entryIDs, func(i, j int) bool { return entryIDs[i] < entryIDs[j] })

	plans := make([]assessmentFixupEntryAssessmentMatch, 0, len(relations))
	for _, entryID := range entryIDs {
		entry := entryByID[entryID]
		creators := creatorByEntry[entryID]
		if len(creators) == 0 {
			continue
		}
		sort.SliceStable(creators, func(i, j int) bool {
			if !creators[i].TesteeCreatedAt.Equal(creators[j].TesteeCreatedAt) {
				return creators[i].TesteeCreatedAt.Before(creators[j].TesteeCreatedAt)
			}
			if creators[i].TesteeID != creators[j].TesteeID {
				return creators[i].TesteeID < creators[j].TesteeID
			}
			return creators[i].ID < creators[j].ID
		})

		for _, creator := range creators {
			resolveAt := deriveEntryResolveAt(entry.CreatedAt, creator.TesteeCreatedAt)
			intakeAt := deriveEntryIntakeAt(resolveAt)
			plans = append(plans, assessmentFixupEntryAssessmentMatch{
				Relation:    creator,
				Entry:       entry,
				ResolveAt:   resolveAt,
				IntakeAt:    intakeAt,
				SubmitAt:    deriveEntryAssessmentSubmitAt(intakeAt),
				InterpretAt: deriveAssessmentInterpretAt(deriveEntryAssessmentSubmitAt(intakeAt)),
			})
		}
	}
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].Relation.TesteeID != plans[j].Relation.TesteeID {
			return plans[i].Relation.TesteeID < plans[j].Relation.TesteeID
		}
		if !plans[i].IntakeAt.Equal(plans[j].IntakeAt) {
			return plans[i].IntakeAt.Before(plans[j].IntakeAt)
		}
		return plans[i].Relation.ID < plans[j].Relation.ID
	})
	return plans
}

func collectMatchedEntryAssessmentIDs(
	creatorPlans []assessmentFixupEntryAssessmentMatch,
	assessments []assessmentFixupAssessmentRow,
	stats *assessmentFixupStats,
) (map[uint64]struct{}, error) {
	assessmentsByTestee := make(map[uint64][]assessmentFixupAssessmentRow, 128)
	for _, row := range assessments {
		assessmentsByTestee[row.TesteeID] = append(assessmentsByTestee[row.TesteeID], row)
	}
	for testeeID := range assessmentsByTestee {
		sort.SliceStable(assessmentsByTestee[testeeID], func(i, j int) bool {
			if !assessmentsByTestee[testeeID][i].CreatedAt.Equal(assessmentsByTestee[testeeID][j].CreatedAt) {
				return assessmentsByTestee[testeeID][i].CreatedAt.Before(assessmentsByTestee[testeeID][j].CreatedAt)
			}
			return assessmentsByTestee[testeeID][i].ID < assessmentsByTestee[testeeID][j].ID
		})
	}

	usedAssessmentIDs := make(map[uint64]struct{}, len(creatorPlans))
	for _, plan := range creatorPlans {
		matched := false
		for _, row := range assessmentsByTestee[plan.Relation.TesteeID] {
			if _, exists := usedAssessmentIDs[row.ID]; exists {
				continue
			}
			if !assessmentMatchesEntryTarget(row, plan.Entry) {
				continue
			}
			usedAssessmentIDs[row.ID] = struct{}{}
			if stats != nil {
				stats.EntryAssessmentsMatched++
			}
			matched = true
			break
		}
		if !matched && stats != nil {
			stats.EntryAssessmentsUnmatched++
		}
	}
	return usedAssessmentIDs, nil
}

func collectStandaloneAssessments(rows []assessmentFixupAssessmentRow, excluded map[uint64]struct{}) []assessmentFixupAssessmentRow {
	if len(rows) == 0 {
		return nil
	}
	result := make([]assessmentFixupAssessmentRow, 0, len(rows))
	for _, row := range rows {
		if _, exists := excluded[row.ID]; exists {
			continue
		}
		result = append(result, row)
	}
	return result
}

func fixStandaloneAssessments(
	ctx context.Context,
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	rows []assessmentFixupAssessmentRow,
	stats *assessmentFixupStats,
	progress *seedProgressBar,
) error {
	if len(rows) == 0 {
		return nil
	}
	rowsByTestee := make(map[uint64][]assessmentFixupAssessmentRow, 128)
	for _, row := range rows {
		rowsByTestee[row.TesteeID] = append(rowsByTestee[row.TesteeID], row)
	}

	testeeIDs := make([]uint64, 0, len(rowsByTestee))
	for testeeID := range rowsByTestee {
		testeeIDs = append(testeeIDs, testeeID)
	}
	sort.Slice(testeeIDs, func(i, j int) bool { return testeeIDs[i] < testeeIDs[j] })

	for _, testeeID := range testeeIDs {
		items := rowsByTestee[testeeID]
		sort.SliceStable(items, func(i, j int) bool {
			if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
				return items[i].CreatedAt.Before(items[j].CreatedAt)
			}
			return items[i].ID < items[j].ID
		})

		submitTimes := deriveStandaloneAssessmentSubmitTimes(items[0].TesteeCreatedAt, items, deriveStandaloneAssessmentSubmitCeiling(items[0].TesteeCreatedAt))
		for idx, row := range items {
			interpretAt := deriveAssessmentInterpretAt(submitTimes[idx])
			if _, err := applyAssessmentTimestampFixup(ctx, mysqlDB, mongoDB, row, submitTimes[idx], interpretAt, stats); err != nil {
				return err
			}
			stats.StandaloneAssessmentsUpdated++
			progress.Increment()
		}
	}
	return nil
}

func deriveStandaloneAssessmentSubmitCeiling(testeeCreatedAt time.Time) time.Time {
	if testeeCreatedAt.IsZero() {
		return testeeCreatedAtFixupRangeEnd.Add(-seedAssessmentInterpretOffset).Round(0)
	}

	ceiling := testeeCreatedAt.Round(0).Add(assessmentFixupStandaloneWindow)
	if !testeeCreatedAtFixupRangeEnd.IsZero() && ceiling.After(testeeCreatedAtFixupRangeEnd) {
		ceiling = testeeCreatedAtFixupRangeEnd.Round(0)
	}
	if !ceiling.IsZero() {
		ceiling = ceiling.Add(-seedAssessmentInterpretOffset).Round(0)
	}
	if ceiling.Before(testeeCreatedAt.Round(0)) {
		return testeeCreatedAt.Round(0)
	}
	return ceiling
}

func applyAssessmentTimestampFixup(
	ctx context.Context,
	mysqlDB *gorm.DB,
	mongoDB *mongo.Database,
	row assessmentFixupAssessmentRow,
	submittedAt time.Time,
	interpretedAt time.Time,
	stats *assessmentFixupStats,
) (bool, error) {
	reportExists := false
	if normalizeTaskStatus(row.Status) != "failed" {
		updated, err := updatePlanFixupReport(ctx, mongoDB, row.ID, interpretedAt)
		if err != nil {
			return false, err
		}
		if updated {
			reportExists = true
			stats.ReportsUpdated++
		} else {
			stats.MissingReports++
		}
	}

	if err := updatePlanFixupAssessment(ctx, mysqlDB, planFixupAssessmentRow{
		ID:            row.ID,
		AnswerSheetID: row.AnswerSheetID,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		SubmittedAt:   row.SubmittedAt,
		InterpretedAt: row.InterpretedAt,
		FailedAt:      row.FailedAt,
	}, planFixupTimes{
		CompletionAt: submittedAt,
		InterpretAt:  interpretedAt,
	}, reportExists); err != nil {
		return false, err
	}
	stats.AssessmentsUpdated++

	if row.AnswerSheetID == 0 {
		stats.MissingAnswerSheets++
		return reportExists, nil
	}

	answerSheetUpdatedAt := submittedAt
	if normalizeTaskStatus(row.Status) == "interpreted" || reportExists {
		answerSheetUpdatedAt = interpretedAt
	}
	updated, err := updatePlanFixupAnswerSheet(ctx, mongoDB, row.AnswerSheetID, submittedAt, answerSheetUpdatedAt)
	if err != nil {
		return false, err
	}
	if !updated {
		stats.MissingAnswerSheets++
		return reportExists, nil
	}
	stats.AnswerSheetsUpdated++
	return reportExists, nil
}

func assessmentMatchesEntryTarget(row assessmentFixupAssessmentRow, entry assessmentFixupEntryRow) bool {
	targetType := strings.ToLower(strings.TrimSpace(entry.TargetType))
	targetCode := strings.TrimSpace(entry.TargetCode)
	targetVersion := strings.TrimSpace(nullableString(entry.TargetVersion))
	switch targetType {
	case "scale":
		return strings.EqualFold(strings.TrimSpace(nullableString(row.MedicalScaleCode)), targetCode)
	case "questionnaire":
		if !strings.EqualFold(strings.TrimSpace(row.QuestionnaireCode), targetCode) {
			return false
		}
		if targetVersion == "" {
			return true
		}
		return strings.EqualFold(strings.TrimSpace(row.QuestionnaireVersion), targetVersion)
	default:
		return false
	}
}

func deriveStandaloneAssessmentSubmitTimes(
	testeeCreatedAt time.Time,
	rows []assessmentFixupAssessmentRow,
	ceiling time.Time,
) []time.Time {
	if len(rows) == 0 {
		return nil
	}

	startAt := testeeCreatedAt.Round(0).Add(assessmentFixupStandaloneInitialOffset)
	if !ceiling.IsZero() && startAt.After(ceiling) {
		startAt = ceiling.Round(0)
	}
	rawTimes := make([]time.Time, len(rows))
	current := startAt
	for idx, row := range rows {
		if idx == 0 {
			rawTimes[idx] = current
			continue
		}
		unit := stableSeedUnitFloat(row.TesteeID, row.ID, uint64(idx), uint64(len(rows)))
		gap := assessmentFixupStandaloneMinGap + time.Duration(math.Round(float64(assessmentFixupStandaloneGapJitter)*unit))
		current = current.Add(gap).Round(0)
		rawTimes[idx] = current
	}

	if ceiling.IsZero() || !rawTimes[len(rawTimes)-1].After(ceiling) {
		return rawTimes
	}
	if !ceiling.After(startAt) {
		for idx := range rawTimes {
			rawTimes[idx] = ceiling.Round(0)
		}
		return rawTimes
	}

	lastRaw := rawTimes[len(rawTimes)-1]
	rawSpan := lastRaw.Sub(startAt)
	availableSpan := ceiling.Sub(startAt)
	if rawSpan <= 0 || availableSpan <= 0 {
		for idx := range rawTimes {
			rawTimes[idx] = startAt.Round(0)
		}
		return rawTimes
	}

	compressed := make([]time.Time, len(rawTimes))
	for idx, raw := range rawTimes {
		if idx == 0 {
			compressed[idx] = startAt.Round(0)
			continue
		}
		ratio := float64(raw.Sub(startAt)) / float64(rawSpan)
		offset := time.Duration(math.Round(float64(availableSpan) * ratio))
		compressed[idx] = startAt.Add(offset).Round(0)
	}
	return compressed
}

func assessmentFixupEntryClinicianKey(entryID uint64, clinicianID uint64) string {
	return strconv.FormatUint(entryID, 10) + "|" + strconv.FormatUint(clinicianID, 10)
}

func assessmentFixupEntryTesteeKey(entryID uint64, testeeID uint64) string {
	return strconv.FormatUint(entryID, 10) + "|" + strconv.FormatUint(testeeID, 10)
}

var (
	_ = answerSheetMongo.AnswerSheetPO{}
	_ = evaluationMongo.InterpretReportPO{}
)
