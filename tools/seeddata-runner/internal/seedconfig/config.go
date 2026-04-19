package seedconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedapi"
	"gopkg.in/yaml.v3"
)

const (
	DefaultDailySimulationCountPerRun = 10
	DefaultDailySimulationWorkers     = 4
	DefaultDailySimulationRunAt       = "10:00"
	DefaultDailySimulationRetryDelay  = "30m"
	DefaultDailySimulationStateFile   = ".seeddata-cache/daily-simulation-daemon-state.json"
	DefaultDailySimulationPhonePrefix = "+86199"
	DefaultDailySimulationEmailDomain = "fangcunmount.com"
	DefaultDailySimulationPassword    = "DailySim@123"
	DefaultDailySimulationSource      = "daily_simulation"
	DefaultPlanSubmitWorkers          = 1
)

// Config 定义整个种子数据配置结构
type Config struct {
	Global          GlobalConfig          `yaml:"global"`
	API             APIConfig             `yaml:"api"`
	IAM             IAMConfig             `yaml:"iam"`
	DailySimulation DailySimulationConfig `yaml:"dailySimulation"`
	PlanSubmit      PlanSubmitConfig      `yaml:"planSubmit"`
}

// GlobalConfig 全局配置
type GlobalConfig struct {
	OrgID      int64  `yaml:"orgId"`      // 默认机构ID
	DefaultTag string `yaml:"defaultTag"` // 默认标签前缀
}

// APIConfig API 配置
type APIConfig struct {
	BaseURL           string      `yaml:"baseUrl"`
	CollectionBaseURL string      `yaml:"collectionBaseUrl"`
	Token             string      `yaml:"token"`
	Retry             RetryConfig `yaml:"retry"`
}

// IAMConfig IAM 登录配置
type IAMConfig struct {
	BaseURL  string        `yaml:"baseUrl"`
	LoginURL string        `yaml:"loginUrl"`
	Username string        `yaml:"username"`
	Password string        `yaml:"password"`
	TenantID string        `yaml:"tenantId"`
	GRPC     IAMGRPCConfig `yaml:"grpc"`
}

type IAMGRPCConfig struct {
	Address  string           `yaml:"address"`
	Timeout  string           `yaml:"timeout"`
	RetryMax int              `yaml:"retryMax"`
	TLS      IAMGRPCTLSConfig `yaml:"tls"`
}

type IAMGRPCTLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"caFile"`
	CertFile           string `yaml:"certFile"`
	KeyFile            string `yaml:"keyFile"`
	ServerName         string `yaml:"serverName"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

type RetryConfig = seedapi.RetryConfig

// FlexibleID 支持 YAML 字符串或数字格式的 ID 字段。
type FlexibleID string

// UnmarshalYAML 支持 string / number。
func (f *FlexibleID) UnmarshalYAML(node *yaml.Node) error {
	var strVal string
	if err := node.Decode(&strVal); err == nil {
		*f = FlexibleID(strings.TrimSpace(strVal))
		return nil
	}

	var uintVal uint64
	if err := node.Decode(&uintVal); err == nil {
		*f = FlexibleID(strconv.FormatUint(uintVal, 10))
		return nil
	}

	var intVal int64
	if err := node.Decode(&intVal); err == nil {
		if intVal < 0 {
			return fmt.Errorf("cannot parse negative id %d", intVal)
		}
		*f = FlexibleID(strconv.FormatUint(uint64(intVal), 10))
		return nil
	}

	return fmt.Errorf("cannot unmarshal %v into FlexibleID", node.Value)
}

// String 返回底层字符串值。
func (f FlexibleID) String() string {
	return strings.TrimSpace(string(f))
}

// IsZero 判断 ID 是否为空。
func (f FlexibleID) IsZero() bool {
	return f.String() == ""
}

