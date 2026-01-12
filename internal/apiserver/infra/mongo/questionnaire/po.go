package questionnaire

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// QuestionnairePO 问卷MongoDB持久化对象
// 对应MongoDB集合结构
type QuestionnairePO struct {
	base.BaseDocument `bson:",inline"`
	Code              string       `bson:"code" json:"code"` // 问卷唯一标识
	Title             string       `bson:"title" json:"title"`
	Description       string       `bson:"description,omitempty" json:"description,omitempty"`
	ImgUrl            string       `bson:"img_url,omitempty" json:"img_url,omitempty"`
	Version           string       `bson:"version" json:"version"`
	Status            string       `bson:"status" json:"status"`
	Type              string       `bson:"type" json:"type"`
	Questions         []QuestionPO `bson:"questions,omitempty" json:"questions,omitempty"`
	QuestionCount     int          `bson:"question_count,omitempty" json:"question_count,omitempty"`
}

// CollectionName 集合名称
func (QuestionnairePO) CollectionName() string {
	return "questionnaires"
}

// BeforeInsert 插入前设置字段
func (p *QuestionnairePO) BeforeInsert() {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil
	p.QuestionCount = len(p.Questions)

	// 设置默认值
	if p.CreatedBy == 0 {
		p.CreatedBy = 0 // 可以从上下文中获取当前用户ID
	}
	p.UpdatedBy = p.CreatedBy
	p.DeletedBy = 0
}

// BeforeUpdate 更新前设置字段
func (p *QuestionnairePO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
	p.QuestionCount = len(p.Questions)
	// UpdatedBy 应该从上下文中获取当前用户ID
}

// ToBsonM 将 QuestionnairePO 转换为 bson.M
func (p *QuestionnairePO) ToBsonM() (bson.M, error) {
	// 使用 bson.Marshal 序列化结构体
	data, err := bson.Marshal(p)
	if err != nil {
		return nil, err
	}

	// 使用 bson.Unmarshal 反序列化为 bson.M
	var result bson.M
	err = bson.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// FromBsonM 从 bson.M 创建 QuestionnairePO
func (p *QuestionnairePO) FromBsonM(data bson.M) error {
	// 使用 bson.Marshal 序列化 bson.M
	bsonData, err := bson.Marshal(data)
	if err != nil {
		return err
	}

	// 使用 bson.Unmarshal 反序列化为结构体
	err = bson.Unmarshal(bsonData, p)
	if err != nil {
		return err
	}

	return nil
}

// ToBsonMWithFilter 将 QuestionnairePO 转换为 bson.M，支持字段过滤
func (p *QuestionnairePO) ToBsonMWithFilter(includeFields []string) (bson.M, error) {
	// 先转换为完整的 bson.M
	fullBsonM, err := p.ToBsonM()
	if err != nil {
		return nil, err
	}

	// 如果没有指定字段，返回完整的 bson.M
	if len(includeFields) == 0 {
		return fullBsonM, nil
	}

	// 过滤指定字段
	filtered := make(bson.M)
	for _, field := range includeFields {
		if value, exists := fullBsonM[field]; exists {
			filtered[field] = value
		}
	}

	return filtered, nil
}

// QuestionPO 问题
type QuestionPO struct {
	Code            string             `bson:"code" json:"code"`
	Title           string             `bson:"title" json:"title"`
	QuestionType    string             `bson:"question_type" json:"question_type"`
	Tips            string             `bson:"tips" json:"tip"`
	Placeholder     string             `bson:"placeholder" json:"placeholder"`
	Options         []OptionPO         `bson:"options" json:"options"`
	ValidationRules []ValidationRulePO `bson:"validation_rules" json:"validation_rules"`
	CalculationRule CalculationRulePO  `bson:"calculation_rule" json:"calculation_rule"`
	ShowController  *ShowControllerPO  `bson:"show_controller,omitempty" json:"show_controller,omitempty"`
}

// ShowControllerPO 显示控制器持久化对象
type ShowControllerPO struct {
	Rule      string                      `bson:"rule" json:"rule"`
	Questions []ShowControllerConditionPO `bson:"questions" json:"questions"`
}

// ShowControllerConditionPO 显示控制条件持久化对象
type ShowControllerConditionPO struct {
	Code              string   `bson:"code" json:"code"`
	SelectOptionCodes []string `bson:"select_option_codes" json:"select_option_codes"`
}

// ToBsonM 将 QuestionPO 转换为 bson.M
func (p *QuestionPO) ToBsonM() (bson.M, error) {
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

// OptionPO 选项
type OptionPO struct {
	Code    string  `bson:"code" json:"code"`
	Content string  `bson:"content" json:"content"`
	Score   float64 `bson:"score" json:"score"`
}

// ToBsonM 将 OptionPO 转换为 bson.M
func (p *OptionPO) ToBsonM() (bson.M, error) {
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

// ValidationRulePO 校验规则
type ValidationRulePO struct {
	RuleType    string `bson:"rule_type" json:"rule_type"`
	TargetValue string `bson:"target_value" json:"target_value"`
}

// ToBsonM 将 ValidationRulePO 转换为 bson.M
func (p *ValidationRulePO) ToBsonM() (bson.M, error) {
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

// CalculationRulePO 计算规则
type CalculationRulePO struct {
	Formula string `bson:"formula" json:"formula"`
}

// ToBsonM 将 CalculationRulePO 转换为 bson.M
func (p *CalculationRulePO) ToBsonM() (bson.M, error) {
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
