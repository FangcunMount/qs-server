package questionnaire

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	base "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mongo"
)

// QuestionnaireDocument 问卷MongoDB文档
// 对应MongoDB集合结构
type QuestionnaireDocument struct {
	base.BaseDocument `bson:",inline"`
	DomainID          uint64 `bson:"domain_id" json:"domain_id"` // 领域模型ID
	Code              string `bson:"code" json:"code"`
	Title             string `bson:"title" json:"title"`
	Description       string `bson:"description,omitempty" json:"description,omitempty"`
	ImgUrl            string `bson:"img_url,omitempty" json:"img_url,omitempty"`
	Version           uint8  `bson:"version" json:"version"`
	Status            uint8  `bson:"status" json:"status"`
}

// CollectionName 集合名称
func (QuestionnaireDocument) CollectionName() string {
	return "questionnaires"
}

// BeforeInsert 插入前设置字段
func (d *QuestionnaireDocument) BeforeInsert() {
	if d.ID.IsZero() {
		d.ID = primitive.NewObjectID()
	}
	now := time.Now()
	d.CreatedAt = now
	d.UpdatedAt = now

	// 设置默认值
	if d.CreatedBy == 0 {
		d.CreatedBy = 0 // 可以从上下文中获取当前用户ID
	}
	d.UpdatedBy = d.CreatedBy
}

// BeforeUpdate 更新前设置字段
func (d *QuestionnaireDocument) BeforeUpdate() {
	d.UpdatedAt = time.Now()
	// UpdatedBy 应该从上下文中获取当前用户ID
}
