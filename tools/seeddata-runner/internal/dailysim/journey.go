package dailysim

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	authnv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authn/v1"
	identityv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/identity/v1"
	sdk "github.com/FangcunMount/iam-contracts/pkg/sdk"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
	sdkerrors "github.com/FangcunMount/iam-contracts/pkg/sdk/errors"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/identity"
	toolchain "github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/chain"
)

const (
	dailySimulationDefaultCount     = 10
	dailySimulationDefaultWorkers   = 4
	dailySimulationChildPageLimit   = 100
	dailySimulationAnswerSheetPage  = 100
	dailySimulationDefaultPhonePref = "+86199"
	dailySimulationDefaultEmailHost = "fangcunmount.com"
	dailySimulationDefaultPassword  = "DailySim@123"
	dailySimulationDefaultSource    = "daily_simulation"
	dailySimulationDeviceIDPrefix   = "seeddata-daily"
	seedAssessmentPollTimeout       = 5 * time.Minute
	seedAssessmentPollInterval      = 2 * time.Second
)

type dailySimulationIAMBundle struct {
	client   *sdk.Client
	identity *identity.Client
	auth     *auth.Client
}

type dailySimulationResolvedTarget struct {
	TargetType           string
	TargetCode           string
	TargetVersion        string
	QuestionnaireCode    string
	QuestionnaireVersion string
	QuestionnaireTitle   string
	QuestionnaireDetail  *QuestionnaireDetailResponse
	RequiresAssessment   bool
}

type dailySimulationProfile struct {
	Index         int
	RunDate       time.Time
	GuardianName  string
	GuardianPhone string
	GuardianEmail string
	ChildName     string
	ChildDOB      string
	ChildGender   uint8
}

type dailySimulationOutcome struct {
	UserCreated       bool
	ChildCreated      bool
	TesteeCreated     bool
	PlanEnrolled      bool
	PlanID            string
	EntryResolved     bool
	EntryIntaked      bool
	AnswerSheetID     string
	AssessmentID      string
	SkippedSubmission bool
	JourneyTarget     string
	StopReason        string
}

type dailySimulationCounters struct {
	userCreated       int64
	childCreated      int64
	testeeCreated     int64
	enrolled          int64
	resolved          int64
	intaked           int64
	submitted         int64
	skippedSubmission int64
	assessmentCreated int64
	failed            int64
}

type dailySimulationJourneyTarget string

const (
	dailySimulationJourneyRegisterOnly dailySimulationJourneyTarget = "register_only"
	dailySimulationJourneyCreateTestee dailySimulationJourneyTarget = "create_testee"
	dailySimulationJourneyResolveEntry dailySimulationJourneyTarget = "resolve_entry"
	dailySimulationJourneySubmitAnswer dailySimulationJourneyTarget = "submit_answer"
)

type dailySimulationJourneyStage string

const (
	dailySimulationJourneyStageGuardianAccount dailySimulationJourneyStage = "guardian_account"
	dailySimulationJourneyStageTesteeProfile   dailySimulationJourneyStage = "testee_profile"
	dailySimulationJourneyStagePlanEnrollment  dailySimulationJourneyStage = "plan_enrollment"
	dailySimulationJourneyStageAssessmentEntry dailySimulationJourneyStage = "assessment_entry"
	dailySimulationJourneyStageAnswerSheet     dailySimulationJourneyStage = "answersheet_submit"
)

type dailySimulationJourneyState struct {
	deps        *dependencies
	iamBundle   *dailySimulationIAMBundle
	cfg         DailySimulationConfig
	profile     dailySimulationProfile
	clinicianID string
	entry       *AssessmentEntryResponse
	target      *dailySimulationResolvedTarget
	planID      string

	journeyTarget    dailySimulationJourneyTarget
	guardianUserID   string
	guardianToken    string
	userClient       *APIClient
	collectionClient *APIClient
	child            *IAMChildResponse
	testee           *TesteeResponse
	outcome          dailySimulationOutcome
}

func (c *dailySimulationCounters) add(outcome dailySimulationOutcome) {
	if outcome.UserCreated {
		atomic.AddInt64(&c.userCreated, 1)
	}
	if outcome.ChildCreated {
		atomic.AddInt64(&c.childCreated, 1)
	}
	if outcome.TesteeCreated {
		atomic.AddInt64(&c.testeeCreated, 1)
	}
	if outcome.PlanEnrolled {
		atomic.AddInt64(&c.enrolled, 1)
	}
	if outcome.EntryResolved {
		atomic.AddInt64(&c.resolved, 1)
	}
	if outcome.EntryIntaked {
		atomic.AddInt64(&c.intaked, 1)
	}
	if strings.TrimSpace(outcome.AnswerSheetID) != "" {
		atomic.AddInt64(&c.submitted, 1)
	}
	if outcome.SkippedSubmission {
		atomic.AddInt64(&c.skippedSubmission, 1)
	}
	if strings.TrimSpace(outcome.AssessmentID) != "" {
		atomic.AddInt64(&c.assessmentCreated, 1)
	}
}

func (c *dailySimulationCounters) addFailure() {
	atomic.AddInt64(&c.failed, 1)
}

/**
 * 模拟每日用户
 *
 * @param ctx 上下文
 * @param deps 依赖
 * @param iamBundle IAM 绑定
 * @param cfg 配置
 * @param profile 用户信息
 */
