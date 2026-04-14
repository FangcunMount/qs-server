package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	authnv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/authn/v1"
	identityv1 "github.com/FangcunMount/iam-contracts/api/grpc/iam/identity/v1"
	sdk "github.com/FangcunMount/iam-contracts/pkg/sdk"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/auth"
	sdkerrors "github.com/FangcunMount/iam-contracts/pkg/sdk/errors"
	"github.com/FangcunMount/iam-contracts/pkg/sdk/identity"
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
	EntryResolved     bool
	EntryIntaked      bool
	AnswerSheetID     string
	AssessmentID      string
	SkippedSubmission bool
}

type dailySimulationCounters struct {
	userCreated       int64
	childCreated      int64
	testeeCreated     int64
	resolved          int64
	intaked           int64
	submitted         int64
	skippedSubmission int64
	assessmentCreated int64
	failed            int64
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

func seedDailySimulation(ctx context.Context, deps *dependencies) error {
	cfg := deps.Config.DailySimulation
	if isEmptyDailySimulationConfig(cfg) {
		return fmt.Errorf("dailySimulation config is required for daily_simulation step")
	}

	count := normalizeDailySimulationCount(cfg.CountPerRun)
	workers := normalizeDailySimulationWorkers(cfg.Workers, count)
	runDate, err := resolveDailySimulationRunDate(cfg.RunDate)
	if err != nil {
		return err
	}

	iamBundle, err := newDailySimulationIAMBundle(ctx, deps.Config.IAM, deps.Config.Global.OrgID)
	if err != nil {
		return err
	}
	defer func() {
		if iamBundle != nil && iamBundle.client != nil {
			_ = iamBundle.client.Close()
		}
	}()

	entry, target, clinicianItem, err := ensureDailySimulationEntryAndTarget(ctx, deps, cfg)
	if err != nil {
		return err
	}

	progress := newSeedProgressBar("daily_simulation users", count)
	defer progress.Close()

	jobs := make(chan int)
	var wg sync.WaitGroup
	var counters dailySimulationCounters
	var failureMu sync.Mutex
	failures := make([]string, 0, 8)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				profile := buildDailySimulationProfile(cfg, runDate, idx)
				outcome, simErr := simulateDailyUser(
					ctx,
					deps,
					iamBundle,
					cfg,
					profile,
					clinicianItem,
					entry,
					target,
				)
				if simErr != nil {
					deps.Logger.Warnw("Daily simulation user failed",
						"index", profile.Index,
						"guardian_phone", profile.GuardianPhone,
						"guardian_email", profile.GuardianEmail,
						"child_name", profile.ChildName,
						"error", simErr.Error(),
					)
					counters.addFailure()
					failureMu.Lock()
					if len(failures) < 8 {
						failures = append(failures, fmt.Sprintf("idx=%d guardian=%s child=%s err=%v", profile.Index, profile.GuardianEmail, profile.ChildName, simErr))
					}
					failureMu.Unlock()
				} else {
					counters.add(outcome)
				}
				progress.Increment()
			}
		}()
	}

	for idx := 0; idx < count; idx++ {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- idx:
		}
	}
	close(jobs)
	wg.Wait()
	progress.Complete()

	deps.Logger.Infow("Daily simulation completed",
		"run_date", runDate.Format("2006-01-02"),
		"count", count,
		"workers", workers,
		"clinician_id", clinicianItem.ID,
		"clinician_name", clinicianItem.Name,
		"entry_id", entry.ID,
		"target_type", target.TargetType,
		"target_code", target.TargetCode,
		"target_version", target.TargetVersion,
		"users_created", atomic.LoadInt64(&counters.userCreated),
		"children_created", atomic.LoadInt64(&counters.childCreated),
		"testees_created", atomic.LoadInt64(&counters.testeeCreated),
		"entries_resolved", atomic.LoadInt64(&counters.resolved),
		"entries_intaked", atomic.LoadInt64(&counters.intaked),
		"answersheets_submitted", atomic.LoadInt64(&counters.submitted),
		"submissions_skipped", atomic.LoadInt64(&counters.skippedSubmission),
		"assessments_found", atomic.LoadInt64(&counters.assessmentCreated),
		"failed", atomic.LoadInt64(&counters.failed),
	)
	if len(failures) > 0 {
		deps.Logger.Warnw("Daily simulation failure samples", "count", len(failures), "samples", failures)
	}

	if atomic.LoadInt64(&counters.failed) > 0 {
		return fmt.Errorf("daily_simulation completed with %d failures", atomic.LoadInt64(&counters.failed))
	}
	return nil
}

