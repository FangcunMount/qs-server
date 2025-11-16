package medicalscale

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/FangcunMount/component-base/pkg/util/idutil"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// MedicalScalePO 医学量表MongoDB持久化对象
type MedicalScalePO struct {
	base.BaseDocument    `bson:",inline"`
	Code                 string     `bson:"code" json:"code"`
	Title                string     `bson:"title" json:"title"`
	QuestionnaireCode    string     `bson:"questionnaire_code" json:"questionnaire_code"`
	QuestionnaireVersion string     `bson:"questionnaire_version" json:"questionnaire_version"`
	Factors              []FactorPO `bson:"factors" json:"factors"`
}

// CollectionName 集合名称
func (MedicalScalePO) CollectionName() string {
	return "medical_scales"
}

// BeforeInsert 插入前设置字段
func (p *MedicalScalePO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	p.DomainID = meta.ID(idutil.GetIntID())
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.DeletedAt = nil

	// 设置默认值
	if p.CreatedBy == 0 {
		p.CreatedBy = 0 // 可以从上下文中获取当前用户ID
	}
	p.UpdatedBy = p.CreatedBy
	p.DeletedBy = 0
}

// BeforeUpdate 更新前设置字段
func (p *MedicalScalePO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
	// UpdatedBy 应该从上下文中获取当前用户ID
}

// ToBsonM 将 MedicalScalePO 转换为 bson.M
func (p *MedicalScalePO) ToBsonM() (bson.M, error) {
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

// FromBsonM 从 bson.M 创建 MedicalScalePO
func (p *MedicalScalePO) FromBsonM(data bson.M) error {
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

// ToBsonMWithFilter 将 MedicalScalePO 转换为 bson.M，支持字段过滤
func (p *MedicalScalePO) ToBsonMWithFilter(includeFields []string) (bson.M, error) {
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

// FactorPO 因子持久化对象
type FactorPO struct {
	Code            string            `bson:"code" json:"code"`
	Title           string            `bson:"title" json:"title"`
	IsTotalScore    bool              `bson:"is_total_score" json:"is_total_score"`
	FactorType      string            `bson:"factor_type" json:"factor_type"`
	CalculationRule CalculationRulePO `bson:"calculation_rule" json:"calculation_rule"`
	InterpretRules  []InterpretRulePO `bson:"interpret_rules" json:"interpret_rules"`
}

// ToBsonM 将 FactorPO 转换为 bson.M
func (p *FactorPO) ToBsonM() (bson.M, error) {
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

// CalculationRulePO 计算规则持久化对象
type CalculationRulePO struct {
	FormulaType string   `bson:"formula_type" json:"formula_type"`
	SourceCodes []string `bson:"source_codes" json:"source_codes"`
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

// InterpretRulePO 解读规则持久化对象
type InterpretRulePO struct {
	ScoreRange ScoreRangePO `bson:"score_range" json:"score_range"`
	Content    string       `bson:"content" json:"content"`
}

// ToBsonM 将 InterpretRulePO 转换为 bson.M
func (p *InterpretRulePO) ToBsonM() (bson.M, error) {
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

// ScoreRangePO 分数范围持久化对象
type ScoreRangePO struct {
	MinScore float64 `bson:"min_score" json:"min_score"`
	MaxScore float64 `bson:"max_score" json:"max_score"`
}

// ToBsonM 将 ScoreRangePO 转换为 bson.M
func (p *ScoreRangePO) ToBsonM() (bson.M, error) {
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
