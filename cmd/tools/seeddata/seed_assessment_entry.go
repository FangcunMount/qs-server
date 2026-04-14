package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"gorm.io/gorm"
)

const (
	assessmentEntrySeedPageSize       = 100
	assessmentEntrySeedTargetInterval = 10 * time.Minute
)

type clinicianAssessmentEntryAnchor struct {
	ClinicianID       uint64    `gorm:"column:clinician_id"`
	AnchorCreatedAt   time.Time `gorm:"column:anchor_created_at"`
	ActiveTesteeCount int64     `gorm:"column:active_testee_count"`
}

func seedAssessmentEntries(ctx context.Context, deps *dependencies) error {
	orgID := deps.Config.Global.OrgID
	if orgID == 0 {
		return fmt.Errorf("global.orgId is required for assessment entry seeding")
	}
	if len(deps.Config.AssessmentEntryTargets) == 0 {
		return fmt.Errorf("assessmentEntryTargets is required for assessment_entries step")
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return fmt.Errorf("assessment_entries requires local.mysql_dsn because entry timestamps are backfilled from testee.created_at")
	}

	targets, err := normalizeAssessmentEntryTargets(deps.Config.AssessmentEntryTargets)
	if err != nil {
		return err
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after assessment entry seeding", "error", closeErr.Error())
		}
	}()

	allClinicians, err := listAllClinicians(ctx, deps.APIClient, orgID)
	if err != nil {
		return err
	}
	anchors, err := loadClinicianAssessmentEntryAnchors(ctx, mysqlDB, orgID)
	if err != nil {
		return err
	}

	eligibleClinicians := make([]*ClinicianResponse, 0, len(allClinicians))
	for _, item := range allClinicians {
		if item == nil || !item.IsActive || item.AssignedTesteeCount <= 0 {
			continue
		}
		eligibleClinicians = append(eligibleClinicians, item)
	}

	totalCreated := 0
	totalUpdated := 0
	totalSkipped := 0
	totalMissingAnchor := 0

	if len(eligibleClinicians) == 0 {
		deps.Logger.Infow("No eligible clinicians found for assessment entry seeding",
			"total_clinicians", len(allClinicians),
			"eligible_clinicians", 0,
			"target_count", len(targets),
		)
		return nil
	}

	progress := newSeedProgressBar("assessment_entries clinicians", len(eligibleClinicians))
	defer progress.Close()
	for _, clinicianItem := range eligibleClinicians {
		anchor, ok := anchors[strings.TrimSpace(clinicianItem.ID)]
		if !ok || anchor.AnchorCreatedAt.IsZero() {
			totalMissingAnchor++
			deps.Logger.Warnw("Skipping clinician assessment entry seeding because no active testee created_at anchor was found",
				"clinician_id", clinicianItem.ID,
				"clinician_name", clinicianItem.Name,
				"assigned_testee_count", clinicianItem.AssignedTesteeCount,
			)
			progress.Increment()
			continue
		}

		existingEntries, err := listAllClinicianAssessmentEntries(ctx, deps.APIClient, clinicianItem.ID)
		if err != nil {
			return fmt.Errorf("list assessment entries for clinician %s: %w", clinicianItem.ID, err)
		}
		existingTargets := make(map[string]*AssessmentEntryResponse, len(existingEntries))
		for _, item := range existingEntries {
			if item == nil {
				continue
			}
			existingTargets[assessmentEntryTargetKey(item.TargetType, item.TargetCode, item.TargetVersion)] = item
		}

		createdForClinician := 0
		updatedForClinician := 0
		skippedForClinician := 0
		for idx, target := range targets {
			createdAt := deriveAssessmentEntryCreatedAt(anchor.AnchorCreatedAt, idx)
			expiresAt, err := resolveAssessmentEntryExpiresAt(target, createdAt)
			if err != nil {
				return fmt.Errorf("resolve expires_at for clinician %s target %s: %w", clinicianItem.ID, assessmentEntryTargetLabel(target), err)
			}

			targetKey := assessmentEntryTargetKey(target.TargetType, target.TargetCode, target.TargetVersion)
			if existingEntry, exists := existingTargets[targetKey]; exists {
				updated, ensureErr := ensureAssessmentEntryTimes(ctx, mysqlDB, existingEntry.ID, createdAt, expiresAt)
				if ensureErr != nil {
					return fmt.Errorf("ensure assessment entry %s timestamps: %w", existingEntry.ID, ensureErr)
				}
				if updated {
					totalUpdated++
					updatedForClinician++
				} else {
					totalSkipped++
					skippedForClinician++
				}
				continue
			}

			entryResp, err := deps.APIClient.CreateClinicianAssessmentEntry(ctx, clinicianItem.ID, CreateAssessmentEntryRequest{
				TargetType:    target.TargetType,
				TargetCode:    target.TargetCode,
				TargetVersion: target.TargetVersion,
				ExpiresAt:     expiresAt,
			})
			if err != nil {
				return fmt.Errorf("create assessment entry for clinician %s target %s: %w", clinicianItem.ID, assessmentEntryTargetLabel(target), err)
			}
			if err := backfillAssessmentEntryTimes(ctx, mysqlDB, entryResp.ID, createdAt, expiresAt); err != nil {
				return fmt.Errorf("backfill assessment entry %s timestamps: %w", entryResp.ID, err)
			}

			existingTargets[targetKey] = entryResp
			totalCreated++
			createdForClinician++
		}

		deps.Logger.Infow("Clinician assessment entry seeding completed",
			"clinician_id", clinicianItem.ID,
			"clinician_name", clinicianItem.Name,
			"target_count", len(targets),
			"anchor_created_at", anchor.AnchorCreatedAt,
			"active_testee_count", anchor.ActiveTesteeCount,
			"created", createdForClinician,
			"updated", updatedForClinician,
			"skipped", skippedForClinician,
		)
		progress.Increment()
	}
	progress.Complete()

	deps.Logger.Infow("Assessment entry seeding completed",
		"total_clinicians", len(allClinicians),
		"eligible_clinicians", len(eligibleClinicians),
		"missing_anchor_clinicians", totalMissingAnchor,
		"target_count", len(targets),
		"created", totalCreated,
		"updated", totalUpdated,
		"skipped", totalSkipped,
	)
	return nil
}

