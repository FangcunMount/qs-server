package dailysim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedapi"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedconfig"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seediauth"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedprofile"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedruntime"
	"github.com/mozillazg/go-pinyin"
)

type Dependencies = seedruntime.Dependencies
type dependencies = Dependencies
type DailySimulationConfig = seedconfig.DailySimulationConfig
type DailySimulationJourneyMixConfig = seedconfig.DailySimulationJourneyMixConfig
type IAMConfig = seedconfig.IAMConfig
type FlexibleID = seedconfig.FlexibleID

type APIClient = seedapi.APIClient
type ScaleResponse = seedapi.ScaleResponse
type PlanResponse = seedapi.PlanResponse
type TaskResponse = seedapi.TaskResponse
type PlanTaskWindowResponse = seedapi.PlanTaskWindowResponse
type ListPlanTaskWindowRequest = seedapi.ListPlanTaskWindowRequest
type ApiserverTesteeResponse = seedapi.ApiserverTesteeResponse
type ApiserverTesteeListResponse = seedapi.ApiserverTesteeListResponse
type TesteeResponse = seedapi.TesteeResponse
type AssessmentEntryResponse = seedapi.AssessmentEntryResponse
type AssessmentEntryListResponse = seedapi.AssessmentEntryListResponse
type RelationResponse = seedapi.RelationResponse
type TesteeClinicianRelationResponse = seedapi.TesteeClinicianRelationResponse
type TesteeClinicianRelationListResponse = seedapi.TesteeClinicianRelationListResponse
type CreateAssessmentEntryRequest = seedapi.CreateAssessmentEntryRequest
type IntakeAssessmentEntryRequest = seedapi.IntakeAssessmentEntryRequest
type AssessmentEntryResolvedResponse = seedapi.AssessmentEntryResolvedResponse
type AssessmentEntryIntakeResponse = seedapi.AssessmentEntryIntakeResponse
type IAMChildResponse = seedapi.IAMChildResponse
type IAMChildPageResponse = seedapi.IAMChildPageResponse
type IAMChildRegisterRequest = seedapi.IAMChildRegisterRequest
type IAMChildRegisterResponse = seedapi.IAMChildRegisterResponse
type CollectionCreateTesteeRequest = seedapi.CollectionCreateTesteeRequest
type CollectionTesteeExistsResponse = seedapi.CollectionTesteeExistsResponse
type CollectionAssessmentDetailResponse = seedapi.CollectionAssessmentDetailResponse
type AssignClinicianTesteeRequest = seedapi.AssignClinicianTesteeRequest
type EnrollTesteeRequest = seedapi.EnrollTesteeRequest
type EnrollmentResponse = seedapi.EnrollmentResponse
type QuestionnaireDetailResponse = seedapi.QuestionnaireDetailResponse
type QuestionResponse = seedapi.QuestionResponse
type OptionResponse = seedapi.OptionResponse
type SubmitAnswerSheetRequest = seedapi.SubmitAnswerSheetRequest
type AdminSubmitAnswerSheetRequest = seedapi.AdminSubmitAnswerSheetRequest
type Answer = seedapi.Answer
type SubmitAnswerSheetResponse = seedapi.SubmitAnswerSheetResponse
type AdminAnswerSheetListItem = seedapi.AdminAnswerSheetListItem
type AdminAnswerSheetListResponse = seedapi.AdminAnswerSheetListResponse
type AssessmentListResponse = seedapi.AssessmentListResponse
type AssessmentResponse = seedapi.AssessmentResponse

var NewAPIClient = seedapi.NewAPIClient

const (
	assessmentEntryListPageSize   = 100
	assessmentListPageSize        = 100
	questionnaireTypeMedicalScale = "MedicalScale"
)

func RunDaemon(ctx context.Context, deps *Dependencies) error {
	return seedDailySimulationDaemon(ctx, deps)
}

func fetchTokenFromIAMWithPassword(
	ctx context.Context,
	loginURL, username, password, tenantID, deviceID string,
	logger log.Logger,
) (string, error) {
	return seediauth.FetchTokenFromIAMWithPassword(ctx, loginURL, username, password, tenantID, deviceID, logger)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	return seedruntime.SleepWithContext(ctx, d)
}

func parseID(raw string) uint64 {
	return seedruntime.ParseID(raw)
}

func nullableString(value *string) string {
	return seedruntime.NullableString(value)
}

func parseSeedRelativeDuration(raw string) (time.Duration, error) {
	return seedruntime.ParseRelativeDuration(raw)
}

func listAllClinicianAssessmentEntries(ctx context.Context, client *APIClient, clinicianID string) ([]*AssessmentEntryResponse, error) {
	page := 1
	items := make([]*AssessmentEntryResponse, 0, assessmentEntryListPageSize)
	for {
		resp, err := client.ListClinicianAssessmentEntries(ctx, clinicianID, page, assessmentEntryListPageSize)
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

func assessmentEntryTargetKey(targetType, targetCode, targetVersion string) string {
	cfg := CreateAssessmentEntryRequest{
		TargetType:    strings.ToLower(strings.TrimSpace(targetType)),
		TargetCode:    strings.TrimSpace(targetCode),
		TargetVersion: strings.TrimSpace(targetVersion),
	}
	return fmt.Sprintf("%s:%s@%s", cfg.TargetType, cfg.TargetCode, cfg.TargetVersion)
}

func buildSeedProfile(cfg DailySimulationConfig, runDate time.Time, idx int) dailySimulationProfile {
	generator := seedprofile.New(cfg.UserPhonePrefix, cfg.UserEmailDomain)
	profile := generator.Generate(runDate, idx)
	return dailySimulationProfile(profile)
}

func normalizePhone(value string) string {
	return strings.TrimSpace(value)
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func buildGeneratedClinicianEmailLocal(name string) (string, error) {
	value := strings.TrimSpace(name)
	if value == "" {
		return "", fmt.Errorf("empty name")
	}
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal
	parts := pinyin.LazyPinyin(value, args)
	if len(parts) == 0 {
		parts = []string{value}
	}
	var builder strings.Builder
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				builder.WriteRune(r)
			}
		}
	}
	if builder.Len() == 0 {
		return "", fmt.Errorf("empty normalized pinyin result")
	}
	return builder.String(), nil
}