func simulateDailyUser(
	ctx context.Context,
	deps *dependencies,
	iamBundle *dailySimulationIAMBundle,
	cfg DailySimulationConfig,
	profile dailySimulationProfile,
	clinicianID string,
	entry *AssessmentEntryResponse,
	target *dailySimulationResolvedTarget,
) (dailySimulationOutcome, error) {
	state := &dailySimulationJourneyState{
		deps:          deps,
		iamBundle:     iamBundle,
		cfg:           cfg,
		profile:       profile,
		clinicianID:   clinicianID,
		entry:         entry,
		target:        target,
		planID:        selectDailySimulationPlanID(cfg, profile.RunDate, profile.Index),
		journeyTarget: resolveDailySimulationJourneyTarget(cfg, profile.RunDate, profile.Index),
	}

	state.outcome.JourneyTarget = string(state.journeyTarget)
	state.outcome.PlanID = state.planID

	decision, err := toolchain.Run(ctx, "daily_simulation_user", state,
		toolchain.FuncHandler[dailySimulationJourneyState]{HandlerName: string(dailySimulationJourneyStageGuardianAccount), HandlerFunc: dailySimulationStageEnsureGuardianAccount},
		toolchain.FuncHandler[dailySimulationJourneyState]{HandlerName: "child_profile", HandlerFunc: dailySimulationStageEnsureChild},
		toolchain.FuncHandler[dailySimulationJourneyState]{HandlerName: string(dailySimulationJourneyStageTesteeProfile), HandlerFunc: dailySimulationStageEnsureTestee},
		toolchain.FuncHandler[dailySimulationJourneyState]{HandlerName: string(dailySimulationJourneyStagePlanEnrollment), HandlerFunc: dailySimulationStageEnrollPlan},
		toolchain.FuncHandler[dailySimulationJourneyState]{HandlerName: string(dailySimulationJourneyStageAssessmentEntry), HandlerFunc: dailySimulationStageEnsureEntryAccess},
		toolchain.FuncHandler[dailySimulationJourneyState]{HandlerName: string(dailySimulationJourneyStageAnswerSheet), HandlerFunc: dailySimulationStageSubmitAnswerSheet},
	)
	if err != nil {
		return state.outcome, err
	}
	if state.outcome.StopReason == "" {
		if decision.StopReason != "" {
			state.outcome.StopReason = decision.StopReason
		} else {
			state.outcome.StopReason = "completed"
		}
	}

	return state.outcome, logDailySimulationOutcome(
		deps,
		profile,
		clinicianID,
		entry,
		target,
		state.testee.ID,
		state.guardianUserID,
		state.outcome,
	)
}

func dailySimulationStageEnsureGuardianAccount(ctx context.Context, state *dailySimulationJourneyState) (toolchain.Decision, error) {
	guardianUserID, guardianToken, userCreated, err := ensureDailySimulationGuardianAccount(
		ctx,
		state.deps,
		state.iamBundle,
		state.cfg,
		state.profile,
	)
	if err != nil {
		return toolchain.Decision{}, err
	}

	state.guardianUserID = guardianUserID
	state.guardianToken = guardianToken
	state.outcome.UserCreated = userCreated

	state.userClient = NewAPIClient(state.deps.APIClient.BaseURL(), guardianToken, state.deps.Logger)
	state.userClient.SetRetryConfig(state.deps.Config.API.Retry)
	state.collectionClient = NewAPIClient(state.deps.CollectionClient.BaseURL(), guardianToken, state.deps.Logger)
	state.collectionClient.SetRetryConfig(state.deps.Config.API.Retry)
	return state.nextDecision(dailySimulationJourneyStageGuardianAccount), nil
}

func dailySimulationStageEnsureChild(ctx context.Context, state *dailySimulationJourneyState) (toolchain.Decision, error) {
	child, childCreated, err := ensureDailySimulationChild(ctx, state.userClient, state.cfg, state.profile)
	if err != nil {
		return toolchain.Decision{}, err
	}
	state.child = child
	state.outcome.ChildCreated = childCreated
	return toolchain.Next(), nil
}

func dailySimulationStageEnsureTestee(ctx context.Context, state *dailySimulationJourneyState) (toolchain.Decision, error) {
	testee, testeeCreated, err := ensureDailySimulationTestee(
		ctx,
		state.collectionClient,
		state.guardianUserID,
		state.cfg,
		state.profile,
		state.child,
	)
	if err != nil {
		return toolchain.Decision{}, err
	}
	state.testee = testee
	state.outcome.TesteeCreated = testeeCreated
	return state.nextDecision(dailySimulationJourneyStageTesteeProfile), nil
}

func dailySimulationStageEnrollPlan(ctx context.Context, state *dailySimulationJourneyState) (toolchain.Decision, error) {
	if strings.TrimSpace(state.planID) == "" {
		return toolchain.Decision{}, fmt.Errorf("dailySimulation.planIds resolved empty plan")
	}
	if _, err := state.deps.APIClient.EnrollTesteeInPlan(ctx, EnrollTesteeRequest{
		PlanID:    state.planID,
		TesteeID:  state.testee.ID,
		StartDate: state.profile.RunDate.Format("2006-01-02"),
	}); err != nil {
		return toolchain.Decision{}, err
	}
	state.outcome.PlanEnrolled = true
	return toolchain.Next(), nil
}

