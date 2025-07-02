package questionnaire

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	base "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
)

// QuestionnairePO 问卷MongoDB持久化对象
// 对应MongoDB集合结构
type QuestionnairePO struct {
	base.BaseDocument `bson:",inline"`
	Code              string       `bson:"code" json:"code"`
	Title             string       `bson:"title" json:"title"`
	Description       string       `bson:"description,omitempty" json:"description,omitempty"`
	ImgUrl            string       `bson:"img_url,omitempty" json:"img_url,omitempty"`
	Version           string       `bson:"version" json:"version"`
	Status            uint8        `bson:"status" json:"status"`
	Questions         []QuestionPO `bson:"questions" json:"questions"`
}

// CollectionName 集合名称
func (QuestionnairePO) CollectionName() string {
	return "questionnaires"
}

// BeforeInsert 插入前设置字段
func (p *QuestionnairePO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	// 设置默认值
	if p.CreatedBy == 0 {
		p.CreatedBy = 0 // 可以从上下文中获取当前用户ID
	}
	p.UpdatedBy = p.CreatedBy
}

// BeforeUpdate 更新前设置字段
func (p *QuestionnairePO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
	// UpdatedBy 应该从上下文中获取当前用户ID
}

// QuestionPO 问题
type QuestionPO struct {
	Code            string             `bson:"code" json:"code"`
	Title           string             `bson:"title" json:"title"`
	QuestionType    string             `bson:"question_type" json:"question_type"`
	Tip             string             `bson:"tip" json:"tip"`
	Placeholder     string             `bson:"placeholder" json:"placeholder"`
	Options         []OptionPO         `bson:"options" json:"options"`
	ValidationRules []ValidationRulePO `bson:"validation_rules" json:"validation_rules"`
	CalculationRule CalculationRulePO  `bson:"calculation_rule" json:"calculation_rule"`
}

// OptionPO 选项
type OptionPO struct {
	Code    string `bson:"code" json:"code"`
	Content string `bson:"content" json:"content"`
	Score   int    `bson:"score" json:"score"`
}

// ValidationRulePO 校验规则
type ValidationRulePO struct {
	RuleType    string `bson:"rule_type" json:"rule_type"`
	TargetValue string `bson:"target_value" json:"target_value"`
}

// CalculationRulePO 计算规则
type CalculationRulePO struct {
	Formula string `bson:"formula" json:"formula"`
}
