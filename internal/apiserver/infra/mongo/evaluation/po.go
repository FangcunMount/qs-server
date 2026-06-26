package evaluation

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/FangcunMount/component-base/pkg/util/idutil"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// ==================== InterpretReport 持久化对象 ====================

// InterpretReportPO 解读报告MongoDB持久化对象
type InterpretReportPO struct {
	base.BaseDocument `bson:",inline"`

	// 量表信息（legacy v1）
	ScaleName string `bson:"scale_name" json:"scale_name"`
	ScaleCode string `bson:"scale_code" json:"scale_code"`

	// v2 model identity and outcome summary
	Model        *ModelIdentityPO `bson:"model,omitempty" json:"model,omitempty"`
	PrimaryScore *ScoreValuePO    `bson:"primary_score,omitempty" json:"primary_score,omitempty"`
	Level        *ResultLevelPO   `bson:"level,omitempty" json:"level,omitempty"`

	// 受试者ID（冗余，用于查询）
	TesteeID uint64 `bson:"testee_id" json:"testee_id"`

	// 评估结果汇总
	TotalScore float64 `bson:"total_score" json:"total_score"`
	RiskLevel  string  `bson:"risk_level" json:"risk_level"`
	Conclusion string  `bson:"conclusion" json:"conclusion"`

	// 维度解读列表
	Dimensions []DimensionInterpretPO `bson:"dimensions" json:"dimensions"`

	// 建议列表
	Suggestions []SuggestionPO `bson:"suggestions" json:"suggestions"`

	// 解释模型扩展（SBTI 等人格类测评）
	ModelExtra *ModelExtraPO `bson:"model_extra,omitempty" json:"model_extra,omitempty"`
}

// DimensionInterpretPO 维度解读持久化对象
type DimensionInterpretPO struct {
	FactorCode  string         `bson:"factor_code" json:"factor_code"`
	FactorName  string         `bson:"factor_name" json:"factor_name"`
	RawScore    float64        `bson:"raw_score" json:"raw_score"`
	MaxScore    *float64       `bson:"max_score,omitempty" json:"max_score,omitempty"`
	RiskLevel   string         `bson:"risk_level" json:"risk_level"`
	Score       *ScoreValuePO  `bson:"score,omitempty" json:"score,omitempty"`
	Level       *ResultLevelPO `bson:"level,omitempty" json:"level,omitempty"`
	Description string         `bson:"description" json:"description"`
	Suggestion  string         `bson:"suggestion,omitempty" json:"suggestion,omitempty"`
}

type ModelIdentityPO struct {
	Kind      string `bson:"kind,omitempty" json:"kind,omitempty"`
	SubKind   string `bson:"sub_kind,omitempty" json:"sub_kind,omitempty"`
	Algorithm string `bson:"algorithm,omitempty" json:"algorithm,omitempty"`
	Code      string `bson:"code,omitempty" json:"code,omitempty"`
	Version   string `bson:"version,omitempty" json:"version,omitempty"`
	Title     string `bson:"title,omitempty" json:"title,omitempty"`
}

type ScoreValuePO struct {
	Kind  string   `bson:"kind,omitempty" json:"kind,omitempty"`
	Value float64  `bson:"value" json:"value"`
	Label string   `bson:"label,omitempty" json:"label,omitempty"`
	Max   *float64 `bson:"max,omitempty" json:"max,omitempty"`
}

type ResultLevelPO struct {
	Code     string `bson:"code,omitempty" json:"code,omitempty"`
	Label    string `bson:"label,omitempty" json:"label,omitempty"`
	Severity string `bson:"severity,omitempty" json:"severity,omitempty"`
}

// SuggestionPO 结构化建议持久化对象
type SuggestionPO struct {
	Category   string  `bson:"category" json:"category"`
	Content    string  `bson:"content" json:"content"`
	FactorCode *string `bson:"factor_code,omitempty" json:"factor_code,omitempty"`
}

// ModelExtraPO 解释模型扩展持久化对象
type ModelExtraPO struct {
	Kind           string         `bson:"kind,omitempty" json:"kind,omitempty"`
	TypeCode       string         `bson:"type_code,omitempty" json:"type_code,omitempty"`
	TypeName       string         `bson:"type_name,omitempty" json:"type_name,omitempty"`
	OneLiner       string         `bson:"one_liner,omitempty" json:"one_liner,omitempty"`
	ImageURL       string         `bson:"image_url,omitempty" json:"image_url,omitempty"`
	MatchPercent   float64        `bson:"match_percent,omitempty" json:"match_percent,omitempty"`
	IsSpecial      bool           `bson:"is_special,omitempty" json:"is_special,omitempty"`
	SpecialTrigger string         `bson:"special_trigger,omitempty" json:"special_trigger,omitempty"`
	Rarity         *ModelRarityPO `bson:"rarity,omitempty" json:"rarity,omitempty"`
	Commentary     string         `bson:"commentary,omitempty" json:"commentary,omitempty"`
}

// ModelRarityPO 理论稀有度持久化对象
type ModelRarityPO struct {
	Percent float64 `bson:"percent,omitempty" json:"percent,omitempty"`
	Label   string  `bson:"label,omitempty" json:"label,omitempty"`
	OneInX  int     `bson:"one_in_x,omitempty" json:"one_in_x,omitempty"`
}

// CollectionName 集合名称
func (InterpretReportPO) CollectionName() string {
	return "interpret_reports"
}

// BeforeInsert 插入前设置字段
func (p *InterpretReportPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	if p.DomainID.IsZero() {
		p.DomainID = report.NewID(idutil.GetIntID())
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
}

// BeforeUpdate 更新前设置字段
func (p *InterpretReportPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

// ToBsonM 转换为 BSON.M
func (p *InterpretReportPO) ToBsonM() (bson.M, error) {
	data, err := bson.Marshal(p)
	if err != nil {
		return nil, err
	}

	var result bson.M
	err = bson.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