func dailySimulationStageEnsureEntryAccess(ctx context.Context, state *dailySimulationJourneyState) (toolchain.Decision, error) {
	hasCreator, err := hasAssessmentEntryCreatorRelation(ctx, state.deps.APIClient, state.testee.ID, state.entry.ID)
	if err != nil {
		return toolchain.Decision{}, err
	}
	if hasCreator {
		return state.nextDecision(dailySimulationJourneyStageAssessmentEntry), nil
	}

	if _, err := state.deps.APIClient.ResolveAssessmentEntry(ctx, state.entry.Token); err != nil {
		return toolchain.Decision{}, err
	}
	state.outcome.EntryResolved = true

	childID := parseID(state.child.ID)
	if childID == 0 {
		return toolchain.Decision{}, fmt.Errorf("invalid child id %q", state.child.ID)
	}
	birthday, err := parseDailySimulationDOB(state.child.DOB)
	if err != nil {
		return toolchain.Decision{}, fmt.Errorf("parse child dob %q: %w", state.child.DOB, err)
	}
	intakeResp, err := state.deps.APIClient.IntakeAssessmentEntry(ctx, state.entry.Token, IntakeAssessmentEntryRequest{
		ProfileID: &childID,
		Name:      state.child.LegalName,
		Gender:    dailySimulationAPIGender(state.child.Gender),
		Birthday:  birthday,
	})
	if err != nil {
		return toolchain.Decision{}, err
	}
	if intakeResp.Testee != nil && strings.TrimSpace(intakeResp.Testee.ID) != "" {
		state.testee.ID = intakeResp.Testee.ID
	}
	state.outcome.EntryIntaked = true
	return state.nextDecision(dailySimulationJourneyStageAssessmentEntry), nil
}

func dailySimulationStageSubmitAnswerSheet(ctx context.Context, state *dailySimulationJourneyState) (toolchain.Decision, error) {
	existingAnswerSheet, err := findDailySimulationAnswerSheet(
		ctx,
		state.deps.APIClient,
		state.target.QuestionnaireCode,
		state.guardianUserID,
		state.testee.ID,
	)
	if err != nil {
		return toolchain.Decision{}, err
	}
	existingAssessmentID, err := findDailySimulationAssessment(
		ctx,
		state.deps.APIClient,
		state.testee.ID,
		state.target.QuestionnaireCode,
		state.target.QuestionnaireVersion,
	)
	if err != nil {
		return toolchain.Decision{}, err
	}
	if strings.TrimSpace(existingAssessmentID) != "" {
		state.outcome.AssessmentID = existingAssessmentID
		state.outcome.SkippedSubmission = true
		return state.nextDecision(dailySimulationJourneyStageAnswerSheet), nil
	}
	if existingAnswerSheet != nil && !state.target.RequiresAssessment {
		state.outcome.AnswerSheetID = existingAnswerSheet.ID
		state.outcome.SkippedSubmission = true
		return state.nextDecision(dailySimulationJourneyStageAnswerSheet), nil
	}
	if existingAnswerSheet != nil && state.target.RequiresAssessment {
		assessmentID, waitErr := waitForDailySimulationAssessment(ctx, state.collectionClient, existingAnswerSheet.ID)
		if waitErr == nil {
			state.outcome.AnswerSheetID = existingAnswerSheet.ID
			state.outcome.AssessmentID = assessmentID
			state.outcome.SkippedSubmission = true
			return state.nextDecision(dailySimulationJourneyStageAnswerSheet), nil
		}
	}

	testeeID := parseID(state.testee.ID)
	if testeeID == 0 {
		return toolchain.Decision{}, fmt.Errorf("invalid testee id %q", state.testee.ID)
	}
	questionnaireDetail := state.target.QuestionnaireDetail
	if questionnaireDetail == nil {
		return toolchain.Decision{}, fmt.Errorf("questionnaire detail for %s is not preloaded", state.target.QuestionnaireCode)
	}

	rng := newDailySimulationRand(
		"answers:" + state.profile.RunDate.Format("20060102") + ":" + strconv.Itoa(state.profile.Index) + ":" + state.target.QuestionnaireCode,
	)
	answers := buildAnswers(questionnaireDetail, rng)
	submitResp, err := state.collectionClient.SubmitAnswerSheet(ctx, SubmitAnswerSheetRequest{
		QuestionnaireCode:    state.target.QuestionnaireCode,
		QuestionnaireVersion: state.target.QuestionnaireVersion,
		Title:                state.target.QuestionnaireTitle,
		TesteeID:             testeeID,
		Answers:              answers,
	})
	if err != nil {
		return toolchain.Decision{}, err
	}
	state.outcome.AnswerSheetID = submitResp.ID

	if state.target.RequiresAssessment {
		assessmentID, err := waitForDailySimulationAssessment(ctx, state.collectionClient, submitResp.ID)
		if err != nil {
			return toolchain.Decision{}, err
		}
		state.outcome.AssessmentID = assessmentID
	}
	return state.nextDecision(dailySimulationJourneyStageAnswerSheet), nil
}

