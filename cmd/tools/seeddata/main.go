// Package main implements the QS seed data tool.
//
// This tool populates the QS database with test data including:
// - Testees (受试者)
// - Questionnaires (问卷)
// - Medical Scales (医学量表)
// - Answer Sheets (答卷)
// - Assessments (测评)
//
// The tool is modularized into separate files:
// - seed_assessment.go: Assessment data seeding
// - seed_plan_*.go: Plan task creation, selection, and processing
//
// Usage:
//
//	go run ./cmd/tools/seeddata \
//	  --config configs/seeddata.yaml \
//	  --steps assessment
//	go run ./cmd/tools/seeddata \
//	  --config configs/seeddata.yaml \
//	  --steps plan_create_tasks \
//	  --plan-id 614186929759466030 \
//	  --plan-workers 4
//	go run ./cmd/tools/seeddata \
//	  --config configs/seeddata.yaml \
//	  --steps plan_process_tasks \
//	  --plan-id 614186929759466030
//
// See README.md for detailed documentation.
package main

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"strings"
	"syscall"

	"github.com/FangcunMount/component-base/pkg/log"
)

// ==================== 核心类型定义 ====================

// seedStep represents a specific seeding step.
type seedStep string

// All available seed steps.
const (
	stepStaff                seedStep = "staff"
	stepClinician            seedStep = "clinician"
	stepAssignTestees        seedStep = "assign_testees"
	stepTesteeFixupCreated   seedStep = "testee_fixup_created_at"
	stepActorFixupTimes      seedStep = "actor_fixup_timestamps"
	stepAssessmentEntries    seedStep = "assessment_entries"
	stepAssessmentEntryFlow  seedStep = "assessment_entry_flow"
	stepAssessmentByEntry    seedStep = "assessment_by_entry"
	stepAssessmentEntryFixup seedStep = "assessment_entry_fixup_timestamps"
	stepAssessmentFixup      seedStep = "assessment_fixup_timestamps"
	stepDailySimulation      seedStep = "daily_simulation"
	stepAssessment           seedStep = "assessment"         // 提交答卷并生成测评
	stepPlan                 seedStep = "plan"               // 兼容旧入口：先造 task，再处理 task
	stepPlanCreateTasks      seedStep = "plan_create_tasks"  // 批量创建/补齐计划任务
	stepPlanProcessTasks     seedStep = "plan_process_tasks" // 调度并处理计划任务
	stepPlanFixupTimes       seedStep = "plan_fixup_timestamps"
	stepStatisticsBackfill   seedStep = "statistics_backfill"
)

// defaultSteps defines the default execution order of all seed steps.
var defaultSteps = []seedStep{
	stepAssessment,
}

// dependencies holds all external dependencies required by seed functions.
type dependencies struct {
	Logger           log.Logger  // 日志记录器
	Config           *SeedConfig // 种子数据配置
	APIClient        *APIClient  // 管理端 HTTP API 客户端
	CollectionClient *APIClient  // 采集端 HTTP API 客户端
}

// ==================== 主程序入口 ====================

