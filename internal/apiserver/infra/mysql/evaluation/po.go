package evaluation

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
)

// ==================== Assessment 持久化对象 ====================

// AssessmentPO 测评持久化对象
type AssessmentPO struct {
	mysql.AuditFields

	// 组织信息
	OrgID int64 `gorm:"column:org_id;not null"`

	// 受试者信息
	TesteeID uint64 `gorm:"column:testee_id;not null"`

	// 问卷引用
	QuestionnaireCode    string `gorm:"column:questionnaire_code;size:100;not null"`
	QuestionnaireVersion string `gorm:"column:questionnaire_version;size:50;not null"`

	// 通用解释模型引用（可选）
	EvaluationModelKind      *string `gorm:"column:evaluation_model_kind;size:50;index:idx_evaluation_model"`
	EvaluationModelSubKind   *string `gorm:"column:evaluation_model_sub_kind;size:50"`
	EvaluationModelAlgorithm *string `gorm:"column:evaluation_model_algorithm;size:50"`
	EvaluationModelCode      *string `gorm:"column:evaluation_model_code;size:100;index:idx_evaluation_model"`
	EvaluationModelVersion   *string `gorm:"column:evaluation_model_version;size:50"`
	EvaluationModelTitle     *string `gorm:"column:evaluation_model_title;size:255"`

	// v2 outcome summary projection
	PrimaryScoreKind  *string  `gorm:"column:primary_score_kind;size:50"`
	PrimaryScoreValue *float64 `gorm:"column:primary_score_value"`
	PrimaryScoreLabel *string  `gorm:"column:primary_score_label;size:100"`
	PrimaryScoreMax   *float64 `gorm:"column:primary_score_max"`
	LevelCode         *string  `gorm:"column:level_code;size:50;index:idx_assessment_level_code"`
	LevelLabel        *string  `gorm:"column:level_label;size:100"`
	Severity          *string  `gorm:"column:severity;size:50;index:idx_assessment_severity"`

	// 答卷引用
	AnswerSheetID uint64 `gorm:"column:answer_sheet_id;not null;uniqueIndex:uk_answer_sheet_id"`

	// 来源信息
	OriginType string  `gorm:"column:origin_type;size:50;not null"`
	OriginID   *string `gorm:"column:origin_id;size:100"`

	// 状态
	Status string `gorm:"column:status;size:50;not null"`

	// 评估结果（可选）
	TotalScore *float64 `gorm:"column:total_score"`
	RiskLevel  *string  `gorm:"column:risk_level;size:50;index:idx_risk_level"`

	// 时间戳
	SubmittedAt   *time.Time `gorm:"column:submitted_at"`
	EvaluatedAt   *time.Time `gorm:"column:evaluated_at"`
	FailedAt      *time.Time `gorm:"column:failed_at"`
	FailureReason *string    `gorm:"column:failure_reason;size:500"`
	CurrentRunID  *string    `gorm:"column:current_run_id;size:100;index:idx_assessment_current_run_id"`
}

// TableName 指定表名
func (AssessmentPO) TableName() string {
	return "assessment"
}

// BeforeCreate GORM hook，在创建前执行
func (p *AssessmentPO) BeforeCreate(_ *gorm.DB) error {
	// 如果 ID 为 0，使用 ID 生成器生成 ID
	if p.ID == 0 {
		p.ID = meta.New()
	}
	// 设置默认版本号
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}

// ==================== AssessmentScore 持久化对象 ====================

// AssessmentScorePO 测评得分持久化对象
type AssessmentScorePO struct {
	mysql.AuditFields

	// 关联 Assessment
	AssessmentID        uint64  `gorm:"column:assessment_id;not null"`
	EvaluationOutcomeID *uint64 `gorm:"column:evaluation_outcome_id;index:idx_assessment_score_outcome"`

	// 受试者（冗余，用于趋势分析查询）
	TesteeID uint64 `gorm:"column:testee_id;not null"`

	// 因子信息
	FactorCode   string `gorm:"column:factor_code;size:100;not null"`
	FactorName   string `gorm:"column:factor_name;size:255;not null"`
	IsTotalScore bool   `gorm:"column:is_total_score;not null;default:false"`

	// 得分
	RawScore float64 `gorm:"column:raw_score;not null"`

	RiskLevel string `gorm:"column:risk_level;size:50;not null;index:idx_risk_level"`
}

// TableName 指定表名
func (AssessmentScorePO) TableName() string {
	return "assessment_score"
}

// BeforeCreate GORM hook，在创建前执行
func (p *AssessmentScorePO) BeforeCreate(_ *gorm.DB) error {
	// 如果 ID 为 0，使用 ID 生成器生成 ID
	if p.ID == 0 {
		p.ID = meta.New()
	}
	// 设置默认版本号
	if p.Version == 0 {
		p.Version = mysql.InitialVersion
	}
	return nil
}

// ==================== 辅助类型 ====================

// StringSlice 字符串切片列，用于 JSON 存储
type StringSlice []string

// Value 实现 driver.Valuer 接口
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan 实现 sql.Scanner 接口
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	return json.Unmarshal(bytes, s)
}