func (state *dailySimulationJourneyState) nextDecision(stage dailySimulationJourneyStage) toolchain.Decision {
	if shouldStopDailySimulationJourneyAfter(state.journeyTarget, stage) {
		reason := "target_reached:" + string(state.journeyTarget)
		state.outcome.StopReason = reason
		return toolchain.Stop(reason)
	}
	return toolchain.Next()
}

func shouldStopDailySimulationJourneyAfter(target dailySimulationJourneyTarget, stage dailySimulationJourneyStage) bool {
	switch target {
	case dailySimulationJourneyRegisterOnly:
		return stage == dailySimulationJourneyStageGuardianAccount
	case dailySimulationJourneyCreateTestee:
		return stage == dailySimulationJourneyStageTesteeProfile
	case dailySimulationJourneyResolveEntry:
		return stage == dailySimulationJourneyStageAssessmentEntry
	case dailySimulationJourneySubmitAnswer:
		return stage == dailySimulationJourneyStageAnswerSheet
	default:
		return stage == dailySimulationJourneyStageAnswerSheet
	}
}

func resolveDailySimulationJourneyTarget(cfg DailySimulationConfig, runDate time.Time, index int) dailySimulationJourneyTarget {
	mix := normalizeDailySimulationJourneyMix(cfg.JourneyMix)
	totalWeight := totalDailySimulationJourneyWeight(mix)
	if totalWeight <= 0 {
		return dailySimulationJourneySubmitAnswer
	}

	bucket := int(newDailySimulationRand(
		fmt.Sprintf("journey:%s:%d", runDate.Format("20060102"), index),
	).Int63n(int64(totalWeight)))
	switch {
	case bucket < mix.RegisterOnlyWeight:
		return dailySimulationJourneyRegisterOnly
	case bucket < mix.RegisterOnlyWeight+mix.CreateTesteeWeight:
		return dailySimulationJourneyCreateTestee
	case bucket < mix.RegisterOnlyWeight+mix.CreateTesteeWeight+mix.ResolveEntryWeight:
		return dailySimulationJourneyResolveEntry
	default:
		return dailySimulationJourneySubmitAnswer
	}
}

func normalizeDailySimulationJourneyMix(cfg DailySimulationJourneyMixConfig) DailySimulationJourneyMixConfig {
	if cfg.RegisterOnlyWeight < 0 {
		cfg.RegisterOnlyWeight = 0
	}
	if cfg.CreateTesteeWeight < 0 {
		cfg.CreateTesteeWeight = 0
	}
	if cfg.ResolveEntryWeight < 0 {
		cfg.ResolveEntryWeight = 0
	}
	if cfg.SubmitAnswerWeight < 0 {
		cfg.SubmitAnswerWeight = 0
	}
	if totalDailySimulationJourneyWeight(cfg) == 0 {
		cfg.SubmitAnswerWeight = 100
	}
	return cfg
}

func totalDailySimulationJourneyWeight(cfg DailySimulationJourneyMixConfig) int {
	return cfg.RegisterOnlyWeight + cfg.CreateTesteeWeight + cfg.ResolveEntryWeight + cfg.SubmitAnswerWeight
}

func logDailySimulationOutcome(
	deps *dependencies,
	profile dailySimulationProfile,
	clinicianID string,
	entry *AssessmentEntryResponse,
	target *dailySimulationResolvedTarget,
	testeeID string,
	guardianUserID string,
	outcome dailySimulationOutcome,
) error {
	deps.Logger.Infow("Daily simulation user completed",
		"index", profile.Index,
		"run_date", profile.RunDate.Format("2006-01-02"),
		"guardian_name", profile.GuardianName,
		"guardian_phone", profile.GuardianPhone,
		"guardian_email", profile.GuardianEmail,
		"guardian_user_id", guardianUserID,
		"child_name", profile.ChildName,
		"child_dob", profile.ChildDOB,
		"testee_id", testeeID,
		"clinician_id", clinicianID,
		"plan_id", outcome.PlanID,
		"entry_id", entry.ID,
		"target_type", target.TargetType,
		"target_code", target.TargetCode,
		"user_created", outcome.UserCreated,
		"child_created", outcome.ChildCreated,
		"testee_created", outcome.TesteeCreated,
		"plan_enrolled", outcome.PlanEnrolled,
		"entry_resolved", outcome.EntryResolved,
		"entry_intaked", outcome.EntryIntaked,
		"answersheet_id", outcome.AnswerSheetID,
		"assessment_id", outcome.AssessmentID,
		"submission_skipped", outcome.SkippedSubmission,
		"journey_target", outcome.JourneyTarget,
		"stop_reason", outcome.StopReason,
	)
	return nil
}

