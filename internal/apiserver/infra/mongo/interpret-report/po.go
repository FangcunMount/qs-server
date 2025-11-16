package interpretreport

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/FangcunMount/component-base/pkg/util/idutil"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
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

// TesteePO 被试者持久化对象
type TesteePO struct {
	UserID uint64 `bson:"user_id" json:"user_id"`
}

// InterpretItemPO 解读项持久化对象
type InterpretItemPO struct {
	FactorCode string  `bson:"factor_code" json:"factor_code"`
	Title      string  `bson:"title" json:"title"`
	Score      float64 `bson:"score" json:"score"`
	Content    string  `bson:"content" json:"content"`
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
	p.DomainID = idutil.GetIntID()
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
