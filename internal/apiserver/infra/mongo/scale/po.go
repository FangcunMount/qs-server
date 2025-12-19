package scale

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// ScalePO 量表 MongoDB 持久化对象
type ScalePO struct {
	base.BaseDocument `bson:",inline"`

	// 基本信息
	Code        string `bson:"code" json:"code"`
	Title       string `bson:"title" json:"title"`
	Description string `bson:"description,omitempty" json:"description,omitempty"`

	// 分类信息
	Category      string   `bson:"category,omitempty" json:"category,omitempty"`           // 主类
	Stage         string   `bson:"stage,omitempty" json:"stage,omitempty"`                 // 阶段
	ApplicableAge string   `bson:"applicable_age,omitempty" json:"applicable_age,omitempty"` // 使用年龄
	Reporters     []string `bson:"reporters,omitempty" json:"reporters,omitempty"`         // 填报人列表
	Tags          []string `bson:"tags,omitempty" json:"tags,omitempty"`                  // 标签列表

	// 关联的问卷
	QuestionnaireCode    string `bson:"questionnaire_code,omitempty" json:"questionnaire_code,omitempty"`
	QuestionnaireVersion string `bson:"questionnaire_version,omitempty" json:"questionnaire_version,omitempty"`

	// 状态
	Status uint8 `bson:"status" json:"status"`

	// 因子列表
	Factors []FactorPO `bson:"factors,omitempty" json:"factors,omitempty"`
}

// CollectionName 返回集合名称
func (ScalePO) CollectionName() string {
	return "scales"
}

// BeforeInsert 插入前设置字段
func (p *ScalePO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
	p.DeletedBy = 0
}

// BeforeUpdate 更新前设置字段
func (p *ScalePO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

// ToBsonM 将 ScalePO 转换为 bson.M
func (p *ScalePO) ToBsonM() (bson.M, error) {
	data, err := bson.Marshal(p)
	if err != nil {
		return nil, err
	}

	var result bson.M
	if err := bson.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// FactorPO 因子持久化对象
type FactorPO struct {
	// 基本信息
	Code       string `bson:"code" json:"code"`
	Title      string `bson:"title" json:"title"`
	FactorType string `bson:"factor_type" json:"factor_type"`

	// 是否为总分因子
	IsTotalScore bool `bson:"is_total_score" json:"is_total_score"`

	// 关联的题目编码列表
	QuestionCodes []string `bson:"question_codes,omitempty" json:"question_codes,omitempty"`

	// 计分策略配置
	ScoringStrategy string                 `bson:"scoring_strategy" json:"scoring_strategy"`
	ScoringParams   map[string]interface{} `bson:"scoring_params,omitempty" json:"scoring_params,omitempty"`

	// 解读规则
	InterpretRules []InterpretRulePO `bson:"interpret_rules,omitempty" json:"interpret_rules,omitempty"`
}

// InterpretRulePO 解读规则持久化对象
type InterpretRulePO struct {
	MinScore   float64 `bson:"min_score" json:"min_score"`
	MaxScore   float64 `bson:"max_score" json:"max_score"`
	RiskLevel  string  `bson:"risk_level" json:"risk_level"`
	Conclusion string  `bson:"conclusion" json:"conclusion"`
	Suggestion string  `bson:"suggestion,omitempty" json:"suggestion,omitempty"`
}

// ScaleSummaryPO 量表摘要持久化对象（不包含 factors，用于列表查询）
type ScaleSummaryPO struct {
	Code              string   `bson:"code"`
	Title             string   `bson:"title"`
	Description       string   `bson:"description"`
	Category          string   `bson:"category"`
	Stage             string   `bson:"stage"`
	ApplicableAge     string   `bson:"applicable_age"`
	Reporters         []string `bson:"reporters"`
	Tags              []string `bson:"tags"`
	QuestionnaireCode string   `bson:"questionnaire_code"`
	Status            uint8    `bson:"status"`
}