func ensureDailySimulationEntryAndTarget(
	ctx context.Context,
	deps *dependencies,
	cfg DailySimulationConfig,
) (*AssessmentEntryResponse, *dailySimulationResolvedTarget, string, error) {
	var clinicianID string

	if !cfg.EntryID.IsZero() {
		entry, err := deps.APIClient.GetAssessmentEntry(ctx, cfg.EntryID.String())
		if err != nil {
			return nil, nil, "", fmt.Errorf("get daily simulation entry %s: %w", cfg.EntryID.String(), err)
		}
		if entry == nil {
			return nil, nil, "", fmt.Errorf("daily simulation entry %s not found", cfg.EntryID.String())
		}
		if !entry.IsActive {
			entry, err = deps.APIClient.ReactivateAssessmentEntry(ctx, entry.ID)
			if err != nil {
				return nil, nil, "", fmt.Errorf("reactivate daily simulation entry %s: %w", entry.ID, err)
			}
		}
		clinicianID = strings.TrimSpace(entry.ClinicianID)
		if clinicianID == "" {
			return nil, nil, "", fmt.Errorf("daily simulation entry %s has empty clinician_id", entry.ID)
		}
		target, err := resolveDailySimulationTarget(ctx, deps.CollectionClient, entry.TargetType, entry.TargetCode, entry.TargetVersion)
		if err != nil {
			return nil, nil, "", err
		}
		return entry, target, clinicianID, nil
	}

	clinicianIDs := collectDailySimulationClinicianIDs(cfg.ClinicianIDs)
	if len(clinicianIDs) == 0 {
		return nil, nil, "", fmt.Errorf("dailySimulation clinicianIds is required when entryId is not set")
	}
	clinicianID = clinicianIDs[0]

	targetType := strings.ToLower(strings.TrimSpace(cfg.TargetType))
	targetCode := strings.TrimSpace(cfg.TargetCode)
	targetVersion := strings.TrimSpace(cfg.TargetVersion)
	if targetType == "" || targetCode == "" {
		return nil, nil, "", fmt.Errorf("dailySimulation targetType and targetCode are required when entryId is not set")
	}

	entries, err := listAllClinicianAssessmentEntries(ctx, deps.APIClient, clinicianID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("list daily simulation clinician assessment entries: %w", err)
	}
	targetKey := assessmentEntryTargetKey(targetType, targetCode, targetVersion)
	for _, item := range entries {
		if item == nil {
			continue
		}
		if assessmentEntryTargetKey(item.TargetType, item.TargetCode, item.TargetVersion) != targetKey {
			continue
		}
		if !item.IsActive {
			item, err = deps.APIClient.ReactivateAssessmentEntry(ctx, item.ID)
			if err != nil {
				return nil, nil, "", fmt.Errorf("reactivate daily simulation entry %s: %w", item.ID, err)
			}
		}
		target, err := resolveDailySimulationTarget(ctx, deps.CollectionClient, item.TargetType, item.TargetCode, item.TargetVersion)
		if err != nil {
			return nil, nil, "", err
		}
		return item, target, clinicianID, nil
	}

	entry, err := deps.APIClient.CreateClinicianAssessmentEntry(ctx, clinicianID, CreateAssessmentEntryRequest{
		TargetType:    targetType,
		TargetCode:    targetCode,
		TargetVersion: targetVersion,
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("create daily simulation entry: %w", err)
	}
	target, err := resolveDailySimulationTarget(ctx, deps.CollectionClient, entry.TargetType, entry.TargetCode, entry.TargetVersion)
	if err != nil {
		return nil, nil, "", err
	}
	return entry, target, clinicianID, nil
}

func resolveDailySimulationTarget(
	ctx context.Context,
	client *APIClient,
	targetType, targetCode, targetVersion string,
) (*dailySimulationResolvedTarget, error) {
	targetType = strings.ToLower(strings.TrimSpace(targetType))
	targetCode = strings.TrimSpace(targetCode)
	targetVersion = strings.TrimSpace(targetVersion)
	if targetType == "" || targetCode == "" {
		return nil, fmt.Errorf("daily simulation targetType and targetCode are required")
	}

	switch targetType {
	case "scale":
		scaleItem, err := client.GetScale(ctx, targetCode)
		if err != nil {
			return nil, fmt.Errorf("get scale %s: %w", targetCode, err)
		}
		if scaleItem == nil {
			return nil, fmt.Errorf("scale %s not found", targetCode)
		}
		detail, err := client.GetQuestionnaireDetail(ctx, scaleItem.QuestionnaireCode)
		if err != nil {
			return nil, fmt.Errorf("get questionnaire %s for scale %s: %w", scaleItem.QuestionnaireCode, targetCode, err)
		}
		version := strings.TrimSpace(scaleItem.QuestionnaireVersion)
		if version == "" {
			version = strings.TrimSpace(detail.Version)
		}
		return &dailySimulationResolvedTarget{
			TargetType:           targetType,
			TargetCode:           targetCode,
			TargetVersion:        targetVersion,
			QuestionnaireCode:    strings.TrimSpace(scaleItem.QuestionnaireCode),
			QuestionnaireVersion: version,
			QuestionnaireTitle:   strings.TrimSpace(detail.Title),
			QuestionnaireDetail:  detail,
			RequiresAssessment:   true,
		}, nil
	case "questionnaire":
		detail, err := client.GetQuestionnaireDetail(ctx, targetCode)
		if err != nil {
			return nil, fmt.Errorf("get questionnaire %s: %w", targetCode, err)
		}
		if detail == nil {
			return nil, fmt.Errorf("questionnaire %s not found", targetCode)
		}
		version := targetVersion
		if version == "" {
			version = strings.TrimSpace(detail.Version)
		}
		return &dailySimulationResolvedTarget{
			TargetType:           targetType,
			TargetCode:           targetCode,
			TargetVersion:        targetVersion,
			QuestionnaireCode:    targetCode,
			QuestionnaireVersion: version,
			QuestionnaireTitle:   strings.TrimSpace(detail.Title),
			QuestionnaireDetail:  detail,
			RequiresAssessment:   strings.EqualFold(strings.TrimSpace(detail.Type), questionnaireTypeMedicalScale),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported daily simulation targetType %q", targetType)
	}
}

func newDailySimulationIAMBundle(
	ctx context.Context,
	cfg IAMConfig,
	orgID int64,
) (*dailySimulationIAMBundle, error) {
	if strings.TrimSpace(cfg.GRPC.Address) == "" {
		return nil, fmt.Errorf("daily_simulation requires iam.grpc.address")
	}

	timeout := 15 * time.Second
	if strings.TrimSpace(cfg.GRPC.Timeout) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(cfg.GRPC.Timeout))
		if err != nil {
			return nil, fmt.Errorf("invalid iam.grpc.timeout %q: %w", cfg.GRPC.Timeout, err)
		}
		timeout = parsed
	}

	clientCfg := &sdk.Config{
		Endpoint: cfg.GRPC.Address,
		Timeout:  timeout,
	}
	if cfg.GRPC.RetryMax > 0 {
		clientCfg.Retry = &sdk.RetryConfig{
			Enabled:     true,
			MaxAttempts: cfg.GRPC.RetryMax,
		}
	}
	if cfg.GRPC.TLS.Enabled {
		clientCfg.TLS = &sdk.TLSConfig{
			Enabled:            true,
			CACert:             strings.TrimSpace(cfg.GRPC.TLS.CAFile),
			ClientCert:         strings.TrimSpace(cfg.GRPC.TLS.CertFile),
			ClientKey:          strings.TrimSpace(cfg.GRPC.TLS.KeyFile),
			ServerName:         strings.TrimSpace(cfg.GRPC.TLS.ServerName),
			InsecureSkipVerify: cfg.GRPC.TLS.InsecureSkipVerify,
		}
	}

	client, err := sdk.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("create daily simulation iam grpc client: %w", err)
	}
	return &dailySimulationIAMBundle{
		client:   client,
		identity: client.Identity(),
		auth:     client.Auth(),
	}, nil
}