func main() {
	// 解析命令行参数
	var (
		apiBaseURL                     = flag.String("api-base-url", "", "API base URL (e.g., http://localhost:18082)")
		collectionBaseURL              = flag.String("collection-base-url", "", "Collection server API base URL (defaults to api-base-url)")
		apiToken                       = flag.String("api-token", "", "API authentication token")
		configFile                     = flag.String("config", "", "Base seed data config file (testees, legacy data)")
		stepsRaw                       = flag.String("steps", "", "Comma-separated steps to run (default: all)")
		planID                         = flag.String("plan-id", defaultPlanID, "Plan ID for plan backfill step")
		planWorkers                    = flag.Int("plan-workers", 1, "Concurrent workers for plan backfill enrollment and task execution")
		planSubmitWorkers              = flag.Int("plan-submit-workers", 0, "Concurrent workers for plan task answersheet submission (defaults to --plan-workers)")
		planWaitWorkers                = flag.Int("plan-wait-workers", 0, "Concurrent workers for waiting plan task completion (defaults to --plan-workers)")
		planMaxInFlightTasks           = flag.Int("plan-max-inflight-tasks", 0, "Maximum in-flight submitted plan tasks waiting for worker/apiserver completion (defaults based on submit/wait workers)")
		planSubmitQueueSize            = flag.Int("plan-submit-queue-size", 0, "Buffered queue size before plan task answersheet submission dispatch (defaults based on submit workers and inflight limit)")
		planSubmitQPS                  = flag.Float64("plan-submit-qps", 0, "Global dequeue rate for plan task answersheet submission queue (0 = derive from submit workers)")
		planSubmitBurst                = flag.Int("plan-submit-burst", 0, "Burst size for plan task answersheet submission queue dispatch (0 = derive from submit workers)")
		planExpireRate                 = flag.Float64("plan-expire-rate", 0.2, "Ratio of opened plan tasks to expire instead of submit (0.0-1.0)")
		planTesteeIDsRaw               = flag.String("plan-testee-ids", "", "Comma-separated testee IDs to include in plan backfill (overrides random sampling)")
		localMySQLDSN                  = flag.String("local-mysql-dsn", "", "Local seed_plan MySQL DSN override")
		localMongoURI                  = flag.String("local-mongo-uri", "", "Local seed_plan MongoDB URI override")
		localMongoDatabase             = flag.String("local-mongo-database", "", "Local seed_plan MongoDB database override")
		localRedisAddr                 = flag.String("local-redis-addr", "", "Local seed_plan Redis address override")
		localRedisUsername             = flag.String("local-redis-username", "", "Local seed_plan Redis username override")
		localRedisPassword             = flag.String("local-redis-password", "", "Local seed_plan Redis password override")
		localRedisDB                   = flag.Int("local-redis-db", -1, "Local seed_plan Redis DB override")
		localPlanEntryBaseURL          = flag.String("local-plan-entry-base-url", "", "Local seed_plan plan entry base URL override")
		assessmentMin                  = flag.Int("assessment-min", 5, "Minimum assessments per testee")
		assessmentMax                  = flag.Int("assessment-max", 10, "Maximum assessments per testee")
		assessmentWorkers              = flag.Int("assessment-workers", 10, "Concurrent workers for assessment seeding")
		assessmentSubmitWorkers        = flag.Int("assessment-submit-workers", 10, "Concurrent workers for assessment submission")
		assignmentWorkers              = flag.Int("assignment-workers", 8, "Concurrent workers for testee-to-clinician assignment seeding")
		testeePageSize                 = flag.Int("testee-page-size", 1, "Page size when listing testees for assessment seeding")
		testeeOffset                   = flag.Int("testee-offset", 0, "Starting offset when listing testees for assessment seeding")
		testeeLimit                    = flag.Int("testee-limit", 0, "Maximum number of testees to load/process for assessment and plan task creation (0 = no limit)")
		assessmentCategories           = flag.String("assessment-scale-categories", "", "Comma-separated scale categories to include (defaults to all)")
		verbose                        = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()
	steps := parseSteps(*stepsRaw)

	// 初始化全局日志与 seeddata 自身日志。
	//
	// seeddata 自身保持 info/debug 级别，便于观察脚本进度；而 local plan 模式下
	// 需要的 component-base 全局日志会在解析出模式后再单独降到 warn，避免把
	// 本地应用服务的高频 INFO 日志刷到终端。
	seedLogger := newSeeddataLogger(*verbose)
	configureSeeddataGlobalLog(*verbose, false)
	logger := seedLogger

	// 加载配置文件
	var config *SeedConfig
	if strings.TrimSpace(*configFile) != "" {
		logger.Infow("Loading seed config", "file", *configFile)
		cfg, err := LoadSeedConfig(*configFile)
		if err != nil {
			logger.Fatalw("Failed to load config", "error", err)
		}
		config = cfg
	} else {
		config = &SeedConfig{}
	}

	// 从配置补全 API 参数（命令行优先）
	if strings.TrimSpace(*apiBaseURL) == "" {
		*apiBaseURL = strings.TrimSpace(config.API.BaseURL)
	}
	if strings.TrimSpace(*collectionBaseURL) == "" {
		*collectionBaseURL = strings.TrimSpace(config.API.CollectionBaseURL)
	}
	if strings.TrimSpace(*apiToken) == "" {
		*apiToken = strings.TrimSpace(config.API.Token)
	}

	// 如果 API token 为空，尝试从 IAM 获取
	if strings.TrimSpace(*apiToken) == "" && (config.IAM != IAMConfig{}) {
		logger.Infow("Fetching API token from IAM", "login_url", config.IAM.LoginURL, "username", config.IAM.Username)
		token, err := fetchTokenFromIAM(context.Background(), config.IAM, logger)
		if err != nil {
			logger.Fatalw("Failed to fetch token from IAM", "error", err)
		}
		*apiToken = token
	}

	// 从命令行覆盖 local runtime 配置，避免把敏感信息写入仓库配置文件。
	if strings.TrimSpace(*localMySQLDSN) != "" {
		config.Local.MySQLDSN = strings.TrimSpace(*localMySQLDSN)
	}
	if strings.TrimSpace(*localMongoURI) != "" {
		config.Local.MongoURI = strings.TrimSpace(*localMongoURI)
	}
	if strings.TrimSpace(*localMongoDatabase) != "" {
		config.Local.MongoDatabase = strings.TrimSpace(*localMongoDatabase)
	}
	if strings.TrimSpace(*localRedisAddr) != "" {
		config.Local.RedisAddr = strings.TrimSpace(*localRedisAddr)
	}
	if strings.TrimSpace(*localRedisUsername) != "" {
		config.Local.RedisUsername = strings.TrimSpace(*localRedisUsername)
	}
	if *localRedisPassword != "" {
		config.Local.RedisPassword = *localRedisPassword
	}
	if *localRedisDB >= 0 {
		config.Local.RedisDB = *localRedisDB
	}
	if strings.TrimSpace(*localPlanEntryBaseURL) != "" {
		config.Local.PlanEntryBaseURL = strings.TrimSpace(*localPlanEntryBaseURL)
	}

	// 初始化 API 客户端
	if strings.TrimSpace(*apiBaseURL) == "" {
		logger.Fatalw("API base URL is required, set via --api-base-url or API_BASE_URL env var")
	}
	if strings.TrimSpace(*apiToken) == "" {
		logger.Fatalw("API token is required, set via --api-token or seeddata config")
	}

	apiClient := NewAPIClient(*apiBaseURL, *apiToken, logger)
	apiClient.SetRetryConfig(config.API.Retry)
	logger.Infow("Initialized API client", "base_url", *apiBaseURL)

	collectionURL := strings.TrimSpace(*collectionBaseURL)
	if collectionURL == "" {
		collectionURL = *apiBaseURL
	}
	collectionClient := NewAPIClient(collectionURL, *apiToken, logger)
	collectionClient.SetRetryConfig(config.API.Retry)
	logger.Infow("Initialized collection client", "base_url", collectionURL)

	if (config.IAM != IAMConfig{}) {
		refresher := func(ctx context.Context) (string, error) {
			return fetchTokenFromIAM(ctx, config.IAM, logger)
		}
		tokenProvider := newSeedTokenProvider(*apiToken, refresher)
		apiClient.SetTokenProvider(tokenProvider)
		collectionClient.SetTokenProvider(tokenProvider)
		apiClient.SetTokenRefresher(refresher)
		collectionClient.SetTokenRefresher(refresher)
	}

	// 构建依赖
	deps := &dependencies{
		Logger:           logger,
		Config:           config,
		APIClient:        apiClient,
		CollectionClient: collectionClient,
	}

	logger.Infow("Starting seed process", "steps", stepListToStrings(steps))
	stepProgress := newSeedProgressBar("seed steps", len(steps))
	defer stepProgress.Close()

	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if containsSeedStep(steps, stepAssessment) && (*assessmentMin <= 0 || *assessmentMax <= 0 || *assessmentMax < *assessmentMin) {
		logger.Fatalw("Invalid assessment range", "min", *assessmentMin, "max", *assessmentMax)
	}
	configureSeeddataGlobalLog(*verbose, shouldQuietSeedPlanComponentLogs(steps))

	assessmentOpts := assessmentSeedOptions{
		MinPerTestee:      *assessmentMin,
		MaxPerTestee:      *assessmentMax,
		WorkerCount:       *assessmentWorkers,
		SubmitWorkerCount: *assessmentSubmitWorkers,
		TesteePageSize:    *testeePageSize,
		TesteeOffset:      *testeeOffset,
		TesteeLimit:       *testeeLimit,
		CategoryFilter:    *assessmentCategories,
		Verbose:           *verbose,
	}
	assignmentOpts := assignmentSeedOptions{
		WorkerCount: *assignmentWorkers,
	}
	planCreateOpts := planCreateOptions{
		PlanID:           *planID,
		PlanTesteeIDsRaw: *planTesteeIDsRaw,
		PlanWorkers:      *planWorkers,
		TesteePageSize:   *testeePageSize,
		TesteeOffset:     *testeeOffset,
		TesteeLimit:      *testeeLimit,
		Verbose:          *verbose,
	}
	planProcessOpts := planProcessOptions{
		PlanID:               *planID,
		ScopeTesteeIDs:       parsePlanTesteeIDs(*planTesteeIDsRaw),
		PlanWorkers:          *planWorkers,
		PlanSubmitWorkers:    *planSubmitWorkers,
		PlanWaitWorkers:      *planWaitWorkers,
		PlanMaxInFlightTasks: *planMaxInFlightTasks,
		PlanSubmitQueueSize:  *planSubmitQueueSize,
		PlanSubmitQPS:        *planSubmitQPS,
		PlanSubmitBurst:      *planSubmitBurst,
		PlanExpireRate:       *planExpireRate,
		Verbose:              *verbose,
		Continuous:           true,
	}
	planFixupOpts := planFixupOptions{
		PlanID:         *planID,
		ScopeTesteeIDs: parsePlanTesteeIDs(*planTesteeIDsRaw),
		Verbose:        *verbose,
	}

	for _, step := range steps {
		logger.Infow("Running seed step", "step", step)
		var err error
		switch step {
		case stepStaff:
			err = seedStaffs(runCtx, deps)
		case stepClinician:
			err = seedClinicians(runCtx, deps)
		case stepAssignTestees:
			err = seedAssignTestees(runCtx, deps, assignmentOpts)
		case stepTesteeFixupCreated:
			err = seedTesteeFixupCreatedAt(runCtx, deps)
		case stepActorFixupTimes:
			err = seedActorFixupTimestamps(runCtx, deps)
		case stepAssessmentEntries:
			err = seedAssessmentEntries(runCtx, deps)
		case stepAssessmentEntryFlow:
			err = seedAssessmentEntryFlow(runCtx, deps)
		case stepAssessmentByEntry:
			err = seedAssessmentByEntry(runCtx, deps)
		case stepAssessmentEntryFixup:
			err = legacySeedStepRemovedError(stepAssessmentEntryFixup)
		case stepAssessmentFixup:
			err = legacySeedStepRemovedError(stepAssessmentFixup)
		case stepDailySimulation:
			err = seedDailySimulation(runCtx, deps)
		case stepAssessment:
			err = seedAssessments(runCtx, deps, assessmentOpts)
		case stepPlan:
			err = seedPlanBackfill(runCtx, deps, planCreateOpts, planProcessOpts.withScope(planProcessOpts.ScopeTesteeIDs, false))
		case stepPlanCreateTasks:
			_, err = seedPlanCreateTasks(runCtx, deps, planCreateOpts)
		case stepPlanProcessTasks:
			_, err = seedPlanProcessTasks(runCtx, deps, planProcessOpts)
		case stepPlanFixupTimes:
			err = seedPlanFixupTimestamps(runCtx, deps, planFixupOpts)
		case stepStatisticsBackfill:
			err = seedStatisticsBackfill(runCtx, deps)
		default:
			logger.Warnw("Skipping unimplemented step", "step", step)
		}
		if err != nil {
			stepProgress.Close()
			logger.Fatalw(seedStepFailureMessage(step), "error", err)
		}
		stepProgress.Increment()
	}

	stepProgress.Complete()
	logger.Infow("Seed process completed successfully")
}

func newSeeddataLogger(verbose bool) log.Logger {
	opts := log.NewOptions()
	if verbose {
		opts.Level = "debug"
	} else {
		opts.Level = "info"
	}
	return log.New(opts)
}

func configureSeeddataGlobalLog(verbose bool, quiet bool) {
	opts := log.NewOptions()
	switch {
	case verbose:
		opts.Level = "debug"
	case quiet:
		opts.Level = "warn"
	default:
		opts.Level = "info"
	}
	log.Init(opts)
}

func shouldQuietSeedPlanComponentLogs(steps []seedStep) bool {
	return containsSeedStep(steps, stepPlan) ||
		containsSeedStep(steps, stepPlanCreateTasks) ||
		containsSeedStep(steps, stepPlanProcessTasks) ||
		containsSeedStep(steps, stepPlanFixupTimes)
}

// ==================== 通用辅助函数 ====================

// parseSteps 解析步骤字符串为步骤列表
func parseSteps(raw string) []seedStep {
	if strings.TrimSpace(raw) == "" {
		return defaultSteps
	}
	items := strings.Split(raw, ",")
	var steps []seedStep
	for _, item := range items {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" {
			continue
		}
		steps = append(steps, seedStep(item))
	}
	return steps
}

