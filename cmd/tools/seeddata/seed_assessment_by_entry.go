package main

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"gorm.io/gorm"
)

type entryAssessmentCandidateRow struct {
	EntryID         uint64    `gorm:"column:entry_id"`
	ClinicianID     uint64    `gorm:"column:clinician_id"`
	TesteeID        uint64    `gorm:"column:testee_id"`
	BoundAt         time.Time `gorm:"column:bound_at"`
	TesteeCreatedAt time.Time `gorm:"column:testee_created_at"`
	TargetType      string    `gorm:"column:target_type"`
	TargetCode      string    `gorm:"column:target_code"`
	TargetVersion   *string   `gorm:"column:target_version"`
}

func seedAssessmentByEntry(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for assessment_by_entry")
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for assessment_by_entry")
	}
	if deps.CollectionClient == nil {
		return fmt.Errorf("collection client is not initialized")
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after assessment_by_entry", "error", closeErr.Error())
		}
	}()

	cfg := deps.Config.AssessmentByEntry
	clinicians, err := resolveSeedClinicianScope(ctx, deps, seedClinicianScopeSpec{
		refs:        cfg.ClinicianRefs,
		keyPrefixes: cfg.ClinicianKeyPrefixes,
		ids:         cfg.ClinicianIDs,
	})
	if err != nil {
		return err
	}
	clinicianIDs := make([]uint64, 0, len(clinicians))
	for _, item := range clinicians {
		if item == nil {
			continue
		}
		if parsed := parseID(item.ID); parsed > 0 {
			clinicianIDs = append(clinicianIDs, parsed)
		}
	}
	entryIDs := make([]uint64, 0, len(cfg.EntryIDs))
	for _, id := range nonZeroFlexibleIDs(cfg.EntryIDs) {
		if parsed, err := id.Uint64(); err == nil && parsed > 0 {
			entryIDs = append(entryIDs, parsed)
		}
	}

	candidates, err := loadEntryAssessmentCandidates(ctx, mysqlDB, deps.Config.Global.OrgID, clinicianIDs, entryIDs)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		deps.Logger.Infow("No eligible entry-based assessment candidates found", "org_id", deps.Config.Global.OrgID)
		return nil
	}

	maxPerEntry := normalizeMaxAssessmentsPerEntry(cfg.MaxAssessmentsPerEntry)
	entryCounts := make(map[uint64]int, len(candidates))
	questionnaireCache := make(map[string]*QuestionnaireDetailResponse)
	var questionnaireCacheMu sync.RWMutex

	createdCount := 0
	skippedCount := 0
	progress := newSeedProgressBar("assessment_by_entry candidates", len(candidates))
	defer progress.Close()
	for _, candidate := range candidates {
		if err := func(candidate entryAssessmentCandidateRow) error {
			defer progress.Increment()

			if entryCounts[candidate.EntryID] >= maxPerEntry {
				skippedCount++
				return nil
			}

			target, err := resolveEntryAssessmentTarget(ctx, deps.APIClient, candidate)
			if err != nil {
				deps.Logger.Warnw("Skipping entry-based assessment because target resolution failed",
					"entry_id", candidate.EntryID,
					"testee_id", candidate.TesteeID,
					"error", err.Error(),
				)
				skippedCount++
				return nil
			}
			if target.SkipReason != "" {
				deps.Logger.Infow("Skipping entry-based assessment",
					"entry_id", candidate.EntryID,
					"testee_id", candidate.TesteeID,
					"reason", target.SkipReason,
				)
				skippedCount++
				return nil
			}

			exists, err := assessmentExistsForEntryCandidate(ctx, mysqlDB, deps.Config.Global.OrgID, candidate, target.QuestionnaireCode, target.QuestionnaireVersion)
			if err != nil {
				return err
			}
			if exists {
				skippedCount++
				return nil
			}

			detail := getQuestionnaireDetail(ctx, deps.APIClient, target.QuestionnaireCode, questionnaireCache, &questionnaireCacheMu, deps.Logger)
			if detail == nil {
				skippedCount++
				return nil
			}
			if detail.Type != questionnaireTypeMedicalScale {
				skippedCount++
				return nil
			}

			rng := rand.New(rand.NewSource(int64(candidate.TesteeID) + int64(candidate.EntryID)))
			answers := buildAnswers(detail, rng)
			if len(answers) == 0 {
				skippedCount++
				return nil
			}

			submitReq := SubmitAnswerSheetRequest{
				QuestionnaireCode:    target.QuestionnaireCode,
				QuestionnaireVersion: target.QuestionnaireVersion,
				Title:                detail.Title,
				TesteeID:             candidate.TesteeID,
				Answers:              answers,
			}
			submitResp, err := deps.APIClient.SubmitAnswerSheetAdmin(ctx, buildAdminSubmitAnswerSheetRequest(submitReq))
			if err != nil {
				return fmt.Errorf("submit entry-based answersheet entry=%d testee=%d: %w", candidate.EntryID, candidate.TesteeID, err)
			}

			answerSheetID := parseID(submitResp.ID)
			if answerSheetID == 0 {
				return fmt.Errorf("invalid answersheet id after entry-based submit: %s", submitResp.ID)
			}
			assessmentRow, err := waitForAssessmentByAnswerSheet(ctx, mysqlDB, answerSheetID)
			if err != nil {
				return fmt.Errorf("wait for assessment by answersheet %d: %w", answerSheetID, err)
			}

			entryCounts[candidate.EntryID]++
			createdCount++
			deps.Logger.Debugw("Entry-based assessment created",
				"entry_id", candidate.EntryID,
				"testee_id", candidate.TesteeID,
				"answersheet_id", answerSheetID,
				"assessment_id", assessmentRow.ID,
				"assessment_status", assessmentRow.Status,
			)
			return nil
		}(candidate); err != nil {
			return err
		}
	}
	progress.Complete()

	deps.Logger.Infow("Assessment-by-entry seeding completed",
		"org_id", deps.Config.Global.OrgID,
		"candidate_count", len(candidates),
		"created", createdCount,
		"skipped", skippedCount,
		"max_assessments_per_entry", maxPerEntry,
	)
	return nil
}