func ensureDailySimulationGuardianAccount(
	ctx context.Context,
	deps *dependencies,
	iamBundle *dailySimulationIAMBundle,
	cfg DailySimulationConfig,
	profile dailySimulationProfile,
) (string, string, bool, error) {
	password := normalizeDailySimulationPassword(cfg.UserPassword)
	userID, created, err := ensureDailySimulationIAMUser(ctx, iamBundle, profile)
	if err != nil {
		return "", "", false, err
	}

	loginURL, err := resolveDailySimulationIAMLoginURL(deps.Config.IAM)
	if err != nil {
		return "", "", false, err
	}
	tenantID := resolveDailySimulationTenantID(deps.Config.IAM, deps.Config.Global.OrgID)
	deviceID := fmt.Sprintf("%s-%s-%03d", dailySimulationDeviceIDPrefix, profile.RunDate.Format("20060102"), profile.Index+1)

	token, err := tryDailySimulationGuardianLogin(ctx, loginURL, tenantID, deviceID, profile.GuardianEmail, profile.GuardianPhone, password, deps.Logger)
	if err == nil {
		return userID, token, created, nil
	}

	if _, regErr := iamBundle.auth.RegisterOperationAccount(ctx, &authnv1.RegisterOperationAccountRequest{
		ExistingUserId: userID,
		Name:           profile.GuardianName,
		Phone:          profile.GuardianPhone,
		Email:          profile.GuardianEmail,
		ScopedTenantId: tenantID,
		OperaLoginId:   profile.GuardianEmail,
		Password:       password,
	}); regErr != nil && !sdkerrors.IsAlreadyExists(regErr) {
		return "", "", false, fmt.Errorf("register guardian account for user %s: %w", userID, regErr)
	}

	token, err = tryDailySimulationGuardianLogin(ctx, loginURL, tenantID, deviceID, profile.GuardianEmail, profile.GuardianPhone, password, deps.Logger)
	if err != nil {
		return "", "", false, fmt.Errorf("login guardian %s after ensuring account: %w", profile.GuardianEmail, err)
	}
	return userID, token, created, nil
}

func ensureDailySimulationIAMUser(
	ctx context.Context,
	iamBundle *dailySimulationIAMBundle,
	profile dailySimulationProfile,
) (string, bool, error) {
	userID, err := findDailySimulationIAMUser(ctx, iamBundle, profile.GuardianPhone, profile.GuardianEmail)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(userID) != "" {
		return userID, false, nil
	}

	resp, err := iamBundle.identity.CreateUser(ctx, &identityv1.CreateUserRequest{
		Nickname: profile.GuardianName,
		Phone:    profile.GuardianPhone,
		Email:    profile.GuardianEmail,
	})
	if err != nil {
		if sdkerrors.IsAlreadyExists(err) {
			userID, lookupErr := findDailySimulationIAMUser(ctx, iamBundle, profile.GuardianPhone, profile.GuardianEmail)
			if lookupErr != nil {
				return "", false, lookupErr
			}
			if strings.TrimSpace(userID) != "" {
				return userID, false, nil
			}
		}
		return "", false, fmt.Errorf("create guardian iam user %s: %w", profile.GuardianEmail, err)
	}
	if resp == nil || resp.GetUser() == nil || strings.TrimSpace(resp.GetUser().GetId()) == "" {
		return "", false, fmt.Errorf("create guardian iam user returned empty id")
	}
	return strings.TrimSpace(resp.GetUser().GetId()), true, nil
}

