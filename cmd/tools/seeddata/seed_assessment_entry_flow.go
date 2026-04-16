package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func seedAssessmentEntryFlow(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for assessment_entry_flow")
	}

	cfg := deps.Config.AssessmentEntryFlow
	clinicians, err := resolveSeedClinicianScope(ctx, deps, seedClinicianScopeSpec{
		refs:        cfg.ClinicianRefs,
		keyPrefixes: cfg.ClinicianKeyPrefixes,
		ids:         cfg.ClinicianIDs,
	})
	if err != nil {
		return err
	}
	entryFilter := flexibleIDSet(cfg.EntryIDs)
	maxIntakes := normalizeMaxIntakesPerEntry(cfg.MaxIntakesPerEntry)
	activeClinicians := make([]*ClinicianResponse, 0, len(clinicians))
	for _, item := range clinicians {
		if item != nil && item.IsActive {
			activeClinicians = append(activeClinicians, item)
		}
	}

	totalEntries := 0
	totalResolved := 0
	totalSkipped := 0
	totalExpired := 0
	progress := newSeedProgressBar("assessment_entry_flow clinicians", len(activeClinicians))
	defer progress.Close()

	for _, clinicianItem := range activeClinicians {
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
			if isExpiredAssessmentEntry(entry, time.Now()) {
				totalSkipped++
				totalExpired++
				skippedForClinician++
				deps.Logger.Warnw("Skipping expired assessment entry during flow seeding",
					"entry_id", entry.ID,
					"clinician_id", clinicianItem.ID,
					"expires_at", entry.ExpiresAt,
				)
				continue
			}
			totalEntries++

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

				if _, err := deps.APIClient.ResolveAssessmentEntry(ctx, entry.Token); err != nil {
					return fmt.Errorf("resolve assessment entry %s for testee %s: %w", entry.ID, testeeID, err)
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
				_ = intakeResp

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
		progress.Increment()
	}
	progress.Complete()

	deps.Logger.Infow("Assessment entry flow completed",
		"org_id", deps.Config.Global.OrgID,
		"processed_entries", totalEntries,
		"resolved_and_intaked", totalResolved,
		"skipped", totalSkipped,
		"skipped_expired", totalExpired,
		"max_intakes_per_entry", maxIntakes,
		"allow_temporary_testee", cfg.AllowTemporaryTestee,
	)
	return nil
}

func isExpiredAssessmentEntry(entry *AssessmentEntryResponse, now time.Time) bool {
	if entry == nil || entry.ExpiresAt == nil {
		return false
	}
	return !entry.ExpiresAt.After(now)
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
