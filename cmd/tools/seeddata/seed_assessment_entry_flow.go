package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"gorm.io/gorm"
)

func seedAssessmentEntryFlow(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for assessment_entry_flow")
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for assessment_entry_flow")
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after assessment entry flow", "error", closeErr.Error())
		}
	}()

	cfg := deps.Config.AssessmentEntryFlow
	clinicians, err := resolveSeedClinicianScope(ctx, deps, seedClinicianScopeSpec{
		refs: cfg.ClinicianRefs,
		ids:  cfg.ClinicianIDs,
	})
	if err != nil {
		return err
	}
	entryFilter := flexibleIDSet(cfg.EntryIDs)
	maxIntakes := normalizeMaxIntakesPerEntry(cfg.MaxIntakesPerEntry)

	totalEntries := 0
	totalResolved := 0
	totalSkipped := 0

	for _, clinicianItem := range clinicians {
		if clinicianItem == nil || !clinicianItem.IsActive {
			continue
		}

		entries, err := listAllClinicianAssessmentEntries(ctx, deps.APIClient, clinicianItem.ID)
		if err != nil {
			return fmt.Errorf("list assessment entries for clinician %s: %w", clinicianItem.ID, err)
		}
		relations, err := listAllClinicianRelations(ctx, deps.APIClient, clinicianItem.ID)
		if err != nil {
			return fmt.Errorf("list relations for clinician %s: %w", clinicianItem.ID, err)
		}

		creatorByEntryAndTestee := make(map[string]struct{}, len(relations))
		accessCandidates := make([]*ClinicianRelationResponse, 0, len(relations))
		for _, item := range relations {
			if item == nil || item.Relation == nil || item.Testee == nil {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(item.Relation.RelationType), "creator") {
				key := strings.TrimSpace(item.Testee.ID) + "|" + strings.TrimSpace(nullableString(item.Relation.SourceID))
				creatorByEntryAndTestee[key] = struct{}{}
				continue
			}
			if isAccessGrantRelationType(item.Relation.RelationType) {
				accessCandidates = append(accessCandidates, item)
			}
		}
		sortClinicianRelationsByTesteeCreatedAt(accessCandidates)

		createdForClinician := 0
		skippedForClinician := 0
		for _, entry := range entries {
			if entry == nil || !entry.IsActive {
				continue
			}
			if len(entryFilter) > 0 {
				if _, ok := entryFilter[strings.TrimSpace(entry.ID)]; !ok {
					continue
				}
			}
			totalEntries++

			entryCreatedAt, err := loadAssessmentEntryCreatedAt(ctx, mysqlDB, entry.ID)
			if err != nil {
				return fmt.Errorf("load created_at for assessment entry %s: %w", entry.ID, err)
			}

			processedForEntry := 0
			for _, candidate := range accessCandidates {
				if processedForEntry >= maxIntakes {
					break
				}
				if candidate == nil || candidate.Testee == nil || candidate.Relation == nil {
					continue
				}
				testeeID := strings.TrimSpace(candidate.Testee.ID)
				if testeeID == "" {
					continue
				}

				creatorKey := testeeID + "|" + strings.TrimSpace(entry.ID)
				if _, exists := creatorByEntryAndTestee[creatorKey]; exists {
					totalSkipped++
					skippedForClinician++
					continue
				}

				testeeDetail, err := deps.APIClient.GetTesteeByID(ctx, testeeID)
				if err != nil {
					return fmt.Errorf("get testee %s detail: %w", testeeID, err)
				}
				if testeeDetail == nil {
					totalSkipped++
					skippedForClinician++
					continue
				}
				if testeeDetail.ProfileID == nil && !cfg.AllowTemporaryTestee {
					totalSkipped++
					skippedForClinician++
					continue
				}

				resolveAt := deriveEntryResolveAt(entryCreatedAt, testeeDetail.CreatedAt)
				if _, err := deps.APIClient.ResolveAssessmentEntry(ctx, entry.Token); err != nil {
					return fmt.Errorf("resolve assessment entry %s for testee %s: %w", entry.ID, testeeID, err)
				}
				if err := backfillAssessmentEntryResolveLogTimes(ctx, mysqlDB, deps.Config.Global.OrgID, entry.ID, clinicianItem.ID, resolveAt); err != nil {
					return fmt.Errorf("backfill resolve log for entry %s: %w", entry.ID, err)
				}

				intakeReq, err := buildAssessmentEntryIntakeRequest(testeeDetail, cfg.AllowTemporaryTestee)
				if err != nil {
					totalSkipped++
					skippedForClinician++
					deps.Logger.Warnw("Skipping assessment entry intake because testee detail is incomplete",
						"entry_id", entry.ID,
						"clinician_id", clinicianItem.ID,
						"testee_id", testeeID,
						"error", err.Error(),
					)
					continue
				}
				intakeResp, err := deps.APIClient.IntakeAssessmentEntry(ctx, entry.Token, intakeReq)
				if err != nil {
					return fmt.Errorf("intake assessment entry %s for testee %s: %w", entry.ID, testeeID, err)
				}

				intakeAt := deriveEntryIntakeAt(resolveAt)
				if intakeResp != nil && isAssessmentEntryRelation(intakeResp.Relation, entry.ID) {
					if err := updateAssessmentEntryRelationTimes(ctx, mysqlDB, intakeResp.Relation.ID, intakeAt); err != nil {
						return err
					}
				}
				if intakeResp != nil && isAssessmentEntryRelation(intakeResp.Assignment, entry.ID) {
					if err := updateAssessmentEntryRelationTimes(ctx, mysqlDB, intakeResp.Assignment.ID, deriveEntryAccessRelationAt(intakeAt)); err != nil {
						return err
					}
				}

				creatorByEntryAndTestee[creatorKey] = struct{}{}
				totalResolved++
				createdForClinician++
				processedForEntry++
			}
		}

		deps.Logger.Infow("Assessment entry flow completed for clinician",
			"clinician_id", clinicianItem.ID,
			"clinician_name", clinicianItem.Name,
			"created", createdForClinician,
			"skipped", skippedForClinician,
			"max_intakes_per_entry", maxIntakes,
		)
	}

	deps.Logger.Infow("Assessment entry flow completed",
		"org_id", deps.Config.Global.OrgID,
		"processed_entries", totalEntries,
		"resolved_and_intaked", totalResolved,
		"skipped", totalSkipped,
		"max_intakes_per_entry", maxIntakes,
		"allow_temporary_testee", cfg.AllowTemporaryTestee,
	)
	return nil
}

