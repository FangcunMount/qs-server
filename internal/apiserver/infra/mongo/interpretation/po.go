package interpretation

import (
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"time"
)

// ==================== ArchivedReport persistence shape ====================

// ArchivedReportPO is the immutable historical v0 report projection.
type ArchivedReportPO struct {
	base.BaseDocument `bson:",inline"`

	OutcomeID     uint64     `bson:"outcome_id,omitempty" json:"outcome_id,omitempty"`
	Status        string     `bson:"status,omitempty" json:"status,omitempty"`
	Attempt       uint       `bson:"attempt" json:"attempt"`
	FailureReason *string    `bson:"failure_reason,omitempty" json:"failure_reason,omitempty"`
	GeneratingAt  *time.Time `bson:"generating_at,omitempty" json:"generating_at,omitempty"`
	GeneratedAt   *time.Time `bson:"generated_at,omitempty" json:"generated_at,omitempty"`
	FailedAt      *time.Time `bson:"failed_at,omitempty" json:"failed_at,omitempty"`

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
	Kind           string         `bson:"kind,omitempty" json:"kind,omitempty"`
	FactorCode     string         `bson:"factor_code" json:"factor_code"`
	FactorName     string         `bson:"factor_name" json:"factor_name"`
	RawScore       float64        `bson:"raw_score" json:"raw_score"`
	MaxScore       *float64       `bson:"max_score,omitempty" json:"max_score,omitempty"`
	RiskLevel      string         `bson:"risk_level" json:"risk_level"`
	Role           string         `bson:"role,omitempty" json:"role,omitempty"`
	ParentCode     string         `bson:"parent_code,omitempty" json:"parent_code,omitempty"`
	HierarchyLevel int            `bson:"hierarchy_level,omitempty" json:"hierarchy_level,omitempty"`
	SortOrder      int            `bson:"sort_order,omitempty" json:"sort_order,omitempty"`
	Score          *ScoreValuePO  `bson:"score,omitempty" json:"score,omitempty"`
	Level          *ResultLevelPO `bson:"level,omitempty" json:"level,omitempty"`
	Description    string         `bson:"description" json:"description"`
	Suggestion     string         `bson:"suggestion,omitempty" json:"suggestion,omitempty"`
}

type ModelIdentityPO struct {
	Kind            string `bson:"kind,omitempty" json:"kind,omitempty"`
	SubKind         string `bson:"sub_kind,omitempty" json:"sub_kind,omitempty"`
	Algorithm       string `bson:"algorithm,omitempty" json:"algorithm,omitempty"`
	Code            string `bson:"code,omitempty" json:"code,omitempty"`
	Version         string `bson:"version,omitempty" json:"version,omitempty"`
	Title           string `bson:"title,omitempty" json:"title,omitempty"`
	ProductChannel  string `bson:"product_channel,omitempty" json:"product_channel,omitempty"`
	AlgorithmFamily string `bson:"algorithm_family,omitempty" json:"algorithm_family,omitempty"`
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

func (ArchivedReportPO) CollectionName() string {
	return "archived_reports"
}
