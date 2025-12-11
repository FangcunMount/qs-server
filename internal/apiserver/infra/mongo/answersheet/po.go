package answersheet

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/util/idutil"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AnswerSheetPO 答卷MongoDB持久化对象
// 对应MongoDB集合结构
type AnswerSheetPO struct {
	base.BaseDocument    `bson:",inline"`
	QuestionnaireCode    string     `bson:"questionnaire_code" json:"questionnaire_code"`
	QuestionnaireVersion string     `bson:"questionnaire_version" json:"questionnaire_version"`
	QuestionnaireTitle   string     `bson:"questionnaire_title" json:"questionnaire_title"`
	FillerID             int64      `bson:"filler_id" json:"filler_id"`
	FillerType           string     `bson:"filler_type" json:"filler_type"`
	TotalScore           float64    `bson:"total_score" json:"total_score"`
	FilledAt             time.Time  `bson:"filled_at" json:"filled_at"`
	Answers              []AnswerPO `bson:"answers" json:"answers"`
}

// CollectionName 集合名称
func (AnswerSheetPO) CollectionName() string {
	return "answersheets"
}

// BeforeInsert 插入前设置字段
func (p *AnswerSheetPO) BeforeInsert() {
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}

	// 生成DomainID
	domainID := meta.ID(idutil.GetIntID())
	p.DomainID = domainID

	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	p.DeletedAt = nil

	// 设置默认值
	if p.CreatedBy == 0 {
		p.CreatedBy = 0 // 可以从上下文中获取当前用户ID
	}
	p.UpdatedBy = p.CreatedBy
	p.DeletedBy = 0
}

// BeforeUpdate 更新前设置字段
func (p *AnswerSheetPO) BeforeUpdate() {
	p.UpdatedAt = time.Now()
	// UpdatedBy 应该从上下文中获取当前用户ID
}

// ToBsonM 将 AnswerSheetPO 转换为 bson.M
func (p *AnswerSheetPO) ToBsonM() (bson.M, error) {
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

// FromBsonM 从 bson.M 创建 AnswerSheetPO
func (p *AnswerSheetPO) FromBsonM(data bson.M) error {
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

// AnswerPO 答案持久化对象
type AnswerPO struct {
	QuestionCode string        `bson:"question_code" json:"question_code"`
	QuestionType string        `bson:"question_type" json:"question_type"`
	Score        float64       `bson:"score" json:"score"`
	Value        AnswerValuePO `bson:"value" json:"value"`
}

// ToBsonM 将 AnswerPO 转换为 bson.M
func (p *AnswerPO) ToBsonM() (bson.M, error) {
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

// AnswerValuePO 答案值持久化对象
type AnswerValuePO struct {
	Value interface{} `bson:"value" json:"value"`
}

// ToBsonM 将 AnswerValuePO 转换为 bson.M
func (p *AnswerValuePO) ToBsonM() (bson.M, error) {
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

// AnswerSheetSummaryPO 答卷摘要持久化对象（不包含 answers 字段，用于列表查询）
type AnswerSheetSummaryPO struct {
	DomainID           uint64     `bson:"domain_id"`
	QuestionnaireCode  string     `bson:"questionnaire_code"`
	QuestionnaireTitle string     `bson:"questionnaire_title"`
	FillerID           int64      `bson:"filler_id"`
	FillerType         string     `bson:"filler_type"`
	TotalScore         float64    `bson:"total_score"`
	AnswerCount        int        `bson:"answer_count"` // 由聚合管道计算
	FilledAt           *time.Time `bson:"filled_at"`
}
