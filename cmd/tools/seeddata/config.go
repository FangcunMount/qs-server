package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FlexFloat 是一个可以从 YAML 字符串或数字解析的浮点数类型
type FlexFloat float64

// UnmarshalYAML 实现 yaml.Unmarshaler 接口，支持字符串和数字格式
func (f *FlexFloat) UnmarshalYAML(node *yaml.Node) error {
	var floatVal float64
	if err := node.Decode(&floatVal); err == nil {
		*f = FlexFloat(floatVal)
		return nil
	}

	var strVal string
	if err := node.Decode(&strVal); err == nil {
		if strVal == "" {
			*f = 0
			return nil
		}
		parsed, err := strconv.ParseFloat(strVal, 64)
		if err != nil {
			return fmt.Errorf("cannot parse '%s' as float: %w", strVal, err)
		}
		*f = FlexFloat(parsed)
		return nil
	}

	return fmt.Errorf("cannot unmarshal %v into FlexFloat", node.Value)
}

// Float64 返回底层的 float64 值
func (f FlexFloat) Float64() float64 {
	return float64(f)
}

// BoolString 兼容布尔值或字符串的字段，最终存储为字符串
type BoolString string

// UnmarshalYAML 支持 bool / string
func (b *BoolString) UnmarshalYAML(node *yaml.Node) error {
	var boolVal bool
	if err := node.Decode(&boolVal); err == nil {
		if boolVal {
			*b = "1"
		} else {
			*b = "0"
		}
		return nil
	}

	var strVal string
	if err := node.Decode(&strVal); err == nil {
		*b = BoolString(strVal)
		return nil
	}

	return fmt.Errorf("cannot unmarshal %v into BoolString", node.Value)
}