func listAllClinicianRelations(ctx context.Context, client *APIClient, clinicianID string) ([]*ClinicianRelationResponse, error) {
	page := 1
	items := make([]*ClinicianRelationResponse, 0, seedEntryFlowPageSize)
	for {
		resp, err := client.ListClinicianRelations(ctx, clinicianID, page, seedEntryFlowPageSize)
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

func buildAssessmentEntryIntakeRequest(testee *ApiserverTesteeResponse, allowTemporary bool) (IntakeAssessmentEntryRequest, error) {
	if testee == nil {
		return IntakeAssessmentEntryRequest{}, fmt.Errorf("testee is nil")
	}
	req := IntakeAssessmentEntryRequest{
		Name:     strings.TrimSpace(testee.Name),
		Gender:   strings.TrimSpace(testee.Gender),
		Birthday: testee.Birthday,
	}
	if req.Name == "" {
		return IntakeAssessmentEntryRequest{}, fmt.Errorf("testee name is empty")
	}
	if testee.ProfileID != nil {
		profileID := parseID(strings.TrimSpace(*testee.ProfileID))
		if profileID > 0 {
			req.ProfileID = &profileID
			return req, nil
		}
	}
	if !allowTemporary {
		return IntakeAssessmentEntryRequest{}, fmt.Errorf("testee has no usable profile_id")
	}
	return req, nil
}

func loadAssessmentEntryCreatedAt(ctx context.Context, mysqlDB *gorm.DB, entryID string) (time.Time, error) {
	var row struct {
		CreatedAt time.Time `gorm:"column:created_at"`
	}
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.AssessmentEntryPO{}).TableName()).
		Select("created_at").
		Where("id = ? AND deleted_at IS NULL", strings.TrimSpace(entryID)).
		Take(&row).Error
	if err != nil {
		return time.Time{}, fmt.Errorf("load assessment_entry created_at: %w", err)
	}
	return row.CreatedAt, nil
}

func backfillAssessmentEntryResolveLogTimes(
	ctx context.Context,
	mysqlDB *gorm.DB,
	orgID int64,
	entryID string,
	clinicianID string,
	resolvedAt time.Time,
) error {
	var row struct {
		ID uint64 `gorm:"column:id"`
	}
	err := mysqlDB.WithContext(ctx).
		Table("assessment_entry_resolve_log").
		Select("id").
		Where("org_id = ? AND entry_id = ? AND clinician_id = ? AND deleted_at IS NULL", orgID, strings.TrimSpace(entryID), strings.TrimSpace(clinicianID)).
		Order("id DESC").
		Take(&row).Error
	if err != nil {
		return fmt.Errorf("load latest assessment_entry_resolve_log row: %w", err)
	}
	err = mysqlDB.WithContext(ctx).
		Table("assessment_entry_resolve_log").
		Where("id = ?", row.ID).
		Updates(map[string]interface{}{
			"resolved_at": resolvedAt,
			"created_at":  resolvedAt,
			"updated_at":  resolvedAt,
		}).Error
	if err != nil {
		return fmt.Errorf("update assessment_entry_resolve_log %d timestamps: %w", row.ID, err)
	}
	return nil
}

func updateAssessmentEntryRelationTimes(ctx context.Context, mysqlDB *gorm.DB, relationID string, boundAt time.Time) error {
	relationID = strings.TrimSpace(relationID)
	if relationID == "" {
		return nil
	}
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", relationID).
		Updates(map[string]interface{}{
			"bound_at":   boundAt,
			"created_at": boundAt,
			"updated_at": boundAt,
		}).Error
	if err != nil {
		return fmt.Errorf("update assessment entry relation %s timestamps: %w", relationID, err)
	}
	return nil
}