func simulateDailyUser(
	ctx context.Context,
	deps *dependencies,
	iamBundle *dailySimulationIAMBundle,
	cfg DailySimulationConfig,
	profile dailySimulationProfile,
	clinicianItem *ClinicianResponse,
	entry *AssessmentEntryResponse,
	target *dailySimulationResolvedTarget,
) (dailySimulationOutcome, error) {
	outcome := dailySimulationOutcome{}

	guardianUserID, guardianToken, userCreated, err := ensureDailySimulationGuardianAccount(ctx, deps, iamBundle, cfg, profile)
	if err != nil {
		return outcome, err
	}
	outcome.UserCreated = userCreated

	userClient := NewAPIClient(deps.APIClient.baseURL, guardianToken, deps.Logger)
	userClient.SetRetryConfig(deps.Config.API.Retry)
	collectionClient := NewAPIClient(deps.CollectionClient.baseURL, guardianToken, deps.Logger)
	collectionClient.SetRetryConfig(deps.Config.API.Retry)

	child, childCreated, err := ensureDailySimulationChild(ctx, userClient, cfg, profile)
	if err != nil {
		return outcome, err
	}
	outcome.ChildCreated = childCreated

	testee, testeeCreated, err := ensureDailySimulationTestee(ctx, collectionClient, guardianUserID, cfg, profile, child)
	if err != nil {
		return outcome, err
	}
	outcome.TesteeCreated = testeeCreated

	hasCreator, err := hasAssessmentEntryCreatorRelation(ctx, deps.APIClient, testee.ID, entry.ID)
	if err != nil {
		return outcome, err
	}
	if !hasCreator {
		if _, err := deps.APIClient.ResolveAssessmentEntry(ctx, entry.Token); err != nil {
			return outcome, err
		}
		outcome.EntryResolved = true

		childID := parseID(child.ID)
		if childID == 0 {
			return outcome, fmt.Errorf("invalid child id %q", child.ID)
		}
		birthday, err := parseDailySimulationDOB(child.DOB)
		if err != nil {
			return outcome, fmt.Errorf("parse child dob %q: %w", child.DOB, err)
		}
		intakeResp, err := deps.APIClient.IntakeAssessmentEntry(ctx, entry.Token, IntakeAssessmentEntryRequest{
			ProfileID: &childID,
			Name:      child.LegalName,
			Gender:    dailySimulationAPIGender(child.Gender),
			Birthday:  birthday,
		})
		if err != nil {
			return outcome, err
		}
		if intakeResp.Testee != nil && strings.TrimSpace(intakeResp.Testee.ID) != "" {
			testee.ID = intakeResp.Testee.ID
		}
		outcome.EntryIntaked = true
	}

	existingAnswerSheet, err := findDailySimulationAnswerSheet(ctx, deps.APIClient, target.QuestionnaireCode, guardianUserID, testee.ID)
	if err != nil {
		return outcome, err
	}
	existingAssessmentID, err := findDailySimulationAssessment(ctx, deps.APIClient, testee.ID, target.QuestionnaireCode, target.QuestionnaireVersion)
	if err != nil {
		return outcome, err
	}
	if strings.TrimSpace(existingAssessmentID) != "" {
		outcome.AssessmentID = existingAssessmentID
		outcome.SkippedSubmission = true
		return outcome, logDailySimulationOutcome(deps, profile, clinicianItem, entry, target, testee.ID, guardianUserID, outcome)
	}
	if existingAnswerSheet != nil && !target.RequiresAssessment {
		outcome.AnswerSheetID = existingAnswerSheet.ID
		outcome.SkippedSubmission = true
		return outcome, logDailySimulationOutcome(deps, profile, clinicianItem, entry, target, testee.ID, guardianUserID, outcome)
	}
	if existingAnswerSheet != nil && target.RequiresAssessment {
		assessmentID, waitErr := waitForDailySimulationAssessment(ctx, collectionClient, existingAnswerSheet.ID)
		if waitErr == nil {
			outcome.AnswerSheetID = existingAnswerSheet.ID
			outcome.AssessmentID = assessmentID
			outcome.SkippedSubmission = true
			return outcome, logDailySimulationOutcome(deps, profile, clinicianItem, entry, target, testee.ID, guardianUserID, outcome)
		}
	}

	testeeID := parseID(testee.ID)
	if testeeID == 0 {
		return outcome, fmt.Errorf("invalid testee id %q", testee.ID)
	}
	questionnaireDetail, err := collectionClient.GetQuestionnaireDetail(ctx, target.QuestionnaireCode)
	if err != nil {
		return outcome, fmt.Errorf("get questionnaire detail %s: %w", target.QuestionnaireCode, err)
	}

	rng := newDailySimulationRand("answers:" + profile.RunDate.Format("20060102") + ":" + strconv.Itoa(profile.Index) + ":" + target.QuestionnaireCode)
	answers := buildAnswers(questionnaireDetail, rng)
	submitResp, err := collectionClient.SubmitAnswerSheet(ctx, SubmitAnswerSheetRequest{
		QuestionnaireCode:    target.QuestionnaireCode,
		QuestionnaireVersion: target.QuestionnaireVersion,
		Title:                target.QuestionnaireTitle,
		TesteeID:             testeeID,
		Answers:              answers,
	})
	if err != nil {
		return outcome, err
	}
	outcome.AnswerSheetID = submitResp.ID

	if target.RequiresAssessment {
		assessmentID, err := waitForDailySimulationAssessment(ctx, collectionClient, submitResp.ID)
		if err != nil {
			return outcome, err
		}
		outcome.AssessmentID = assessmentID
	}

	return outcome, logDailySimulationOutcome(deps, profile, clinicianItem, entry, target, testee.ID, guardianUserID, outcome)
}