func normalizeAssessmentEntryTargets(configs []AssessmentEntryTargetConfig) ([]AssessmentEntryTargetConfig, error) {
	result := make([]AssessmentEntryTargetConfig, 0, len(configs))
	seen := make(map[string]struct{}, len(configs))
	for idx, cfg := range configs {
		normalized, err := validateAndNormalizeAssessmentEntryTargetConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid assessmentEntryTargets config at index %d: %w", idx, err)
		}
		key := assessmentEntryTargetKey(normalized.TargetType, normalized.TargetCode, normalized.TargetVersion)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate assessment entry target %q", assessmentEntryTargetLabel(normalized))
		}
		seen[key] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func validateAndNormalizeAssessmentEntryTargetConfig(cfg AssessmentEntryTargetConfig) (AssessmentEntryTargetConfig, error) {
	cfg.Key = strings.TrimSpace(cfg.Key)
	cfg.TargetType = strings.ToLower(strings.TrimSpace(cfg.TargetType))
	cfg.TargetCode = strings.TrimSpace(cfg.TargetCode)
	cfg.TargetVersion = strings.TrimSpace(cfg.TargetVersion)
	cfg.ExpiresAt = strings.TrimSpace(cfg.ExpiresAt)
	cfg.ExpiresAfter = strings.TrimSpace(cfg.ExpiresAfter)

	switch cfg.TargetType {
	case "questionnaire", "scale":
	default:
		return AssessmentEntryTargetConfig{}, fmt.Errorf("unsupported targetType %q", cfg.TargetType)
	}
	if cfg.TargetCode == "" {
		return AssessmentEntryTargetConfig{}, fmt.Errorf("targetCode is required")
	}
	if cfg.ExpiresAt != "" && cfg.ExpiresAfter != "" {
		return AssessmentEntryTargetConfig{}, fmt.Errorf("expiresAt and expiresAfter cannot both be set")
	}
	if cfg.ExpiresAt != "" {
		if _, err := parseFlexibleSeedTime(cfg.ExpiresAt); err != nil {
			return AssessmentEntryTargetConfig{}, fmt.Errorf("invalid expiresAt %q: %w", cfg.ExpiresAt, err)
		}
	}
	if cfg.ExpiresAfter != "" {
		duration, err := parseSeedRelativeDuration(cfg.ExpiresAfter)
		if err != nil {
			return AssessmentEntryTargetConfig{}, fmt.Errorf("invalid expiresAfter %q: %w", cfg.ExpiresAfter, err)
		}
		if duration <= 0 {
			return AssessmentEntryTargetConfig{}, fmt.Errorf("expiresAfter must be greater than 0")
		}
	}
	return cfg, nil
}

func listAllClinicianAssessmentEntries(ctx context.Context, client *APIClient, clinicianID string) ([]*AssessmentEntryResponse, error) {
	page := 1
	items := make([]*AssessmentEntryResponse, 0, assessmentEntrySeedPageSize)
	for {
		resp, err := client.ListClinicianAssessmentEntries(ctx, clinicianID, page, assessmentEntrySeedPageSize)
		if err != nil {
			return nil, err
		}
		if len(resp.Items) == 0 {
			break
		}
		items = append(items, resp.Items...)
		if resp.TotalPages > 0 && page >= resp.TotalPages {
			break
		}
		page++
	}
	return items, nil
}

func loadClinicianAssessmentEntryAnchors(ctx context.Context, mysqlDB *gorm.DB, orgID int64) (map[string]clinicianAssessmentEntryAnchor, error) {
	rows := make([]clinicianAssessmentEntryAnchor, 0, 64)
	if err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()+" AS cr").
		Select("cr.clinician_id, MIN(t.created_at) AS anchor_created_at, COUNT(DISTINCT cr.testee_id) AS active_testee_count").
		Joins("JOIN "+(actorMySQL.TesteePO{}).TableName()+" AS t ON t.id = cr.testee_id AND t.deleted_at IS NULL").
		Where("cr.org_id = ? AND cr.is_active = 1 AND cr.deleted_at IS NULL", orgID).
		Group("cr.clinician_id").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load clinician assessment entry anchors: %w", err)
	}

	result := make(map[string]clinicianAssessmentEntryAnchor, len(rows))
	for _, row := range rows {
		result[strconv.FormatUint(row.ClinicianID, 10)] = row
	}
	return result, nil
}