// SeedConfig 定义整个种子数据配置结构
type SeedConfig struct {
	// 全局配置
	Global GlobalConfig `yaml:"global"`
	// API 配置
	API APIConfig `yaml:"api"`
	// IAM 配置
	IAM IAMConfig `yaml:"iam"`
	// 本地 plan runtime 配置
	Local LocalRuntimeConfig `yaml:"local"`

	// 各个领域的种子数据配置
	Staffs                  []StaffConfig                 `yaml:"staffs"`
	Clinicians              []ClinicianConfig             `yaml:"clinicians"`
	ClinicianGenerators     []ClinicianGeneratorConfig    `yaml:"clinicianGenerators"`
	TesteeAssignments       []TesteeAssignmentConfig      `yaml:"testeeAssignments"`
	ActorTimeline           ActorTimelineConfig           `yaml:"actorTimeline"`
	AssessmentEntryTargets  []AssessmentEntryTargetConfig `yaml:"assessmentEntryTargets"`
	AssessmentEntryFlow     AssessmentEntryFlowConfig     `yaml:"assessmentEntryFlow"`
	AssessmentByEntry       AssessmentByEntryConfig       `yaml:"assessmentByEntry"`
	DailySimulation         DailySimulationConfig         `yaml:"dailySimulation"`
	AssessmentStatusProfile AssessmentStatusProfileConfig `yaml:"assessmentStatusProfile"`
	Testees                 []TesteeConfig                `yaml:"testees"`
	Questionnaires          []QuestionnaireConfig         `yaml:"questionnaires"`
	Scales                  []ScaleConfig                 `yaml:"scales"`
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

// LocalRuntimeConfig seeddata 本地 plan runtime 配置。
type LocalRuntimeConfig struct {
	MySQLDSN         string `yaml:"mysql_dsn"`
	MongoURI         string `yaml:"mongo_uri"`
	MongoDatabase    string `yaml:"mongo_database"`
	RedisAddr        string `yaml:"redis_addr"`
	RedisUsername    string `yaml:"redis_username"`
	RedisPassword    string `yaml:"redis_password"`
	RedisDB          int    `yaml:"redis_db"`
	PlanEntryBaseURL string `yaml:"plan_entry_base_url"`
}

// RetryConfig defines retry behavior for API calls.
type RetryConfig struct {
	MaxRetries int    `yaml:"maxRetries"` // Max retry attempts (not counting the first request)
	MinDelay   string `yaml:"minDelay"`   // e.g. "200ms"
	MaxDelay   string `yaml:"maxDelay"`   // e.g. "5s"
}

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

// StaffConfig 员工账号种子配置。
type StaffConfig struct {
	Key      string     `yaml:"key"`
	UserID   FlexibleID `yaml:"userId"`
	Name     string     `yaml:"name"`
	Email    string     `yaml:"email"`
	Phone    string     `yaml:"phone"`
	Password string     `yaml:"password"`
	Roles    []string   `yaml:"roles"`
	IsActive *bool      `yaml:"isActive"`
}

// ClinicianConfig 临床医师种子配置。
type ClinicianConfig struct {
	Key           string     `yaml:"key"`
	OperatorRef   string     `yaml:"operatorRef"`
	OperatorID    FlexibleID `yaml:"operatorId"`
	Name          string     `yaml:"name"`
	Department    string     `yaml:"department"`
	Title         string     `yaml:"title"`
	ClinicianType string     `yaml:"clinicianType"`
	EmployeeCode  string     `yaml:"employeeCode"`
	IsActive      *bool      `yaml:"isActive"`
}

// ClinicianGeneratorConfig 批量生成虚拟临床医师配置。
type ClinicianGeneratorConfig struct {
	KeyPrefix            string   `yaml:"keyPrefix"`
	StaffKeyPrefix       string   `yaml:"staffKeyPrefix"`
	NamePrefix           string   `yaml:"namePrefix"`
	NameSourceURLPattern string   `yaml:"nameSourceUrlPattern"`
	NameSourcePages      int      `yaml:"nameSourcePages"`
	EmployeeCodePrefix   string   `yaml:"employeeCodePrefix"`
	PhonePrefix          string   `yaml:"phonePrefix"`
	EmailDomain          string   `yaml:"emailDomain"`
	Password             string   `yaml:"password"`
	StaffRoles           []string `yaml:"staffRoles"`
	GenerateStaff        *bool    `yaml:"generateStaff"`
	Count                int      `yaml:"count"`
	StartIndex           int      `yaml:"startIndex"`
	Departments          []string `yaml:"departments"`
	Titles               []string `yaml:"titles"`
	ClinicianType        string   `yaml:"clinicianType"`
	IsActive             *bool    `yaml:"isActive"`
}

// TesteeAssignmentConfig 受试者分配配置。
type TesteeAssignmentConfig struct {
	Key                    string       `yaml:"key"`
	Strategy               string       `yaml:"strategy"`
	RelationType           string       `yaml:"relationType"`
	SourceType             string       `yaml:"sourceType"`
	ClinicianRef           string       `yaml:"clinicianRef"`
	ClinicianID            FlexibleID   `yaml:"clinicianId"`
	ClinicianRefs          []string     `yaml:"clinicianRefs"`
	ClinicianKeyPrefixes   []string     `yaml:"clinicianKeyPrefixes"`
	ClinicianIDs           []FlexibleID `yaml:"clinicianIds"`
	TesteeIDs              []FlexibleID `yaml:"testeeIds"`
	TesteeOffset           int          `yaml:"testeeOffset"`
	TesteeLimit            int          `yaml:"testeeLimit"`
	TesteePageSize         int          `yaml:"testeePageSize"`
	FocusTargetCount       int          `yaml:"focusTargetCount"`
	FocusTargetRatio       FlexFloat    `yaml:"focusTargetRatio"`
	IncludeAlreadyAssigned bool         `yaml:"includeAlreadyAssigned"`
}

// ActorTimelineConfig actor/relation 时间分布配置。
type ActorTimelineConfig struct {
	WaveInterval   string `yaml:"waveInterval"`
	WaveWeeks      int    `yaml:"waveWeeks"`
	DayStartHour   int    `yaml:"dayStartHour"`
	DayEndHour     int    `yaml:"dayEndHour"`
	SlotInterval   string `yaml:"slotInterval"`
	WaveDaysOfWeek []int  `yaml:"waveDaysOfWeek"`
}

// AssessmentEntryTargetConfig clinician 共享测评入口目标配置。
type AssessmentEntryTargetConfig struct {
	Key           string `yaml:"key"`
	TargetType    string `yaml:"targetType"`
	TargetCode    string `yaml:"targetCode"`
	TargetVersion string `yaml:"targetVersion"`
	ExpiresAt     string `yaml:"expiresAt"`
	ExpiresAfter  string `yaml:"expiresAfter"`
}

// AssessmentEntryFlowConfig 入口 resolve/intake 批处理配置。
type AssessmentEntryFlowConfig struct {
	ClinicianRefs        []string     `yaml:"clinicianRefs"`
	ClinicianKeyPrefixes []string     `yaml:"clinicianKeyPrefixes"`
	ClinicianIDs         []FlexibleID `yaml:"clinicianIds"`
	EntryIDs             []FlexibleID `yaml:"entryIDs"`
	MaxIntakesPerEntry   int          `yaml:"maxIntakesPerEntry"`
	AllowTemporaryTestee bool         `yaml:"allowTemporaryTestee"`
}

// AssessmentByEntryConfig 基于入口 intake 结果继续创建测评的配置。
type AssessmentByEntryConfig struct {
	ClinicianRefs          []string     `yaml:"clinicianRefs"`
	ClinicianKeyPrefixes   []string     `yaml:"clinicianKeyPrefixes"`
	ClinicianIDs           []FlexibleID `yaml:"clinicianIds"`
	EntryIDs               []FlexibleID `yaml:"entryIDs"`
	MaxAssessmentsPerEntry int          `yaml:"maxAssessmentsPerEntry"`
}

// DailySimulationConfig 每日模拟真实用户注册/建档/扫码/填报。
type DailySimulationConfig struct {
	CountPerRun      int        `yaml:"countPerRun"`
	Workers          int        `yaml:"workers"`
	RunDate          string     `yaml:"runDate"`
	ClinicianRef     string     `yaml:"clinicianRef"`
	ClinicianID      FlexibleID `yaml:"clinicianId"`
	EntryID          FlexibleID `yaml:"entryId"`
	TargetType       string     `yaml:"targetType"`
	TargetCode       string     `yaml:"targetCode"`
	TargetVersion    string     `yaml:"targetVersion"`
	UserPassword     string     `yaml:"userPassword"`
	UserPhonePrefix  string     `yaml:"userPhonePrefix"`
	UserEmailDomain  string     `yaml:"userEmailDomain"`
	GuardianRelation string     `yaml:"guardianRelation"`
	TesteeSource     string     `yaml:"testeeSource"`
	TesteeTags       []string   `yaml:"testeeTags"`
	IsKeyFocus       bool       `yaml:"isKeyFocus"`
}

// AssessmentStatusProfileConfig 第二阶段状态分布配置。
type AssessmentStatusProfileConfig struct {
	Pending     float64 `yaml:"pending"`
	Submitted   float64 `yaml:"submitted"`
	Interpreted float64 `yaml:"interpreted"`
	Failed      float64 `yaml:"failed"`
}

// TesteeConfig 受试者配置
type TesteeConfig struct {
	Name       string   `yaml:"name"`
	Gender     string   `yaml:"gender"`     // "male", "female", "unknown"
	Birthday   string   `yaml:"birthday"`   // "2010-01-15" 格式
	ProfileID  *uint64  `yaml:"profileId"`  // 可选：IAM用户档案ID
	Tags       []string `yaml:"tags"`       // 业务标签
	Source     string   `yaml:"source"`     // 数据来源
	IsKeyFocus bool     `yaml:"isKeyFocus"` // 是否重点关注
}

// QuestionnaireConfig 问卷配置（兼容 survey_questionnaires.yaml 的结构）
type QuestionnaireConfig struct {
	Code        string           `yaml:"code"`
	Name        string           `yaml:"name"` // 数据文件里的标题字段
	Title       string           `yaml:"title"`
	Description string           `yaml:"description"`
	ImgUrl      string           `yaml:"imgUrl"`
	Icon        string           `yaml:"icon"`
	Version     string           `yaml:"version"`
	Status      string           `yaml:"status"` // "draft", "published", "archived"
	Questions   []QuestionConfig `yaml:"questions"`
}

// QuestionConfig 问题配置，兼容问卷和量表的题目结构
type QuestionConfig struct {
	Code           string         `yaml:"code"`
	Type           string         `yaml:"type"`
	Text           string         `yaml:"text"`  // 旧格式字段
	Title          string         `yaml:"title"` // 新格式字段
	Tips           string         `yaml:"tips"`
	Router         any            `yaml:"router"`
	ShowController any            `yaml:"show_controller"` // 可能是 null/map，放宽为 any
	Description    string         `yaml:"description"`     // 问题描述
	Required       bool           `yaml:"required"`
	Order          int            `yaml:"order"`
	Options        []OptionConfig `yaml:"options"` // 选项列表
	CalcRule       CalcRuleConfig `yaml:"calc_rule"`
	MaxScore       string         `yaml:"max_score"`
	ValidateRules  struct {
		Required string `yaml:"required"`
	} `yaml:"validate_rules"`
}

// CalcRuleConfig 计算规则配置
type CalcRuleConfig struct {
	Formula      string                 `yaml:"formula"`
	AppendParams map[string]interface{} `yaml:"append_params"`
}

// OptionConfig 选项配置，保留原始数据字段
type OptionConfig struct {
	Code              string     `yaml:"code"`
	Text              string     `yaml:"text"`    // 旧格式字段
	Content           string     `yaml:"content"` // 新格式字段
	Score             FlexFloat  `yaml:"score"`
	Order             int        `yaml:"order"`
	IsSelect          BoolString `yaml:"is_select"`
	IsOther           BoolString `yaml:"is_other"`
	AllowExtendText   BoolString `yaml:"allow_extend_text"`
	ExtendContent     string     `yaml:"extend_content"`
	ExtendPlaceholder string     `yaml:"extend_placeholder"`
}

// ScaleConfig 量表配置
type ScaleConfig struct {
	Code                 string                    `yaml:"code"`
	Name                 string                    `yaml:"name"`
	Title                string                    `yaml:"title"`
	Description          string                    `yaml:"description"`
	Icon                 string                    `yaml:"icon"`
	Category             string                    `yaml:"category"`       // 主类
	Stages               []string                  `yaml:"stages"`         // 阶段列表
	ApplicableAges       []string                  `yaml:"applicableAges"` // 使用年龄列表（注意：YAML中是驼峰命名）
	Reporters            []string                  `yaml:"reporters"`      // 填报人列表
	Tags                 []string                  `yaml:"tags"`           // 标签列表
	QuestionnaireCode    string                    `yaml:"questionnaireCode"`
	QuestionnaireVersion string                    `yaml:"questionnaireVersion"`
	Status               string                    `yaml:"status"` // "draft", "published", "archived"
	Factors              []FactorConfig            `yaml:"factors"`
	Interpretation       InterpretationGroupConfig `yaml:"interpretation"`
	Questions            []QuestionConfig          `yaml:"questions"`
}

// FactorConfig 因子配置
type FactorConfig struct {
	Code            string                    `yaml:"code"`
	Title           string                    `yaml:"title"`
	Name            string                    `yaml:"name"`
	Description     string                    `yaml:"description"`
	QuestionCodes   []string                  `yaml:"questionCodes"`   // 关联的问题编码
	Interpretations []InterpretationConfig    `yaml:"interpretations"` // 解读规则（旧格式）
	InterpretRule   InterpretationGroupConfig `yaml:"interpret_rule"`  // 兼容新字段
	Type            string                    `yaml:"type"`
	IsTotalScore    string                    `yaml:"is_total_score"`
	SourceCodes     []string                  `yaml:"source_codes"`
	CalcRule        CalcRuleConfig            `yaml:"calc_rule"`
	MaxScore        string                    `yaml:"max_score"`
}

// InterpretationConfig 解读规则配置
type InterpretationConfig struct {
	MinScore    *float64 `yaml:"minScore"`
	MaxScore    *float64 `yaml:"maxScore"`
	Start       string   `yaml:"start"`
	End         string   `yaml:"end"`
	Content     string   `yaml:"content"`
	Level       string   `yaml:"level"`       // "low", "medium", "high"
	RiskLevel   string   `yaml:"risk_level"`  // 风险等级：none, low, medium, high, severe
	Description string   `yaml:"description"` // 解读文本
}

// InterpretationGroupConfig 解读规则组（包含显示开关和规则列表）
type InterpretationGroupConfig struct {
	IsShow         string                 `yaml:"is_show"`
	Items          []InterpretationConfig `yaml:"interpretation"`
	Interpretation []InterpretationConfig `yaml:"interpretations"` // 兼容另一种命名
}

// LoadSeedConfig 从 YAML 文件加载种子数据配置
func LoadSeedConfig(filepath string) (*SeedConfig, error) {
	return LoadSeedConfigWithPreference(filepath, false)
}

// LoadSeedConfigWithPreference 允许指定是否优先按 Scales 解析（用于量表问卷文件）
func LoadSeedConfigWithPreference(filepath string, preferScale bool) (*SeedConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config SeedConfig
	if err := yaml.Unmarshal(data, &config); err == nil {
		if len(config.Staffs) > 0 ||
			len(config.Clinicians) > 0 ||
			len(config.ClinicianGenerators) > 0 ||
			len(config.TesteeAssignments) > 0 ||
			!isEmptyDailySimulationConfig(config.DailySimulation) ||
			len(config.Testees) > 0 ||
			len(config.Questionnaires) > 0 ||
			len(config.Scales) > 0 ||
			config.Global != (GlobalConfig{}) {
			return &config, nil
		}
	}

	// 兼容问卷/量表列表顶层直接是数组的配置文件（可指定优先解析量表）
	var questionnaires []QuestionnaireConfig
	var scales []ScaleConfig

	// 如果偏好量表，先尝试解析量表
	if preferScale {
		_ = yaml.Unmarshal(data, &scales)
		if len(scales) > 0 {
			return &SeedConfig{Scales: scales}, nil
		}
	}

	if err := yaml.Unmarshal(data, &questionnaires); err == nil && len(questionnaires) > 0 && !preferScale {
		return &SeedConfig{Questionnaires: questionnaires}, nil
	}

	if err := yaml.Unmarshal(data, &scales); err == nil && len(scales) > 0 {
		hasScaleSignal := false
		for _, s := range scales {
			if len(s.Factors) > 0 || s.QuestionnaireVersion != "" || !isEmptyInterpretationGroup(s.Interpretation) {
				hasScaleSignal = true
				break
			}
		}
		if hasScaleSignal {
			return &SeedConfig{Scales: scales}, nil
		}
	}

	// 如果 preferScale=false 且问卷解析成功（即使 scales 存在，也优先问卷）
	if len(questionnaires) > 0 {
		return &SeedConfig{Questionnaires: questionnaires}, nil
	}

	return nil, fmt.Errorf("failed to parse config file: unrecognized format")
}

// 判断解读规则组是否为空
func isEmptyInterpretationGroup(g InterpretationGroupConfig) bool {
	return g.IsShow == "" && len(g.Items) == 0 && len(g.Interpretation) == 0
}

func isEmptyDailySimulationConfig(cfg DailySimulationConfig) bool {
	return cfg.CountPerRun == 0 &&
		cfg.Workers == 0 &&
		strings.TrimSpace(cfg.RunDate) == "" &&
		strings.TrimSpace(cfg.ClinicianRef) == "" &&
		cfg.ClinicianID.IsZero() &&
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
		!cfg.IsKeyFocus
}

// ParseDate 解析日期字符串
func ParseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

// ParseGender 解析性别字符串
func ParseGender(genderStr string) int8 {
	switch genderStr {
	case "male":
		return 1
	case "female":
		return 2
	default:
		return 0 // unknown
	}
}

// QuestionText 返回问题展示文本，兼容 text/title 字段
func (q QuestionConfig) QuestionText() string {
	if q.Text != "" {
		return q.Text
	}
	return q.Title
}

// OptionContent 返回选项展示文本，兼容 text/content 字段
func (o OptionConfig) OptionContent() string {
	if o.Text != "" {
		return o.Text
	}
	return o.Content
}