func logDailySimulationOutcome(
	deps *dependencies,
	profile dailySimulationProfile,
	clinicianItem *ClinicianResponse,
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
		"clinician_id", clinicianItem.ID,
		"clinician_name", clinicianItem.Name,
		"entry_id", entry.ID,
		"target_type", target.TargetType,
		"target_code", target.TargetCode,
		"user_created", outcome.UserCreated,
		"child_created", outcome.ChildCreated,
		"testee_created", outcome.TesteeCreated,
		"entry_resolved", outcome.EntryResolved,
		"entry_intaked", outcome.EntryIntaked,
		"answersheet_id", outcome.AnswerSheetID,
		"assessment_id", outcome.AssessmentID,
		"submission_skipped", outcome.SkippedSubmission,
	)
	return nil
}

func ensureDailySimulationEntryAndTarget(
	ctx context.Context,
	deps *dependencies,
	cfg DailySimulationConfig,
) (*AssessmentEntryResponse, *dailySimulationResolvedTarget, *ClinicianResponse, error) {
	var clinicianItem *ClinicianResponse

	if !cfg.EntryID.IsZero() {
		entry, err := deps.APIClient.GetAssessmentEntry(ctx, cfg.EntryID.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("get daily simulation entry %s: %w", cfg.EntryID.String(), err)
		}
		if entry == nil {
			return nil, nil, nil, fmt.Errorf("daily simulation entry %s not found", cfg.EntryID.String())
		}
		if !entry.IsActive {
			entry, err = deps.APIClient.ReactivateAssessmentEntry(ctx, entry.ID)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("reactivate daily simulation entry %s: %w", entry.ID, err)
			}
		}

		clinicians, err := listAllClinicians(ctx, deps.APIClient, deps.Config.Global.OrgID)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, item := range clinicians {
			if item != nil && strings.TrimSpace(item.ID) == strings.TrimSpace(entry.ClinicianID) {
				clinicianItem = item
				break
			}
		}
		if clinicianItem == nil {
			return nil, nil, nil, fmt.Errorf("clinician %s for daily simulation entry not found", entry.ClinicianID)
		}
		target, err := resolveDailySimulationTarget(ctx, deps.CollectionClient, entry.TargetType, entry.TargetCode, entry.TargetVersion)
		if err != nil {
			return nil, nil, nil, err
		}
		return entry, target, clinicianItem, nil
	}

	clinicians, err := resolveSeedClinicianScope(ctx, deps, seedClinicianScopeSpec{
		refs: []string{strings.TrimSpace(cfg.ClinicianRef)},
		ids:  []FlexibleID{cfg.ClinicianID},
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if len(clinicians) == 0 {
		return nil, nil, nil, fmt.Errorf("dailySimulation clinician is required")
	}
	clinicianItem = clinicians[0]

	targetType := strings.ToLower(strings.TrimSpace(cfg.TargetType))
	targetCode := strings.TrimSpace(cfg.TargetCode)
	targetVersion := strings.TrimSpace(cfg.TargetVersion)
	if targetType == "" || targetCode == "" {
		return nil, nil, nil, fmt.Errorf("dailySimulation targetType and targetCode are required when entryId is not set")
	}

	entries, err := listAllClinicianAssessmentEntries(ctx, deps.APIClient, clinicianItem.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list daily simulation clinician assessment entries: %w", err)
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
				return nil, nil, nil, fmt.Errorf("reactivate daily simulation entry %s: %w", item.ID, err)
			}
		}
		target, err := resolveDailySimulationTarget(ctx, deps.CollectionClient, item.TargetType, item.TargetCode, item.TargetVersion)
		if err != nil {
			return nil, nil, nil, err
		}
		return item, target, clinicianItem, nil
	}

	entry, err := deps.APIClient.CreateClinicianAssessmentEntry(ctx, clinicianItem.ID, CreateAssessmentEntryRequest{
		TargetType:    targetType,
		TargetCode:    targetCode,
		TargetVersion: targetVersion,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create daily simulation entry: %w", err)
	}
	target, err := resolveDailySimulationTarget(ctx, deps.CollectionClient, entry.TargetType, entry.TargetCode, entry.TargetVersion)
	if err != nil {
		return nil, nil, nil, err
	}
	return entry, target, clinicianItem, nil
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
	guardianName := buildDailySimulationChineseName("guardian", runDate, idx)
	childName := buildDailySimulationChineseName("child", runDate, idx)
	phonePrefix := normalizeDailySimulationPhonePrefix(cfg.UserPhonePrefix)
	emailDomain := normalizeDailySimulationEmailDomain(cfg.UserEmailDomain)
	phoneSuffix := fmt.Sprintf("%02d%02d%04d", int(runDate.Month()), runDate.Day(), idx+1)
	phone := phonePrefix + phoneSuffix
	emailLocal, err := buildGeneratedClinicianEmailLocal(guardianName)
	if err != nil || strings.TrimSpace(emailLocal) == "" {
		emailLocal = fmt.Sprintf("dailyguardian%04d", idx+1)
	}
	email := normalizeEmail(fmt.Sprintf("%s_%s_%04d@%s", emailLocal, runDate.Format("20060102"), idx+1, emailDomain))

	childDOB := buildDailySimulationChildDOB(runDate, idx)
	childGender := uint8(1)
	if idx%2 == 1 {
		childGender = 2
	}

	return dailySimulationProfile{
		Index:         idx + 1,
		RunDate:       runDate,
		GuardianName:  guardianName,
		GuardianPhone: phone,
		GuardianEmail: email,
		ChildName:     childName,
		ChildDOB:      childDOB,
		ChildGender:   childGender,
	}
}

