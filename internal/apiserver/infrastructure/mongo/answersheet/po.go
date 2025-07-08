package answersheet

import (
	"time"

	base "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
	"github.com/yshujie/questionnaire-scale/pkg/util/idutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AnswerSheetPO 答卷MongoDB持久化对象
// 对应MongoDB集合结构
type AnswerSheetPO struct {
	base.BaseDocument    `bson:",inline"`
	QuestionnaireCode    string     `bson:"questionnaire_code" json:"questionnaire_code"`
	QuestionnaireVersion string     `bson:"questionnaire_version" json:"questionnaire_version"`
	Title                string     `bson:"title" json:"title"`
	Score                uint16     `bson:"score" json:"score"`
	Answers              []AnswerPO `bson:"answers" json:"answers"`
	Writer               *WriterPO  `bson:"writer" json:"writer"`
	Testee               *TesteePO  `bson:"testee" json:"testee"`
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
	p.DomainID = idutil.GetIntID()
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
	Score        uint16        `bson:"score" json:"score"`
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

// WriterPO 答卷者持久化对象
type WriterPO struct {
	UserID uint64 `bson:"id" json:"id"`
}

// ToBsonM 将 WriterPO 转换为 bson.M
func (p *WriterPO) ToBsonM() (bson.M, error) {
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