type resolvedEntryAssessmentTarget struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	MedicalScaleID       *uint64
	MedicalScaleCode     *string
	MedicalScaleName     *string
	SkipReason           string
}

func resolveEntryAssessmentTarget(ctx context.Context, client *APIClient, candidate entryAssessmentCandidateRow) (*resolvedEntryAssessmentTarget, error) {
	targetType := strings.ToLower(strings.TrimSpace(candidate.TargetType))
	targetVersion := strings.TrimSpace(nullableString(candidate.TargetVersion))
	switch targetType {
	case "scale":
		scaleItem, err := client.GetScale(ctx, candidate.TargetCode)
		if err != nil {
			return nil, fmt.Errorf("get scale %s: %w", candidate.TargetCode, err)
		}
		if scaleItem == nil {
			return &resolvedEntryAssessmentTarget{SkipReason: "scale not found"}, nil
		}
		questionnaireVersion := scaleItem.QuestionnaireVersion
		if questionnaireVersion == "" {
			questionnaireVersion = targetVersion
		}
		code := scaleItem.Code
		name := scaleItem.Title
		return &resolvedEntryAssessmentTarget{
			QuestionnaireCode:    scaleItem.QuestionnaireCode,
			QuestionnaireVersion: questionnaireVersion,
			MedicalScaleCode:     &code,
			MedicalScaleName:     &name,
		}, nil
	case "questionnaire":
		detail, err := client.GetQuestionnaireDetail(ctx, candidate.TargetCode)
		if err != nil {
			return nil, fmt.Errorf("get questionnaire %s: %w", candidate.TargetCode, err)
		}
		if detail == nil {
			return &resolvedEntryAssessmentTarget{SkipReason: "questionnaire not found"}, nil
		}
		if detail.Type != questionnaireTypeMedicalScale {
			return &resolvedEntryAssessmentTarget{SkipReason: "questionnaire is not medical-scale"}, nil
		}
		questionnaireVersion := detail.Version
		if targetVersion != "" {
			questionnaireVersion = targetVersion
		}
		return &resolvedEntryAssessmentTarget{
			QuestionnaireCode:    candidate.TargetCode,
			QuestionnaireVersion: questionnaireVersion,
		}, nil
	default:
		return &resolvedEntryAssessmentTarget{SkipReason: fmt.Sprintf("unsupported target type %q", candidate.TargetType)}, nil
	}
}