func deriveAssessmentEntryCreatedAt(anchor time.Time, targetIndex int) time.Time {
	createdAt := anchor.Round(0)
	if createdAt.IsZero() || targetIndex <= 0 {
		return createdAt
	}
	return createdAt.Add(time.Duration(targetIndex) * assessmentEntrySeedTargetInterval)
}

func resolveAssessmentEntryExpiresAt(cfg AssessmentEntryTargetConfig, createdAt time.Time) (*time.Time, error) {
	return resolveAssessmentEntryExpiresAtAt(cfg, createdAt, time.Now())
}

func resolveAssessmentEntryExpiresAtAt(cfg AssessmentEntryTargetConfig, createdAt, now time.Time) (*time.Time, error) {
	if cfg.ExpiresAfter != "" {
		duration, err := parseSeedRelativeDuration(cfg.ExpiresAfter)
		if err != nil {
			return nil, err
		}
		base := createdAt
		if base.IsZero() {
			base = now.In(time.Local)
		}
		value := base.Add(duration)
		if !now.IsZero() && !value.After(now) {
			value = now.In(base.Location()).Add(duration)
		}
		return &value, nil
	}
	if cfg.ExpiresAt == "" {
		return nil, nil
	}

	value, err := parseFlexibleSeedTime(cfg.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if !createdAt.IsZero() && value.Before(createdAt) {
		return nil, fmt.Errorf("expiresAt %s is before derived created_at %s", value.Format(time.RFC3339), createdAt.Format(time.RFC3339))
	}
	return &value, nil
}

func ensureAssessmentEntryTimes(ctx context.Context, mysqlDB *gorm.DB, entryID string, createdAt time.Time, expiresAt *time.Time) (bool, error) {
	currentCreatedAt, currentExpiresAt, err := loadAssessmentEntryTimes(ctx, mysqlDB, entryID)
	if err != nil {
		return false, err
	}
	if currentCreatedAt.Equal(createdAt) && sameOptionalTime(currentExpiresAt, expiresAt) {
		return false, nil
	}
	if err := backfillAssessmentEntryTimes(ctx, mysqlDB, entryID, createdAt, expiresAt); err != nil {
		return false, err
	}
	return true, nil
}

func loadAssessmentEntryTimes(ctx context.Context, mysqlDB *gorm.DB, entryID string) (time.Time, *time.Time, error) {
	var row struct {
		CreatedAt time.Time  `gorm:"column:created_at"`
		ExpiresAt *time.Time `gorm:"column:expires_at"`
	}
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.AssessmentEntryPO{}).TableName()).
		Select("created_at, expires_at").
		Where("id = ? AND deleted_at IS NULL", strings.TrimSpace(entryID)).
		Take(&row).Error
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("load assessment entry timestamps: %w", err)
	}
	return row.CreatedAt, row.ExpiresAt, nil
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func backfillAssessmentEntryTimes(ctx context.Context, mysqlDB *gorm.DB, entryID string, createdAt time.Time, expiresAt *time.Time) error {
	updates := map[string]interface{}{
		"created_at": createdAt,
		"updated_at": createdAt,
		"expires_at": expiresAt,
	}
	result := mysqlDB.WithContext(ctx).
		Table((actorMySQL.AssessmentEntryPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", strings.TrimSpace(entryID)).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update assessment_entry timestamps: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("assessment_entry %s not found after creation", entryID)
	}
	return nil
}

func assessmentEntryTargetKey(targetType, targetCode, targetVersion string) string {
	return strings.ToLower(strings.TrimSpace(targetType)) + "|" +
		strings.ToLower(strings.TrimSpace(targetCode)) + "|" +
		strings.ToLower(strings.TrimSpace(targetVersion))
}

func assessmentEntryTargetLabel(cfg AssessmentEntryTargetConfig) string {
	if cfg.TargetVersion == "" {
		return fmt.Sprintf("%s:%s", cfg.TargetType, cfg.TargetCode)
	}
	return fmt.Sprintf("%s:%s@%s", cfg.TargetType, cfg.TargetCode, cfg.TargetVersion)
}

func parseFlexibleSeedTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var parsed time.Time
	var err error
	for _, layout := range layouts {
		parsed, err = time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, err
}

func parseSeedRelativeDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, fmt.Errorf("duration is empty")
	}

	if strings.HasSuffix(raw, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(raw, "d"), 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	if strings.HasSuffix(raw, "w") {
		weeks, err := strconv.ParseFloat(strings.TrimSuffix(raw, "w"), 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(weeks * float64(7*24*time.Hour)), nil
	}
	return time.ParseDuration(raw)
}