// stepListToStrings 将步骤列表转换为字符串列表
func stepListToStrings(steps []seedStep) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, string(s))
	}
	return out
}

func containsSeedStep(steps []seedStep, target seedStep) bool {
	for _, step := range steps {
		if step == target {
			return true
		}
	}
	return false
}

func seedStepFailureMessage(step seedStep) string {
	switch step {
	case stepStaff:
		return "Staff seeding failed"
	case stepClinician:
		return "Clinician seeding failed"
	case stepAssignTestees:
		return "Testee assignment seeding failed"
	case stepTesteeFixupCreated:
		return "Testee created_at fixup failed"
	case stepActorFixupTimes:
		return "Actor timestamp fixup failed"
	case stepAssessmentEntries:
		return "Assessment entry seeding failed"
	case stepAssessmentEntryFlow:
		return "Assessment entry flow seeding failed"
	case stepAssessmentByEntry:
		return "Assessment by entry seeding failed"
	case stepAssessmentEntryFixup:
		return "Assessment entry timestamp fixup failed"
	case stepAssessmentFixup:
		return "Assessment timestamp fixup failed"
	case stepDailySimulation:
		return "Daily simulation seeding failed"
	case stepAssessment:
		return "Assessment seeding failed"
	case stepPlan:
		return "Plan backfill failed"
	case stepPlanCreateTasks:
		return "Plan task creation failed"
	case stepPlanProcessTasks:
		return "Plan task processing failed"
	case stepPlanFixupTimes:
		return "Plan timestamp fixup failed"
	case stepStatisticsBackfill:
		return "Statistics backfill failed"
	default:
		return "Seed step failed"
	}
}

func legacySeedStepRemovedError(step seedStep) error {
	return fmt.Errorf("%s has been removed in the behavior_footprint + assessment_episode seed model; regenerate data via real business steps (assessment_entry_flow, assessment_by_entry, assessment, daily_simulation) and use statistics_backfill to rebuild analytics projections", step)
}