func loadEntryAssessmentCandidates(
	ctx context.Context,
	mysqlDB *gorm.DB,
	orgID int64,
	clinicianIDs []uint64,
	entryIDs []uint64,
) ([]entryAssessmentCandidateRow, error) {
	rows := make([]entryAssessmentCandidateRow, 0, 128)
	query := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()+" AS cr").
		Select("cr.source_id AS entry_id, cr.clinician_id, cr.testee_id, cr.bound_at, t.created_at AS testee_created_at, ae.target_type, ae.target_code, ae.target_version").
		Joins("JOIN "+(actorMySQL.AssessmentEntryPO{}).TableName()+" AS ae ON ae.id = cr.source_id AND ae.deleted_at IS NULL").
		Joins("JOIN "+(actorMySQL.TesteePO{}).TableName()+" AS t ON t.id = cr.testee_id AND t.deleted_at IS NULL").
		Where("cr.org_id = ? AND cr.deleted_at IS NULL AND cr.is_active = 1", orgID).
		Where("cr.source_type = ? AND cr.relation_type = ?", "assessment_entry", "creator").
		Order("cr.source_id ASC, t.created_at ASC, cr.id ASC")
	if len(clinicianIDs) > 0 {
		query = query.Where("cr.clinician_id IN ?", clinicianIDs)
	}
	if len(entryIDs) > 0 {
		query = query.Where("cr.source_id IN ?", entryIDs)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load entry assessment candidates: %w", err)
	}
	return rows, nil
}

func assessmentExistsForEntryCandidate(
	ctx context.Context,
	mysqlDB *gorm.DB,
	orgID int64,
	candidate entryAssessmentCandidateRow,
	questionnaireCode string,
	questionnaireVersion string,
) (bool, error) {
	var count int64
	err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Where("org_id = ? AND testee_id = ? AND questionnaire_code = ? AND questionnaire_version = ? AND deleted_at IS NULL", orgID, candidate.TesteeID, questionnaireCode, questionnaireVersion).
		Where("created_at >= ?", candidate.BoundAt).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check assessment exists for entry %d testee %d: %w", candidate.EntryID, candidate.TesteeID, err)
	}
	return count > 0, nil
}

func waitForAssessmentByAnswerSheet(ctx context.Context, mysqlDB *gorm.DB, answerSheetID uint64) (planFixupAssessmentRow, error) {
	deadline := time.Now().Add(seedAssessmentPollTimeout)
	for {
		row, err := loadAssessmentByAnswerSheet(ctx, mysqlDB, answerSheetID)
		if err == nil {
			return row, nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return planFixupAssessmentRow{}, fmt.Errorf("load assessment by answersheet %d: %w", answerSheetID, err)
		}
		if time.Now().After(deadline) {
			return planFixupAssessmentRow{}, fmt.Errorf("assessment not found by answersheet %d before timeout", answerSheetID)
		}
		select {
		case <-ctx.Done():
			return planFixupAssessmentRow{}, ctx.Err()
		case <-time.After(seedAssessmentPollInterval):
		}
	}
}

func loadAssessmentByAnswerSheet(ctx context.Context, mysqlDB *gorm.DB, answerSheetID uint64) (planFixupAssessmentRow, error) {
	var row planFixupAssessmentRow
	err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Select("id, answer_sheet_id, status, created_at, updated_at, submitted_at, interpreted_at, failed_at").
		Where("answer_sheet_id = ? AND deleted_at IS NULL", answerSheetID).
		Order("id DESC").
		Take(&row).Error
	if err != nil {
		return planFixupAssessmentRow{}, err
	}
	return row, nil
}