// Uint64 将 ID 转换为 uint64。
func (f FlexibleID) Uint64() (uint64, error) {
	if f.IsZero() {
		return 0, nil
	}
	value, err := strconv.ParseUint(f.String(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse '%s' as uint64: %w", f.String(), err)
	}
	return value, nil
}

// DailySimulationConfig 每日模拟真实用户注册/建档/扫码/填报。
type DailySimulationConfig struct {
	CountPerRun              int                             `yaml:"countPerRun"`
	CountMin                 int                             `yaml:"countMin"`
	CountMax                 int                             `yaml:"countMax"`
	Workers                  int                             `yaml:"workers"`
	RunDate                  string                          `yaml:"runDate"`
	RunAt                    string                          `yaml:"runAt"`
	RetryDelay               string                          `yaml:"retryDelay"`
	StateFile                string                          `yaml:"stateFile"`
	ClinicianIDs             []FlexibleID                    `yaml:"clinicianIds"`
	FocusCliniciansPerRunMin int                             `yaml:"focusCliniciansPerRunMin"`
	FocusCliniciansPerRunMax int                             `yaml:"focusCliniciansPerRunMax"`
	EntryID                  FlexibleID                      `yaml:"entryId"`
	TargetType               string                          `yaml:"targetType"`
	TargetCode               string                          `yaml:"targetCode"`
	TargetVersion            string                          `yaml:"targetVersion"`
	UserPassword             string                          `yaml:"userPassword"`
	UserPhonePrefix          string                          `yaml:"userPhonePrefix"`
	UserEmailDomain          string                          `yaml:"userEmailDomain"`
	GuardianRelation         string                          `yaml:"guardianRelation"`
	TesteeSource             string                          `yaml:"testeeSource"`
	TesteeTags               []string                        `yaml:"testeeTags"`
	IsKeyFocus               bool                            `yaml:"isKeyFocus"`
	PlanIDs                  []FlexibleID                    `yaml:"planIds"`
	JourneyMix               DailySimulationJourneyMixConfig `yaml:"journeyMix"`
}

type DailySimulationJourneyMixConfig struct {
	RegisterOnlyWeight int `yaml:"registerOnlyWeight"`
	CreateTesteeWeight int `yaml:"createTesteeWeight"`
	ResolveEntryWeight int `yaml:"resolveEntryWeight"`
	SubmitAnswerWeight int `yaml:"submitAnswerWeight"`
}

type PlanSubmitConfig struct {
	PlanIDs []FlexibleID `yaml:"planIds"`
	Workers int          `yaml:"workers"`
}

func Load(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err == nil {
		applyEnvOverrides(&config)
		config.Normalize()
		if err := config.Validate(); err != nil {
			return nil, err
		}
		if config.Global != (GlobalConfig{}) ||
			config.API != (APIConfig{}) ||
			config.IAM != (IAMConfig{}) ||
			!config.DailySimulation.IsZero() ||
			!config.PlanSubmit.IsZero() {
			return &config, nil
		}
	}

	return nil, fmt.Errorf("failed to parse config file: unrecognized format")
}

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if username := strings.TrimSpace(os.Getenv("IAM_USERNAME")); username != "" {
		cfg.IAM.Username = username
	}
	if password := strings.TrimSpace(os.Getenv("IAM_PASSWORD")); password != "" {
		cfg.IAM.Password = password
	}
}

func (cfg *Config) Normalize() {
	if cfg == nil {
		return
	}

	cfg.DailySimulation.Normalize()
	cfg.PlanSubmit.Normalize()
}

func (cfg *Config) Validate() error {
	if cfg == nil {
		return fmt.Errorf("seeddata config is nil")
	}
	if cfg.DailySimulation.IsZero() {
		return fmt.Errorf("dailySimulation config is required")
	}
	if err := cfg.DailySimulation.Validate(); err != nil {
		return err
	}
	if err := cfg.PlanSubmit.Validate(); err != nil {
		return err
	}
	return nil
}

func (cfg DailySimulationConfig) IsZero() bool {
	return cfg.CountPerRun == 0 &&
		cfg.CountMin == 0 &&
		cfg.CountMax == 0 &&
		cfg.Workers == 0 &&
		strings.TrimSpace(cfg.RunDate) == "" &&
		strings.TrimSpace(cfg.RunAt) == "" &&
		strings.TrimSpace(cfg.RetryDelay) == "" &&
		strings.TrimSpace(cfg.StateFile) == "" &&
		len(cfg.ClinicianIDs) == 0 &&
		cfg.FocusCliniciansPerRunMin == 0 &&
		cfg.FocusCliniciansPerRunMax == 0 &&
		cfg.EntryID.IsZero() &&
		strings.TrimSpace(cfg.TargetType) == "" &&
		strings.TrimSpace(cfg.TargetCode) == "" &&
		strings.TrimSpace(cfg.TargetVersion) == "" &&
		strings.TrimSpace(cfg.UserPassword) == "" &&
		strings.TrimSpace(cfg.UserPhonePrefix) == "" &&
		strings.TrimSpace(cfg.UserEmailDomain) == "" &&
		strings.TrimSpace(cfg.GuardianRelation) == "" &&
		strings.TrimSpace(cfg.TesteeSource) == "" &&
		len(cfg.TesteeTags) == 0 &&
		!cfg.IsKeyFocus &&
		len(cfg.PlanIDs) == 0 &&
		cfg.JourneyMix == (DailySimulationJourneyMixConfig{})
}