func buildDailySimulationChineseName(kind string, runDate time.Time, idx int) string {
	surnames := []string{"王", "李", "张", "刘", "陈", "杨", "赵", "黄", "周", "吴", "徐", "孙", "朱", "高", "林", "何", "郭", "马", "罗", "梁"}
	guardianGiven := []string{"雅宁", "雨桐", "欣怡", "梓涵", "若彤", "书妍", "佳宁", "语晨", "梦瑶", "诗涵", "家豪", "宇辰", "浩然", "俊杰", "泽宇", "嘉铭", "博文", "思源", "一诺", "昊天"}
	childGiven := []string{"沐阳", "可欣", "子轩", "梓萌", "亦辰", "欣然", "雨泽", "乐彤", "嘉悦", "宸熙", "可宁", "奕宸", "芷晴", "晨曦", "若熙", "晨语", "依诺", "铭泽", "沐宸", "诗雨"}
	given := guardianGiven
	if kind == "child" {
		given = childGiven
	}
	rng := newDailySimulationRand(fmt.Sprintf("%s:%s:%d", kind, runDate.Format("20060102"), idx))
	return surnames[rng.Intn(len(surnames))] + given[rng.Intn(len(given))]
}

func buildDailySimulationChildDOB(runDate time.Time, idx int) string {
	years := 2 + (idx % 10)
	months := (idx * 3) % 12
	days := (idx * 5) % 27
	dob := runDate.AddDate(-years, -months, -(days + 1))
	return dob.Format("2006-01-02")
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

func normalizeDailySimulationCount(value int) int {
	if value <= 0 {
		return dailySimulationDefaultCount
	}
	return value
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

func normalizeDailySimulationPhonePrefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return dailySimulationDefaultPhonePref
	}
	return value
}

func normalizeDailySimulationEmailDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return dailySimulationDefaultEmailHost
	}
	return strings.TrimPrefix(value, "@")
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
