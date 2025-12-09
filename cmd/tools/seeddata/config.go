package main

import (
	"fmt"
	"os"
	"strconv"
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

	// 各个领域的种子数据配置
	Testees        []TesteeConfig        `yaml:"testees"`
	Questionnaires []QuestionnaireConfig `yaml:"questionnaires"`
	Scales         []ScaleConfig         `yaml:"scales"`
}

// GlobalConfig 全局配置
type GlobalConfig struct {
	OrgID      int64  `yaml:"orgId"`      // 默认机构ID
	DefaultTag string `yaml:"defaultTag"` // 默认标签前缀
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
		if len(config.Testees) > 0 || len(config.Questionnaires) > 0 || len(config.Scales) > 0 || config.Global != (GlobalConfig{}) {
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
