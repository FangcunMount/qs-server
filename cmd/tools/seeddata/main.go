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
//
// Usage:
//
//	go run ./cmd/tools/seeddata \
//	  --config configs/seeddata.yaml \
//	  --steps assessment
//
// See README.md for detailed documentation.
package main

import (
	"context"
	"flag"
	"strings"

	"github.com/FangcunMount/component-base/pkg/log"
)

// ==================== 核心类型定义 ====================

// seedStep represents a specific seeding step.
type seedStep string

// All available seed steps.
const (
	stepAssessment seedStep = "assessment" // 提交答卷并生成测评
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

// seedContext holds the state and references created during seeding.
// This allows later steps to reference entities created by earlier steps.
type seedContext struct {
	// 受试者映射：name -> ID
	TesteeIDsByName map[string]string

	// 问卷映射：code -> version
	QuestionnaireVersionsByCode map[string]string

	// 量表映射：code -> ID
	ScaleIDsByCode map[string]string
}

// newSeedContext creates a new seed context with initialized maps.
func newSeedContext() *seedContext {
	return &seedContext{
		TesteeIDsByName:             make(map[string]string),
		QuestionnaireVersionsByCode: make(map[string]string),
		ScaleIDsByCode:              make(map[string]string),
	}
}

// ==================== 主程序入口 ====================

func main() {
	// 解析命令行参数
	var (
		apiBaseURL              = flag.String("api-base-url", "", "API base URL (e.g., http://localhost:18082)")
		collectionBaseURL       = flag.String("collection-base-url", "", "Collection server API base URL (defaults to api-base-url)")
		apiToken                = flag.String("api-token", "", "API authentication token")
		configFile              = flag.String("config", "", "Base seed data config file (testees, legacy data)")
		stepsRaw                = flag.String("steps", "", "Comma-separated steps to run (default: all)")
		assessmentMin           = flag.Int("assessment-min", 5, "Minimum assessments per testee")
		assessmentMax           = flag.Int("assessment-max", 10, "Maximum assessments per testee")
		assessmentWorkers       = flag.Int("assessment-workers", 10, "Concurrent workers for assessment seeding")
		assessmentSubmitWorkers = flag.Int("assessment-submit-workers", 10, "Concurrent workers for assessment submission")
		testeePageSize          = flag.Int("testee-page-size", 1, "Page size when listing testees for assessment seeding")
		testeeOffset            = flag.Int("testee-offset", 0, "Starting offset when listing testees for assessment seeding")
		testeeLimit             = flag.Int("testee-limit", 0, "Maximum number of testees to process for assessment seeding (0 = no limit)")
		assessmentCategories    = flag.String("assessment-scale-categories", "", "Comma-separated scale categories to include (defaults to all)")
		verbose                 = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()
	steps := parseSteps(*stepsRaw)

	// 初始化日志
	logOpts := log.NewOptions()
	if *verbose {
		logOpts.Level = "debug"
	} else {
		logOpts.Level = "info"
	}
	log.Init(logOpts)
	logger := log.L(context.Background())

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

	// 创建上下文
	seedCtx := newSeedContext()
	runCtx := context.Background()

	if *assessmentMin <= 0 || *assessmentMax <= 0 || *assessmentMax < *assessmentMin {
		logger.Fatalw("Invalid assessment range", "min", *assessmentMin, "max", *assessmentMax)
	}

	for _, step := range steps {
		switch step {
		case stepAssessment:
			if err := seedAssessments(runCtx, deps, seedCtx, *assessmentMin, *assessmentMax, *assessmentWorkers, *assessmentSubmitWorkers, *testeePageSize, *testeeOffset, *testeeLimit, *assessmentCategories, *verbose); err != nil {
				logger.Fatalw("Assessment seeding failed", "error", err)
			}
		default:
			logger.Warnw("Skipping unimplemented step", "step", step)
		}
	}

	logger.Infow("Seed process completed successfully")
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
