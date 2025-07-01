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
	DomainID          uint64 `bson:"domain_id" json:"domain_id"` // 领域模型ID
	Code              string `bson:"code" json:"code"`
	Title             string `bson:"title" json:"title"`
	Description       string `bson:"description,omitempty" json:"description,omitempty"`
	ImgUrl            string `bson:"img_url,omitempty" json:"img_url,omitempty"`
	Version           uint8  `bson:"version" json:"version"`
	Status            uint8  `bson:"status" json:"status"`
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
