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
// - seed_testee.go: Testee data seeding
// - seed_questionnaire.go: Questionnaire data seeding
// - seed_scale.go: Medical scale data seeding
// - seed_answersheet.go: Answer sheet data seeding
// - seed_assessment.go: Assessment data seeding
//
// Usage:
//
//	go run ./cmd/tools/seeddata \
//	  --mysql-dsn "user:pass@tcp(host:port)/qs_apiserver" \
//	  --mongo-uri "mongodb://host:port" \
//	  --mongo-database "qs_apiserver" \
//	  --config cmd/tools/seeddata/data/seeddata.yaml \
//	  --questionnaire-config cmd/tools/seeddata/data/survey_questionnaires.yaml \
//	  --scale-questionnaire-config cmd/tools/seeddata/data/medical_scales.yaml
//
// See README.md for detailed documentation.
package main

import (
	"context"
	"flag"
	"os"
	"strings"

	"github.com/FangcunMount/component-base/pkg/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// ==================== 核心类型定义 ====================

// seedStep represents a specific seeding step.
type seedStep string

// All available seed steps.
const (
	stepQuestionnaire seedStep = "questionnaire" // 创建问卷
	stepScale         seedStep = "scale"         // 创建量表
)

// defaultSteps defines the default execution order of all seed steps.
var defaultSteps = []seedStep{
	stepQuestionnaire,
	stepScale,
}

// dependencies holds all external dependencies required by seed functions.
type dependencies struct {
	MySQLDB   *gorm.DB        // MySQL数据库连接（可选，用于旧代码兼容）
	MongoDB   *mongo.Database // MongoDB数据库连接（可选，用于旧代码兼容）
	Logger    log.Logger      // 日志记录器
	Config    *SeedConfig     // 种子数据配置
	APIClient *APIClient      // HTTP API 客户端
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
		apiBaseURL             = flag.String("api-base-url", os.Getenv("API_BASE_URL"), "API base URL (e.g., http://localhost:18082)")
		apiToken               = flag.String("api-token", os.Getenv("API_TOKEN"), "API authentication token")
		mysqlDSN               = flag.String("mysql-dsn", os.Getenv("MYSQL_DSN"), "MySQL DSN (deprecated, not used when using API)")
		mongoURI               = flag.String("mongo-uri", os.Getenv("MONGO_URI"), "MongoDB URI (deprecated, not used when using API)")
		mongoDatabase          = flag.String("mongo-database", os.Getenv("MONGO_DATABASE"), "MongoDB Database (deprecated, not used when using API)")
		configFile             = flag.String("config", "", "Base seed data config file (testees, legacy data)")
		questionnaireFile      = flag.String("questionnaire-config", "cmd/tools/seeddata/data/survey_questionnaires.yaml", "Questionnaire config file")
		scaleQuestionnaireFile = flag.String("scale-questionnaire-config", "cmd/tools/seeddata/data/medical_scales.yaml", "Medical scale questionnaire config file")
		stepsRaw               = flag.String("steps", "", "Comma-separated steps to run (default: all)")
		verbose                = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

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

	// 加载问卷配置（覆盖/补充问卷数据，保持原始内容）
	if strings.TrimSpace(*questionnaireFile) != "" {
		logger.Infow("Loading questionnaire config", "file", *questionnaireFile)
		qCfg, err := LoadSeedConfig(*questionnaireFile)
		if err != nil {
			logger.Fatalw("Failed to load questionnaire config", "error", err)
		}

		if qCfg.Global.OrgID != 0 || qCfg.Global.DefaultTag != "" {
			config.Global = qCfg.Global
		}
		config.Questionnaires = append([]QuestionnaireConfig{}, qCfg.Questionnaires...)
	}

	// 加载量表问卷配置（追加到问卷列表）
	if strings.TrimSpace(*scaleQuestionnaireFile) != "" {
		logger.Infow("Loading medical scale questionnaire config", "file", *scaleQuestionnaireFile)
		scaleCfg, err := LoadSeedConfigWithPreference(*scaleQuestionnaireFile, true)
		if err != nil {
			logger.Fatalw("Failed to load medical scale questionnaire config", "error", err)
		}

		if (config.Global == GlobalConfig{}) && (scaleCfg.Global.OrgID != 0 || scaleCfg.Global.DefaultTag != "") {
			config.Global = scaleCfg.Global
		}
		config.Scales = scaleCfg.Scales
		config.Questionnaires = append(config.Questionnaires, scaleCfg.Questionnaires...)

		if len(scaleCfg.Scales) == 0 {
			logger.Warnw("No scale definitions found in medical scale config; scale seeding will be skipped unless provided elsewhere", "file", *scaleQuestionnaireFile)
		}
	}

	// 初始化 API 客户端
	if strings.TrimSpace(*apiBaseURL) == "" {
		logger.Fatalw("API base URL is required, set via --api-base-url or API_BASE_URL env var")
	}
	if strings.TrimSpace(*apiToken) == "" {
		logger.Fatalw("API token is required, set via --api-token or API_TOKEN env var")
	}

	apiClient := NewAPIClient(*apiBaseURL, *apiToken, logger)
	logger.Infow("Initialized API client", "base_url", *apiBaseURL)

	// 可选：连接数据库（用于兼容旧代码，但新代码应使用 API）
	var mysqlDB *gorm.DB
	var mongoDB *mongo.Database
	if strings.TrimSpace(*mysqlDSN) != "" && strings.TrimSpace(*mongoURI) != "" {
		logger.Infow("Connecting to databases (for compatibility)", "mysql_dsn", maskDSN(*mysqlDSN))
		db, err := gorm.Open(mysql.Open(*mysqlDSN), &gorm.Config{})
		if err != nil {
			logger.Warnw("Failed to connect to MySQL (optional)", "error", err)
		} else {
			mysqlDB = db
		}

		logger.Infow("Connecting to MongoDB (for compatibility)", "uri", maskURI(*mongoURI), "database", *mongoDatabase)
		mongoClient, err := mongo.Connect(context.Background(), options.Client().ApplyURI(*mongoURI))
		if err != nil {
			logger.Warnw("Failed to connect to MongoDB (optional)", "error", err)
		} else {
			defer mongoClient.Disconnect(context.Background())
			mongoDB = mongoClient.Database(*mongoDatabase)
		}
	}

	// 构建依赖
	deps := &dependencies{
		MySQLDB:   mysqlDB,
		MongoDB:   mongoDB,
		Logger:    logger,
		Config:    config,
		APIClient: apiClient,
	}

	// 解析要执行的步骤
	steps := parseSteps(*stepsRaw)
	logger.Infow("Starting seed process", "steps", stepListToStrings(steps))

	// 创建上下文
	state := newSeedContext()
	ctx := context.Background()

	// 依次执行各个步骤
	for _, step := range steps {
		logger.Infow("Executing step", "step", step)

		var err error
		switch step {
		case stepQuestionnaire:
			err = seedQuestionnaires(ctx, deps, state)
		case stepScale:
			err = seedScales(ctx, deps, state)
		default:
			logger.Warnw("Unknown step", "step", step)
			continue
		}

		if err != nil {
			logger.Fatalw("Step failed", "step", step, "error", err)
		}

		logger.Infow("Step completed", "step", step)
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

// maskDSN 遮蔽 DSN 中的密码
func maskDSN(dsn string) string {
	if idx := strings.Index(dsn, "@"); idx > 0 {
		if colonIdx := strings.Index(dsn[:idx], ":"); colonIdx > 0 {
			return dsn[:colonIdx+1] + "***" + dsn[idx:]
		}
	}
	return dsn
}

// maskURI 遮蔽 URI 中的密码
func maskURI(uri string) string {
	if idx := strings.Index(uri, "@"); idx > 0 {
		if colonIdx := strings.Index(uri, "://"); colonIdx > 0 {
			userPassStart := colonIdx + 3
			if colonInUserPass := strings.Index(uri[userPassStart:idx], ":"); colonInUserPass > 0 {
				return uri[:userPassStart+colonInUserPass+1] + "***" + uri[idx:]
			}
		}
	}
	return uri
}