func findDailySimulationIAMUser(
	ctx context.Context,
	iamBundle *dailySimulationIAMBundle,
	phone, email string,
) (string, error) {
	resp, err := iamBundle.identity.SearchUsers(ctx, &identityv1.SearchUsersRequest{
		Phones: []string{normalizePhone(phone)},
		Emails: []string{normalizeEmail(email)},
	})
	if err != nil {
		return "", fmt.Errorf("search iam users by phone/email: %w", err)
	}
	for _, item := range resp.GetUsers() {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.GetId()) != "" {
			return strings.TrimSpace(item.GetId()), nil
		}
	}
	return "", nil
}

func tryDailySimulationGuardianLogin(
	ctx context.Context,
	loginURL, tenantID, deviceID, email, phone, password string,
	logger log.Logger,
) (string, error) {
	credentials := []string{normalizeEmail(email), normalizePhone(phone)}
	var lastErr error
	for _, username := range credentials {
		if strings.TrimSpace(username) == "" {
			continue
		}
		token, err := fetchTokenFromIAMWithPassword(ctx, loginURL, username, password, tenantID, deviceID, logger)
		if err == nil {
			return token, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no available guardian login username")
	}
	return "", lastErr
}

func ensureDailySimulationChild(
	ctx context.Context,
	iamClient *APIClient,
	cfg DailySimulationConfig,
	profile dailySimulationProfile,
) (*IAMChildResponse, bool, error) {
	child, err := findDailySimulationChild(ctx, iamClient, profile.ChildName, profile.ChildDOB)
	if err != nil {
		return nil, false, err
	}
	if child != nil {
		return child, false, nil
	}

	relation := strings.TrimSpace(cfg.GuardianRelation)
	if relation == "" {
		relation = "guardian"
	}
	registerResp, err := iamClient.RegisterIAMChild(ctx, IAMChildRegisterRequest{
		LegalName: profile.ChildName,
		Gender:    profile.ChildGender,
		DOB:       profile.ChildDOB,
		Relation:  relation,
	})
	if err != nil {
		return nil, false, fmt.Errorf("register iam child %s: %w", profile.ChildName, err)
	}
	if registerResp == nil || registerResp.Child == nil || strings.TrimSpace(registerResp.Child.ID) == "" {
		return nil, false, fmt.Errorf("register iam child returned empty child")
	}
	return registerResp.Child, true, nil
}

func findDailySimulationChild(
	ctx context.Context,
	iamClient *APIClient,
	name, dob string,
) (*IAMChildResponse, error) {
	offset := 0
	for {
		pageResp, err := iamClient.ListIAMMyChildren(ctx, dailySimulationChildPageLimit, offset)
		if err != nil {
			return nil, fmt.Errorf("list iam my children: %w", err)
		}
		for _, item := range pageResp.Items {
			if item == nil {
				continue
			}
			if strings.TrimSpace(item.LegalName) == strings.TrimSpace(name) && strings.TrimSpace(item.DOB) == strings.TrimSpace(dob) {
				return item, nil
			}
		}
		offset += len(pageResp.Items)
		if len(pageResp.Items) == 0 || offset >= pageResp.Total {
			break
		}
	}
	return nil, nil
}

func ensureDailySimulationTestee(
	ctx context.Context,
	collectionClient *APIClient,
	guardianUserID string,
	cfg DailySimulationConfig,
	profile dailySimulationProfile,
	child *IAMChildResponse,
) (*TesteeResponse, bool, error) {
	existsResp, err := collectionClient.TesteeExistsByIAMChildID(ctx, child.ID)
	if err != nil {
		return nil, false, fmt.Errorf("check collection testee exists for child %s: %w", child.ID, err)
	}
	if existsResp != nil && existsResp.Exists && strings.TrimSpace(existsResp.TesteeID) != "" {
		return &TesteeResponse{
			ID:   existsResp.TesteeID,
			Name: child.LegalName,
		}, false, nil
	}

	testeeResp, err := collectionClient.CreateCollectionTestee(ctx, CollectionCreateTesteeRequest{
		IAMUserID:  guardianUserID,
		IAMChildID: child.ID,
		Name:       child.LegalName,
		Gender:     dailySimulationCollectionGender(child.Gender),
		Birthday:   child.DOB,
		Tags:       append([]string(nil), cfg.TesteeTags...),
		Source:     normalizeDailySimulationSource(cfg.TesteeSource),
		IsKeyFocus: cfg.IsKeyFocus,
	})
	if err != nil {
		return nil, false, fmt.Errorf("create collection testee for child %s: %w", child.ID, err)
	}
	return testeeResp, true, nil
}

func hasAssessmentEntryCreatorRelation(
	ctx context.Context,
	apiClient *APIClient,
	testeeID, entryID string,
) (bool, error) {
	relations, err := apiClient.GetTesteeClinicians(ctx, testeeID)
	if err != nil {
		return false, fmt.Errorf("list testee clinicians for %s: %w", testeeID, err)
	}
	for _, item := range relations.Items {
		if item == nil || item.Relation == nil {
			continue
		}
		relation := item.Relation
		if !relation.IsActive {
			continue
		}
		if strings.ToLower(strings.TrimSpace(relation.RelationType)) != "creator" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(relation.SourceType), "assessment_entry") {
			continue
		}
		if strings.TrimSpace(nullableString(relation.SourceID)) == strings.TrimSpace(entryID) {
			return true, nil
		}
	}
	return false, nil
}