func (cfg *DailySimulationConfig) Normalize() {
	if cfg == nil {
		return
	}

	if cfg.CountMin == 0 && cfg.CountMax == 0 && cfg.CountPerRun <= 0 {
		cfg.CountPerRun = DefaultDailySimulationCountPerRun
	}
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultDailySimulationWorkers
	}
	if strings.TrimSpace(cfg.RunAt) == "" {
		cfg.RunAt = DefaultDailySimulationRunAt
	}
	if strings.TrimSpace(cfg.RetryDelay) == "" {
		cfg.RetryDelay = DefaultDailySimulationRetryDelay
	}
	if strings.TrimSpace(cfg.StateFile) == "" {
		cfg.StateFile = DefaultDailySimulationStateFile
	}
	if strings.TrimSpace(cfg.UserPhonePrefix) == "" {
		cfg.UserPhonePrefix = DefaultDailySimulationPhonePrefix
	}
	if strings.TrimSpace(cfg.UserEmailDomain) == "" {
		cfg.UserEmailDomain = DefaultDailySimulationEmailDomain
	}
	if strings.TrimSpace(cfg.UserPassword) == "" {
		cfg.UserPassword = DefaultDailySimulationPassword
	}
	if strings.TrimSpace(cfg.TesteeSource) == "" {
		cfg.TesteeSource = DefaultDailySimulationSource
	}
	if cfg.JourneyMix.totalWeight() == 0 {
		cfg.JourneyMix.SubmitAnswerWeight = 100
	}
	cfg.UserEmailDomain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cfg.UserEmailDomain)), "@")
	cfg.UserPhonePrefix = strings.TrimSpace(cfg.UserPhonePrefix)
	cfg.UserPassword = strings.TrimSpace(cfg.UserPassword)
	cfg.TesteeSource = strings.TrimSpace(cfg.TesteeSource)
	cfg.RunAt = strings.TrimSpace(cfg.RunAt)
	cfg.RetryDelay = strings.TrimSpace(cfg.RetryDelay)
	cfg.StateFile = strings.TrimSpace(cfg.StateFile)
	cfg.PlanIDs = normalizeFlexibleIDs(cfg.PlanIDs)
}

func (cfg DailySimulationConfig) Validate() error {
	if len(cfg.ClinicianIDs) == 0 {
		return fmt.Errorf("dailySimulation.clinicianIds is required")
	}
	if strings.TrimSpace(cfg.TargetType) == "" {
		return fmt.Errorf("dailySimulation.targetType is required")
	}
	if strings.TrimSpace(cfg.TargetCode) == "" {
		return fmt.Errorf("dailySimulation.targetCode is required")
	}
	if len(cfg.PlanIDs) == 0 {
		return fmt.Errorf("dailySimulation.planIds is required")
	}
	if cfg.CountMin < 0 || cfg.CountMax < 0 || cfg.CountPerRun < 0 {
		return fmt.Errorf("dailySimulation counts must be >= 0")
	}
	if cfg.CountMin > 0 && cfg.CountMax > 0 && cfg.CountMax < cfg.CountMin {
		return fmt.Errorf("dailySimulation.countMax must be >= countMin")
	}
	if cfg.FocusCliniciansPerRunMin > 0 && cfg.FocusCliniciansPerRunMax > 0 &&
		cfg.FocusCliniciansPerRunMax < cfg.FocusCliniciansPerRunMin {
		return fmt.Errorf("dailySimulation.focusCliniciansPerRunMax must be >= focusCliniciansPerRunMin")
	}
	if strings.TrimSpace(cfg.RunAt) == "" {
		return fmt.Errorf("dailySimulation.runAt is required")
	}
	return nil
}

func (cfg DailySimulationJourneyMixConfig) totalWeight() int {
	return cfg.RegisterOnlyWeight + cfg.CreateTesteeWeight + cfg.ResolveEntryWeight + cfg.SubmitAnswerWeight
}

func (cfg PlanSubmitConfig) IsZero() bool {
	return len(cfg.PlanIDs) == 0 && cfg.Workers == 0
}

func (cfg *PlanSubmitConfig) Normalize() {
	if cfg == nil {
		return
	}
	cfg.PlanIDs = normalizeFlexibleIDs(cfg.PlanIDs)
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultPlanSubmitWorkers
	}
}

func (cfg PlanSubmitConfig) Validate() error {
	if len(cfg.PlanIDs) == 0 {
		return fmt.Errorf("planSubmit.planIds is required")
	}
	if cfg.Workers <= 0 {
		return fmt.Errorf("planSubmit.workers must be positive")
	}
	return nil
}

func (cfg DailySimulationConfig) PlanIDStrings() []string {
	return flexibleIDsToStrings(cfg.PlanIDs)
}

func (cfg PlanSubmitConfig) PlanIDStrings() []string {
	return flexibleIDsToStrings(cfg.PlanIDs)
}

func normalizeFlexibleIDs(ids []FlexibleID) []FlexibleID {
	if len(ids) == 0 {
		return nil
	}
	normalized := make([]FlexibleID, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		value := strings.TrimSpace(id.String())
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, FlexibleID(value))
	}
	return normalized
}

func flexibleIDsToStrings(ids []FlexibleID) []string {
	if len(ids) == 0 {
		return nil
	}
	values := make([]string, 0, len(ids))
	for _, id := range normalizeFlexibleIDs(ids) {
		values = append(values, id.String())
	}
	return values
}
