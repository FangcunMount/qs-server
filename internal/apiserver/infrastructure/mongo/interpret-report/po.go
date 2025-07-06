package interpretreport

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	base "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
)

// InterpretReportPO 解读报告MongoDB持久化对象
type InterpretReportPO struct {
	base.BaseDocument `bson:",inline"`
	AnswerSheetId     uint64            `bson:"answer_sheet_id" json:"answer_sheet_id"`
	MedicalScaleCode  string            `bson:"medical_scale_code" json:"medical_scale_code"`
	Title             string            `bson:"title" json:"title"`
	Description       string            `bson:"description" json:"description"`
	Testee            *TesteePO         `bson:"testee" json:"testee"`
	InterpretItems    []InterpretItemPO `bson:"interpret_items" json:"interpret_items"`
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
func (p *InterpretReportPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
	// UpdatedBy 应该从上下文中获取当前用户ID
}

// ToBsonM 将 InterpretReportPO 转换为 bson.M
func (p *InterpretReportPO) ToBsonM() (bson.M, error) {
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

// FromBsonM 从 bson.M 创建 InterpretReportPO
func (p *InterpretReportPO) FromBsonM(data bson.M) error {
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

// InterpretItemPO 解读项持久化对象
type InterpretItemPO struct {
	FactorCode string    `bson:"factor_code" json:"factor_code"`
	Title      string    `bson:"title" json:"title"`
	Score      int       `bson:"score" json:"score"`
	Content    string    `bson:"content" json:"content"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

// ToBsonM 将 InterpretItemPO 转换为 bson.M
func (p *InterpretItemPO) ToBsonM() (bson.M, error) {
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

// BeforeInsert 插入前设置字段
func (p *InterpretItemPO) BeforeInsert() {
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
}

// BeforeUpdate 更新前设置字段
func (p *InterpretItemPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
}

// TesteePO 被试者持久化对象
type TesteePO struct {
	UserID uint64 `bson:"id" json:"id"`
}

// ToBsonM 将 TesteePO 转换为 bson.M
func (p *TesteePO) ToBsonM() (bson.M, error) {
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