func findDailySimulationAnswerSheet(
	ctx context.Context,
	adminClient *APIClient,
	questionnaireCode, guardianUserID, testeeID string,
) (*AdminAnswerSheetListItem, error) {
	userID := parseID(guardianUserID)
	if userID == 0 {
		return nil, fmt.Errorf("invalid guardian user id %q", guardianUserID)
	}
	resp, err := adminClient.ListAdminAnswerSheets(ctx, questionnaireCode, userID, 1, dailySimulationAnswerSheetPage)
	if err != nil {
		return nil, fmt.Errorf("list admin answersheets for questionnaire %s filler %s: %w", questionnaireCode, guardianUserID, err)
	}
	for _, item := range resp.Items {
		if strings.TrimSpace(item.TesteeID) == strings.TrimSpace(testeeID) {
			cloned := item
			return &cloned, nil
		}
	}
	return nil, nil
}

func findDailySimulationAssessment(
	ctx context.Context,
	apiClient *APIClient,
	testeeID, questionnaireCode, questionnaireVersion string,
) (string, error) {
	page := 1
	for {
		resp, err := apiClient.ListAssessmentsByTestee(ctx, testeeID, page, assessmentListPageSize)
		if err != nil {
			return "", fmt.Errorf("list assessments by testee %s: %w", testeeID, err)
		}
		for _, item := range resp.Items {
			if item == nil {
				continue
			}
			if strings.TrimSpace(item.QuestionnaireCode) == strings.TrimSpace(questionnaireCode) {
				return strings.TrimSpace(item.ID), nil
			}
		}
		if len(resp.Items) == 0 || page >= resp.TotalPages {
			break
		}
		page++
	}
	return "", nil
}

func waitForDailySimulationAssessment(
	ctx context.Context,
	collectionClient *APIClient,
	answerSheetID string,
) (string, error) {
	deadline := time.Now().Add(seedAssessmentPollTimeout)
	for {
		detail, err := collectionClient.GetAssessmentByAnswerSheetID(ctx, answerSheetID)
		if err != nil {
			return "", fmt.Errorf("get assessment by answersheet %s: %w", answerSheetID, err)
		}
		if detail != nil && strings.TrimSpace(detail.ID) != "" {
			return strings.TrimSpace(detail.ID), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("assessment not found by answersheet %s before timeout", answerSheetID)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(seedAssessmentPollInterval):
		}
	}
}

func buildDailySimulationProfile(cfg DailySimulationConfig, runDate time.Time, idx int) dailySimulationProfile {
	return buildSeedProfile(cfg, runDate, idx)
}

func selectDailySimulationPlanID(cfg DailySimulationConfig, runDate time.Time, index int) string {
	planIDs := collectDailySimulationPlanIDs(cfg.PlanIDs)
	if len(planIDs) == 0 {
		return ""
	}
	if len(planIDs) == 1 {
		return planIDs[0]
	}
	rng := newDailySimulationRand(fmt.Sprintf("plan:%s:%d", runDate.Format("20060102"), index))
	return planIDs[rng.Intn(len(planIDs))]
}

func newDailySimulationRand(seed string) *rand.Rand {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(seed))
	return rand.New(rand.NewSource(int64(hash.Sum64())))
}

func resolveDailySimulationRunDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		now := time.Now().In(time.Local)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local), nil
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.Local), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid dailySimulation.runDate %q", raw)
}

func resolveDailySimulationIAMLoginURL(cfg IAMConfig) (string, error) {
	if strings.TrimSpace(cfg.LoginURL) != "" {
		return strings.TrimSpace(cfg.LoginURL), nil
	}
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		return "", fmt.Errorf("iam.loginUrl or iam.baseUrl is required for daily_simulation")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse iam.baseUrl %q: %w", base, err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v1/authn/login"
	return parsed.String(), nil
}

func resolveDailySimulationTenantID(cfg IAMConfig, orgID int64) string {
	if strings.TrimSpace(cfg.TenantID) != "" {
		return strings.TrimSpace(cfg.TenantID)
	}
	if orgID > 0 {
		return strconv.FormatInt(orgID, 10)
	}
	return ""
}

func normalizeDailySimulationWorkers(value, count int) int {
	if value <= 0 {
		value = dailySimulationDefaultWorkers
	}
	if count > 0 && value > count {
		return count
	}
	return value
}

func normalizeDailySimulationPassword(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return dailySimulationDefaultPassword
	}
	return value
}

func normalizeDailySimulationSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return dailySimulationDefaultSource
	}
	return value
}

func parseDailySimulationDOB(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func dailySimulationCollectionGender(value *uint8) int32 {
	if value == nil {
		return 3
	}
	switch *value {
	case 1:
		return 1
	case 2:
		return 2
	default:
		return 3
	}
}

func dailySimulationAPIGender(value *uint8) string {
	if value == nil {
		return ""
	}
	switch *value {
	case 1:
		return "male"
	case 2:
		return "female"
	default:
		return "unknown"
	}
}
